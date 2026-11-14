package service

import (
	"errors"
	"sort"
	"strings"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"
)

// DrillOrgBrief 组织节点简要信息。
type DrillOrgBrief struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	Code          string `json:"code"`
	Type          string `json:"type"`
	ManagerName   string `json:"manager_name"`
	Level         int    `json:"level"`
	ChildCount    int    `json:"child_count"`
	EmployeeCount int    `json:"employee_count"`
}

// DrillNode 下钻结果中的一行：组织 / 员工及其指标、目标达成情况。
type DrillNode struct {
	ID            uint64   `json:"id"`
	Name          string   `json:"name"`
	Code          string   `json:"code"`
	Type          string   `json:"type"`
	ManagerName   string   `json:"manager_name"`
	ParentID      uint64   `json:"parent_id"`
	ParentName    string   `json:"parent_name"`
	ActualValue   float64  `json:"actual_value"`
	TargetValue   float64  `json:"target_value"`
	Rate          *float64 `json:"rate"`
	HasTarget     bool     `json:"has_target"`
	Status        string   `json:"status"`
	Orders        int      `json:"orders"`
	GrossProfit   float64  `json:"gross_profit"`
	Customers     int      `json:"customers"`
	EmployeeCount int      `json:"employee_count"`
	ChildCount    int      `json:"child_count"`
	HasChildren   bool     `json:"has_children"`
	Drillable     bool     `json:"drillable"`
}

// DrillSummary 当前层级汇总指标。
type DrillSummary struct {
	ActualValue float64 `json:"actual_value"`
	TargetValue float64 `json:"target_value"`
	Rate        float64 `json:"rate"`
	TargetCount int     `json:"target_count"`
	NodeCount   int     `json:"node_count"`
}

// DrillResult 一次下钻查询的完整结果（面包屑、汇总、节点列表）。
type DrillResult struct {
	Current       DrillOrgBrief   `json:"current"`
	Parent        *DrillOrgBrief  `json:"parent"`
	Breadcrumbs   []DrillOrgBrief `json:"breadcrumbs"`
	Mode          string          `json:"mode"`
	PeriodType    string          `json:"period_type"`
	Period        string          `json:"period"`
	StartDate     string          `json:"start_date"`
	EndDate       string          `json:"end_date"`
	Metric        string          `json:"metric"`
	MetricLabel   string          `json:"metric_label"`
	Unit          string          `json:"unit"`
	NextLevelType string          `json:"next_level_type"`
	Summary       DrillSummary    `json:"summary"`
	Nodes         []DrillNode     `json:"nodes"`
}

// DrillService 组织下钻服务。
type DrillService struct {
	orgRepo    *repository.OrganizationRepository
	targetRepo *repository.TargetRepository
	repo       *repository.DrillRepository
	scope      *ScopeService
}

// NewDrillService 创建下钻服务。
func NewDrillService(orgRepo *repository.OrganizationRepository, targetRepo *repository.TargetRepository, repo *repository.DrillRepository, scope *ScopeService) *DrillService {
	return &DrillService{orgRepo: orgRepo, targetRepo: targetRepo, repo: repo, scope: scope}
}

// Query 按组织节点与模式（下级汇总 / 扁平全部 / 门店员工）执行下钻查询。
func (s *DrillService) Query(user *model.User, orgID uint64, periodType, period, metric, mode string) (*DrillResult, error) {
	scope, err := s.scope.Resolve(user)
	if err != nil {
		return nil, err
	}
	return s.QueryScoped(scope, user.OrganizationID, orgID, periodType, period, metric, mode)
}

// QueryScoped 按显式组织范围下钻，供授权分享链接等无登录态场景复用。
func (s *DrillService) QueryScoped(scope DataScope, ownOrgID, orgID uint64, periodType, period, metric, mode string) (*DrillResult, error) {
	orgs, err := s.orgRepo.All()
	if err != nil {
		return nil, err
	}
	byID := make(map[uint64]model.Organization, len(orgs))
	children := make(map[uint64][]model.Organization)
	for _, org := range orgs {
		byID[org.ID] = org
		if org.ParentID != nil {
			children[*org.ParentID] = append(children[*org.ParentID], org)
		}
	}
	allowed := make(map[uint64]bool, len(scope.OrgIDs))
	for _, id := range scope.OrgIDs {
		allowed[id] = true
	}
	current, err := s.resolveCurrent(orgID, ownOrgID, scope, orgs, byID, allowed)
	if err != nil {
		return nil, err
	}
	if periodType == "" {
		periodType = "month"
	}
	if period == "" {
		period = CurrentPeriod(periodType)
	}
	start, end, err := PeriodRange(periodType, period, time.Now())
	if err != nil {
		return nil, err
	}
	metricDef := findMetric(metric)
	if metricDef == nil {
		metricDef = findMetric("revenue")
	}
	if mode == "" {
		mode = "level"
	}
	if mode != "level" && mode != "all" && mode != "employees" {
		return nil, errors.New("视图模式仅支持 level / all / employees")
	}
	subtree := s.subtreeIDs(current.ID, children, allowed)
	result := &DrillResult{
		Current:     s.brief(current, children, subtree),
		Mode:        mode,
		PeriodType:  periodType,
		Period:      period,
		StartDate:   start.Format("2006-01-02"),
		EndDate:     end.Format("2006-01-02"),
		Metric:      metricDef.Key,
		MetricLabel: metricDef.Label,
		Unit:        metricDef.Unit,
		Nodes:       make([]DrillNode, 0),
	}
	result.Breadcrumbs = s.breadcrumbs(current, byID)
	if current.ParentID != nil {
		if parent, ok := byID[*current.ParentID]; ok {
			brief := s.brief(parent, children, s.subtreeIDs(parent.ID, children, allowed))
			result.Parent = &brief
		}
	}

	actualRows, err := s.repo.SalesByOrg(subtree, result.StartDate, result.EndDate, metricDef.Expr)
	if err != nil {
		return nil, err
	}
	actualByOrg := make(map[uint64]repository.SalesAggregate, len(actualRows))
	for _, row := range actualRows {
		actualByOrg[row.OrgID] = row
		result.Summary.ActualValue += row.ActualValue
	}

	switch mode {
	case "level":
		direct := make([]model.Organization, 0)
		for _, child := range children[current.ID] {
			if allowed[child.ID] && child.Status != "disabled" {
				direct = append(direct, child)
			}
		}
		if len(direct) == 0 && current.Type == "store" {
			result.Nodes, result.NextLevelType, err = s.employeeNodes(current, subtree, *metricDef, result.StartDate, result.EndDate, byID)
		} else {
			result.NextLevelType = ""
			if len(direct) > 0 {
				result.NextLevelType = direct[0].Type
			}
			result.Nodes, err = s.orgNodes(direct, children, allowed, actualByOrg, *metricDef, result.StartDate, result.EndDate, byID)
			if len(direct) == 0 {
				result.NextLevelType = ""
			}
		}
	case "all":
		if current.Type == "store" {
			result.Nodes, result.NextLevelType, err = s.employeeNodes(current, subtree, *metricDef, result.StartDate, result.EndDate, byID)
			result.NextLevelType = "employee"
		} else {
			stores := s.storesInSubtree(current, children, allowed, byID)
			result.Nodes, err = s.orgNodes(stores, children, allowed, actualByOrg, *metricDef, result.StartDate, result.EndDate, byID)
			result.NextLevelType = "store"
		}
	case "employees":
		stores := s.storesInSubtree(current, children, allowed, byID)
		result.Nodes, result.NextLevelType, err = s.employeeNodesForStores(stores, *metricDef, result.StartDate, result.EndDate, byID)
		result.NextLevelType = "employee"
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].ActualValue > result.Nodes[j].ActualValue
	})

	if len(subtree) > 0 {
		targets, targetErr := s.targetRepo.List(periodType, period, metricDef.Key, "", subtree)
		if targetErr != nil {
			return nil, targetErr
		}
		targetMap := make(map[uint64]float64, len(targets))
		challengeMap := make(map[uint64]float64, len(targets))
		for _, item := range targets {
			if item.TargetValue > 0 {
				if item.TaskType == TaskTypeChallenge {
					challengeMap[item.OrgID] = item.TargetValue
				} else {
					targetMap[item.OrgID] = item.TargetValue
				}
			}
		}
		// 优先使用基础任务目标，未设置基础任务时回退到挑战任务。
		for orgID, value := range challengeMap {
			if _, ok := targetMap[orgID]; !ok {
				targetMap[orgID] = value
			}
		}
		// 节点未单独设置目标时，向下汇总子组织目标；已设置则优先使用本节点目标，避免重复计算。
		cache := make(map[uint64]float64)
		var effectiveTarget func(uint64) float64
		effectiveTarget = func(id uint64) float64 {
			if value, ok := cache[id]; ok {
				return value
			}
			if value, ok := targetMap[id]; ok {
				cache[id] = value
				return value
			}
			total := 0.0
			for _, child := range children[id] {
				if !allowed[child.ID] || child.Status == "disabled" {
					continue
				}
				total += effectiveTarget(child.ID)
			}
			cache[id] = total
			return total
		}
		nodeTargetCount := 0
		for i := range result.Nodes {
			value := round2(effectiveTarget(result.Nodes[i].ID))
			if value <= 0 {
				continue
			}
			nodeTargetCount++
			result.Nodes[i].TargetValue = value
			result.Nodes[i].HasTarget = true
			rate := round1(result.Nodes[i].ActualValue / value * 100)
			result.Nodes[i].Rate = &rate
			result.Nodes[i].Status = achievementStatus(rate)
		}
		result.Summary.TargetValue = round2(effectiveTarget(current.ID))
		if mode == "employees" {
			result.Summary.TargetCount = len(targetMap)
		} else {
			result.Summary.TargetCount = nodeTargetCount
			if result.Summary.TargetCount == 0 && result.Summary.TargetValue > 0 {
				result.Summary.TargetCount = 1
			}
		}
	}
	result.Summary.ActualValue = round2(result.Summary.ActualValue)
	result.Summary.TargetValue = round2(result.Summary.TargetValue)
	if result.Summary.TargetValue > 0 {
		result.Summary.Rate = round1(result.Summary.ActualValue / result.Summary.TargetValue * 100)
	}
	result.Summary.NodeCount = len(result.Nodes)
	return result, nil
}

func (s *DrillService) resolveCurrent(requested, ownOrg uint64, scope DataScope, orgs []model.Organization, byID map[uint64]model.Organization, allowed map[uint64]bool) (model.Organization, error) {
	if requested == 0 {
		if scope.All {
			root, err := s.rootOf(orgs, nil)
			if err != nil {
				return model.Organization{}, err
			}
			return root, nil
		}
		if ownOrg != 0 && allowed[ownOrg] {
			requested = ownOrg
		} else {
			// 授权范围场景（如分享链接）：取授权集合内层级最高、且上级不在授权内的组织作为根。
			root, err := s.rootOf(orgs, allowed)
			if err != nil {
				return model.Organization{}, err
			}
			return root, nil
		}
	}
	item, ok := byID[requested]
	if !ok {
		return model.Organization{}, errors.New("组织不存在")
	}
	if !scope.All && !allowed[requested] {
		return model.Organization{}, errors.New("无权查看该组织的数据")
	}
	return item, nil
}

// rootOf 在给定组织集合（可为空表示不限制）中寻找层级最高的根节点。
func (s *DrillService) rootOf(orgs []model.Organization, allowed map[uint64]bool) (model.Organization, error) {
	var root *model.Organization
	for i := range orgs {
		item := orgs[i]
		if allowed != nil && !allowed[item.ID] {
			continue
		}
		if item.ParentID != nil && *item.ParentID != 0 {
			if allowed == nil || allowed[*item.ParentID] {
				continue
			}
		}
		if root == nil || item.Level < root.Level {
			root = &orgs[i]
		}
	}
	if root == nil {
		if allowed != nil {
			return model.Organization{}, errors.New("授权范围内没有可访问的组织")
		}
		return model.Organization{}, errors.New("组织架构为空")
	}
	return *root, nil
}

func (s *DrillService) subtreeIDs(rootID uint64, children map[uint64][]model.Organization, allowed map[uint64]bool) []uint64 {
	result := make([]uint64, 0)
	queue := []uint64{rootID}
	visited := make(map[uint64]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		if !allowed[current] {
			continue
		}
		result = append(result, current)
		for _, child := range children[current] {
			queue = append(queue, child.ID)
		}
	}
	return result
}

func (s *DrillService) storesInSubtree(root model.Organization, children map[uint64][]model.Organization, allowed map[uint64]bool, byID map[uint64]model.Organization) []model.Organization {
	if root.Type == "store" {
		return []model.Organization{root}
	}
	result := make([]model.Organization, 0)
	for _, id := range s.subtreeIDs(root.ID, children, allowed) {
		if org, ok := byID[id]; ok && org.Type == "store" {
			result = append(result, org)
		}
	}
	return result
}

func (s *DrillService) orgNodes(orgs []model.Organization, children map[uint64][]model.Organization, allowed map[uint64]bool, actualByOrg map[uint64]repository.SalesAggregate, metric TargetMetric, startDate, endDate string, byID map[uint64]model.Organization) ([]DrillNode, error) {
	storeIDs := make([]uint64, 0)
	for _, org := range orgs {
		if org.Type == "store" {
			storeIDs = append(storeIDs, org.ID)
		}
	}
	employees, err := s.repo.EmployeesByOrgIDs(storeIDs)
	if err != nil {
		return nil, err
	}
	employeeCounts := make(map[uint64]int)
	for _, item := range employees {
		employeeCounts[item.OrgID]++
	}
	nodes := make([]DrillNode, 0, len(orgs))
	for _, org := range orgs {
		actual := s.aggregateForOrg(org, children, allowed, actualByOrg, metric, startDate, endDate)
		childCount := 0
		for _, child := range children[org.ID] {
			if allowed[child.ID] {
				childCount++
			}
		}
		parentName := ""
		if org.ParentID != nil {
			parentName = byID[*org.ParentID].Name
		}
		nodes = append(nodes, DrillNode{
			ID: org.ID, Name: org.Name, Code: org.Code, Type: org.Type,
			ManagerName: org.ManagerName, ParentID: idValue(org.ParentID), ParentName: parentName,
			ActualValue: round2(actual.ActualValue), Orders: actual.Orders,
			GrossProfit: round2(actual.GrossProfit), Customers: actual.Customers,
			EmployeeCount: employeeCounts[org.ID], ChildCount: childCount,
			HasChildren: childCount > 0 || org.Type == "store",
			Drillable:   childCount > 0 || org.Type == "store",
		})
	}
	return nodes, nil
}

// aggregateForOrg 汇总一个组织及其下级（下级仍受当前用户权限限制）。
func (s *DrillService) aggregateForOrg(org model.Organization, children map[uint64][]model.Organization, allowed map[uint64]bool, actualByOrg map[uint64]repository.SalesAggregate, metric TargetMetric, startDate, endDate string) repository.SalesAggregate {
	if org.Type == "store" {
		return actualByOrg[org.ID]
	}
	ids := s.subtreeIDs(org.ID, children, allowed)
	var result repository.SalesAggregate
	for _, id := range ids {
		row := actualByOrg[id]
		result.ActualValue += row.ActualValue
		result.Orders += row.Orders
		result.GrossProfit += row.GrossProfit
		result.Customers += row.Customers
	}
	return result
}

func (s *DrillService) employeeNodes(current model.Organization, storeIDs []uint64, metric TargetMetric, startDate, endDate string, byID map[uint64]model.Organization) ([]DrillNode, string, error) {
	nodes, _, err := s.employeeNodesForStores([]model.Organization{current}, metric, startDate, endDate, byID)
	return nodes, "employee", err
}

func (s *DrillService) employeeNodesForStores(stores []model.Organization, metric TargetMetric, startDate, endDate string, byID map[uint64]model.Organization) ([]DrillNode, string, error) {
	storeIDs := orgIDsOf(stores)
	employees, err := s.repo.EmployeesByOrgIDs(storeIDs)
	if err != nil {
		return nil, "", err
	}
	employeeIDs := make([]uint64, 0, len(employees))
	employeeByID := make(map[uint64]model.Employee, len(employees))
	for _, item := range employees {
		employeeIDs = append(employeeIDs, item.ID)
		employeeByID[item.ID] = item
	}
	rows, err := s.repo.SalesByEmployee(employeeIDs, startDate, endDate, metric.Expr)
	if err != nil {
		return nil, "", err
	}
	actualByEmployee := make(map[uint64]repository.EmployeeAggregate, len(rows))
	for _, row := range rows {
		actualByEmployee[row.EmployeeID] = row
	}
	nodes := make([]DrillNode, 0, len(employees))
	for _, employee := range employees {
		row := actualByEmployee[employee.ID]
		store := byID[employee.OrgID]
		nodes = append(nodes, DrillNode{
			ID: employee.ID, Name: employee.Name, Code: employee.Code, Type: "employee",
			ManagerName: employee.Position, ParentID: employee.OrgID, ParentName: store.Name,
			ActualValue: round2(row.ActualValue), Orders: row.Orders,
			GrossProfit: round2(row.GrossProfit), Customers: row.Customers,
		})
	}
	return nodes, "employee", nil
}

func (s *DrillService) brief(org model.Organization, children map[uint64][]model.Organization, subtree []uint64) DrillOrgBrief {
	childCount := 0
	for _, child := range children[org.ID] {
		if child.Status != "disabled" {
			childCount++
		}
	}
	return DrillOrgBrief{
		ID: org.ID, Name: org.Name, Code: org.Code, Type: org.Type,
		ManagerName: org.ManagerName, Level: org.Level,
		ChildCount: childCount, EmployeeCount: 0,
	}
}

func (s *DrillService) breadcrumbs(current model.Organization, byID map[uint64]model.Organization) []DrillOrgBrief {
	chain := make([]DrillOrgBrief, 0)
	item := current
	for {
		chain = append(chain, DrillOrgBrief{ID: item.ID, Name: item.Name, Code: item.Code, Type: item.Type, ManagerName: item.ManagerName, Level: item.Level})
		if item.ParentID == nil || *item.ParentID == 0 {
			break
		}
		parent, ok := byID[*item.ParentID]
		if !ok {
			break
		}
		item = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func idValue(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func orgIDsOf(orgs []model.Organization) []uint64 {
	result := make([]uint64, 0, len(orgs))
	for _, item := range orgs {
		result = append(result, item.ID)
	}
	return result
}

func normalizeMetricKey(metric string) string {
	return strings.ToLower(strings.TrimSpace(metric))
}
