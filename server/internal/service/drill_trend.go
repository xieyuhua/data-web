package service

import (
	"errors"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"
)

// DrillTrendPoint 趋势图单日数据。
type DrillTrendPoint struct {
	Date        string  `json:"date"`
	Value       float64 `json:"value"`
	Orders      int     `json:"orders"`
	GrossProfit float64 `json:"gross_profit"`
	Customers   int     `json:"customers"`
}

// DrillTrend 组织 / 员工的销售趋势。
type DrillTrend struct {
	NodeID      uint64            `json:"node_id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	ParentName  string            `json:"parent_name"`
	Metric      string            `json:"metric"`
	MetricLabel string            `json:"metric_label"`
	Unit        string            `json:"unit"`
	StartDate   string            `json:"start_date"`
	EndDate     string            `json:"end_date"`
	Total       float64           `json:"total"`
	Average     float64           `json:"average"`
	Peak        float64           `json:"peak"`
	PeakDate    string            `json:"peak_date"`
	Points      []DrillTrendPoint `json:"points"`
}

// Trend 按登录人权限查看某个组织 / 员工的指标趋势。
func (s *DrillService) Trend(user *model.User, nodeID uint64, nodeType, metric, startDate, endDate string) (*DrillTrend, error) {
	scope, err := s.scope.Resolve(user)
	if err != nil {
		return nil, err
	}
	return s.TrendScoped(scope, user.OrganizationID, nodeID, nodeType, metric, startDate, endDate)
}

// TrendScoped 按显式数据范围查看趋势，供授权分享链接复用。
func (s *DrillService) TrendScoped(scope DataScope, ownOrgID, nodeID uint64, nodeType, metric, startDate, endDate string) (*DrillTrend, error) {
	if nodeID == 0 {
		return nil, errors.New("请选择要查看趋势的组织或员工")
	}
	start, end, err := normalizeTrendRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	metricDef := findMetric(metric)
	if metricDef == nil {
		metricDef = findMetric("revenue")
	}
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

	result := &DrillTrend{
		NodeID: nodeID, Metric: metricDef.Key, MetricLabel: metricDef.Label, Unit: metricDef.Unit,
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
	}
	var rows []repository.DailyAggregate
	if nodeType == "employee" {
		employee, err := s.repo.FindEmployee(nodeID)
		if err != nil {
			return nil, errors.New("员工不存在")
		}
		if !scope.All && !allowed[employee.OrgID] {
			return nil, errors.New("无权查看该员工的数据")
		}
		result.Name = employee.Name
		result.Type = "employee"
		if store, ok := byID[employee.OrgID]; ok {
			result.ParentName = store.Name
		}
		rows, err = s.repo.SalesDailyByEmployee(nodeID, result.StartDate, result.EndDate, metricDef.Expr)
		if err != nil {
			return nil, err
		}
	} else {
		current, err := s.resolveCurrent(nodeID, ownOrgID, scope, orgs, byID, allowed)
		if err != nil {
			return nil, err
		}
		result.Name = current.Name
		result.Type = current.Type
		if current.ParentID != nil {
			result.ParentName = byID[*current.ParentID].Name
		}
		subtree := s.subtreeIDs(current.ID, children, allowed)
		rows, err = s.repo.SalesDailyByOrg(subtree, result.StartDate, result.EndDate, metricDef.Expr)
		if err != nil {
			return nil, err
		}
	}

	fillTrendPoints(result, rows, start, end)
	return result, nil
}

// ScopeTrend 按「一组组织」汇总指标趋势，不要求指定具体节点。
// 看板指标卡在当前登录人的组织范围内查看趋势时复用该方法，避免重复实现按日聚合。
func (s *DrillService) ScopeTrend(orgIDs []uint64, metric, startDate, endDate string) (*DrillTrend, error) {
	start, end, err := normalizeTrendRange(startDate, endDate)
	if err != nil {
		return nil, err
	}
	metricDef := findMetric(metric)
	if metricDef == nil {
		metricDef = findMetric("revenue")
	}
	rows, err := s.repo.SalesDailyByOrg(orgIDs, start.Format("2006-01-02"), end.Format("2006-01-02"), metricDef.Expr)
	if err != nil {
		return nil, err
	}
	result := &DrillTrend{
		Name: "当前数据范围", Type: "org",
		Metric: metricDef.Key, MetricLabel: metricDef.Label, Unit: metricDef.Unit,
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
	}
	fillTrendPoints(result, rows, start, end)
	return result, nil
}

// fillTrendPoints 把按日聚合结果补齐为连续日期序列，并计算合计 / 日均 / 峰值。
// 缺失日期补 0，保证折线图 X 轴连续，避免节假日断档造成误读。
func fillTrendPoints(result *DrillTrend, rows []repository.DailyAggregate, start, end time.Time) {
	rowMap := make(map[string]repository.DailyAggregate, len(rows))
	for _, row := range rows {
		rowMap[row.StatDate] = row
	}
	points := make([]DrillTrendPoint, 0, int(end.Sub(start).Hours()/24)+1)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		row := rowMap[key]
		point := DrillTrendPoint{
			Date: key, Value: round2(row.ActualValue), Orders: row.Orders,
			GrossProfit: round2(row.GrossProfit), Customers: row.Customers,
		}
		if point.Value > result.Peak {
			result.Peak = point.Value
			result.PeakDate = point.Date
		}
		result.Total += point.Value
		points = append(points, point)
	}
	result.Points = points
	result.Total = round2(result.Total)
	result.Peak = round2(result.Peak)
	if len(points) > 0 {
		result.Average = round2(result.Total / float64(len(points)))
	}
}

func normalizeTrendRange(startDate, endDate string) (time.Time, time.Time, error) {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.AddDate(0, 0, -6)
	if value := startDate; value != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
			start = parsed
		}
	}
	if value := endDate; value != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
			end = parsed
		}
	}
	if start.After(end) {
		start, end = end, start
	}
	if end.Sub(start) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("趋势查询时间跨度不能超过 366 天")
	}
	return start, end, nil
}
