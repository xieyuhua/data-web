package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/query"
	"biaoge-bi/internal/repository"
)

type DashboardService struct {
	repo      *repository.Repo
	charts    *ChartService
	datasets  *DatasetService
	executor  *query.Executor
}

func NewDashboardService(repo *repository.Repo, charts *ChartService, datasets *DatasetService, executor *query.Executor) *DashboardService {
	return &DashboardService{repo: repo, charts: charts, datasets: datasets, executor: executor}
}

var (
	ErrDashboardNotFound = errors.New("仪表盘不存在")
	ErrDashboardForbidden = errors.New("无权访问该仪表盘")
)

type DashboardInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *DashboardService) Create(in DashboardInput, userID uint) (*model.Dashboard, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("仪表盘名称不能为空")
	}
	d := model.Dashboard{Name: in.Name, Description: in.Description, OwnerID: userID, CreatedByID: userID, Version: 1}
	if err := s.repo.DB.Create(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *DashboardService) Update(id uint, in DashboardInput) (*model.Dashboard, error) {
	d, err := s.repo.DashboardByID(id)
	if err != nil {
		return nil, err
	}
	d.Name = strings.TrimSpace(in.Name)
	d.Description = in.Description
	d.Version++
	if err := s.repo.DB.Save(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DashboardService) Delete(id uint) error {
	return s.repo.DB.Delete(&model.Dashboard{}, id).Error
}

func (s *DashboardService) List(user *model.User, keyword string, page, pageSize int) ([]model.Dashboard, int64, error) {
	access, err := s.repo.AccessibleDashboardIDs(user.ID, user.IsAdmin())
	if err != nil {
		return nil, 0, err
	}
	// Employees also see dashboards published to their departments.
	if deptDashboards, err := s.deptDashboardIDs(user); err != nil {
		return nil, 0, err
	} else {
		for id := range deptDashboards {
			if _, ok := access[id]; !ok {
				access[id] = "view"
			}
		}
	}
	ids := make([]uint, 0, len(access))
	for id := range access {
		ids = append(ids, id)
	}
	q := s.repo.DB.Model(&model.Dashboard{}).Where("id IN ?", ids)
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	var list []model.Dashboard
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Access returns the permission level for a user: "edit", "view" or "".
func (s *DashboardService) Access(dashboardID uint, user *model.User) (string, error) {
	if user.IsAdmin() {
		return "edit", nil
	}
	d, err := s.repo.DashboardByID(dashboardID)
	if err != nil {
		return "", err
	}
	if d.OwnerID == user.ID || d.CreatedByID == user.ID {
		return "edit", nil
	}
	var g model.DashboardGrant
	err = s.repo.DB.Where("dashboard_id = ? AND user_id = ?", dashboardID, user.ID).First(&g).Error
	if err == nil {
		return g.Permission, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	// Department grants: employees inherit dashboards published to their
	// department or any ancestor department.
	if perm, err := s.deptPermission(dashboardID, user); err != nil {
		return "", err
	} else if perm != "" {
		return perm, nil
	}
	return "", nil
}

// deptPermission resolves a department grant for the user, expanding parent
// departments so a grant on "研发中心" also covers "研发中心/后端组".
func (s *DashboardService) deptPermission(dashboardID uint, user *model.User) (string, error) {
	direct := DirectDeptIDs(user)
	if len(direct) == 0 {
		return "", nil
	}
	// Build the ancestor chain for each of the user's departments.
	ancestors := map[int]bool{}
	type parentRow struct {
		WeComID       int
		ParentWeComID int
	}
	frontier := append([]int(nil), direct...)
	for len(frontier) > 0 {
		for _, id := range frontier {
			ancestors[id] = true
		}
		var rows []parentRow
		if err := s.repo.DB.Model(&model.Department{}).
			Where("we_com_id IN ?", frontier).Find(&rows).Error; err != nil {
			return "", err
		}
		var next []int
		for _, r := range rows {
			if r.ParentWeComID != 0 && !ancestors[r.ParentWeComID] {
				next = append(next, r.ParentWeComID)
			}
		}
		frontier = next
	}
	if len(ancestors) == 0 {
		return "", nil
	}
	ids := make([]int, 0, len(ancestors))
	for id := range ancestors {
		ids = append(ids, id)
	}
	var grants []model.DashboardDeptGrant
	if err := s.repo.DB.Where("dashboard_id = ? AND we_com_dept_id IN ?", dashboardID, ids).
		Find(&grants).Error; err != nil {
		return "", err
	}
	perm := ""
	for _, g := range grants {
		if g.Permission == "edit" {
			return "edit", nil
		}
		perm = "view"
	}
	return perm, nil
}

// deptDashboardIDs returns dashboards granted to any of the user's departments.
func (s *DashboardService) deptDashboardIDs(user *model.User) (map[uint]bool, error) {
	direct := DirectDeptIDs(user)
	if len(direct) == 0 {
		return map[uint]bool{}, nil
	}
	ids := DescendantAndAncestorDeptIDs(s.repo.DB, direct)
	if len(ids) == 0 {
		return map[uint]bool{}, nil
	}
	var dashIDs []uint
	if err := s.repo.DB.Model(&model.DashboardDeptGrant{}).
		Where("we_com_dept_id IN ?", ids).Pluck("dashboard_id", &dashIDs).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]bool, len(dashIDs))
	for _, id := range dashIDs {
		out[id] = true
	}
	return out, nil
}

// ---------- Department grants ----------

type DeptGrantView struct {
	ID          uint      `json:"id"`
	DashboardID uint      `json:"dashboardId"`
	WeComDeptID int       `json:"wecomDeptId"`
	DeptName    string    `json:"deptName"`
	Permission  string    `json:"permission"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (s *DashboardService) ListDeptGrants(dashboardID uint) ([]DeptGrantView, error) {
	var grants []model.DashboardDeptGrant
	if err := s.repo.DB.Where("dashboard_id = ?", dashboardID).Order("id").Find(&grants).Error; err != nil {
		return nil, err
	}
	nameByID := map[int]string{}
	if len(grants) > 0 {
		var depts []model.Department
		_ = s.repo.DB.Find(&depts).Error
		for _, d := range depts {
			nameByID[d.WeComID] = d.Name
		}
	}
	out := make([]DeptGrantView, 0, len(grants))
	for _, g := range grants {
		out = append(out, DeptGrantView{
			ID: g.ID, DashboardID: g.DashboardID, WeComDeptID: g.WeComDeptID,
			DeptName: nameByID[g.WeComDeptID], Permission: g.Permission, CreatedAt: g.CreatedAt,
		})
	}
	return out, nil
}

func (s *DashboardService) AddDeptGrant(dashboardID uint, wecomDeptID int, permission string) error {
	if wecomDeptID == 0 {
		return errors.New("请选择部门")
	}
	if permission != "edit" {
		permission = "view"
	}
	var existing model.DashboardDeptGrant
	err := s.repo.DB.Where("dashboard_id = ? AND we_com_dept_id = ?", dashboardID, wecomDeptID).First(&existing).Error
	if err == nil {
		return s.repo.DB.Model(&existing).Update("permission", permission).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.DB.Create(&model.DashboardDeptGrant{
		DashboardID: dashboardID, WeComDeptID: wecomDeptID, Permission: permission,
	}).Error
}

func (s *DashboardService) RemoveDeptGrant(dashboardID, grantID uint) error {
	return s.repo.DB.Where("id = ? AND dashboard_id = ?", grantID, dashboardID).
		Delete(&model.DashboardDeptGrant{}).Error
}

func (s *DashboardService) Get(dashboardID uint, user *model.User) (*model.Dashboard, error) {
	perm, err := s.Access(dashboardID, user)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDashboardNotFound
		}
		return nil, err
	}
	if perm == "" {
		return nil, ErrDashboardForbidden
	}
	return s.repo.DashboardByID(dashboardID)
}

type LayoutInput struct {
	Widgets []WidgetInput `json:"widgets"`
	Filters []FilterInput `json:"filters"`
}

type WidgetInput struct {
	ID          uint   `json:"id"`
	WidgetType  string `json:"widgetType"`
	ChartID     *uint  `json:"chartId"`
	Title       string `json:"title"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
	OptionsJSON string `json:"optionsJson"`
}

type FilterInput struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Label            string `json:"label"`
	FilterType       string `json:"filterType"`
	DatasetID        *uint  `json:"datasetId"`
	ValueColumn      string `json:"valueColumn"`
	LabelColumn      string `json:"labelColumn"`
	DefaultValueJSON string `json:"defaultValueJson"`
	Multiple         bool   `json:"multiple"`
	X                int    `json:"x"`
	Y                int    `json:"y"`
	W                int    `json:"w"`
	H                int    `json:"h"`
}

type BindingInput struct {
	FilterID   uint   `json:"filterId"`
	WidgetID   uint   `json:"widgetId"`
	ParamName  string `json:"paramName"`
	ValueIndex int    `json:"valueIndex"`
}

// SaveLayout replaces widgets/filters transactionally and returns the refreshed dashboard.
func (s *DashboardService) SaveLayout(dashboardID uint, in LayoutInput) (*model.Dashboard, error) {
	err := s.repo.DB.Transaction(func(tx *gorm.DB) error {
		var d model.Dashboard
		if err := tx.First(&d, dashboardID).Error; err != nil {
			return err
		}

		// Validate chart references up front so a bad id cannot leave a broken layout.
		for _, w := range in.Widgets {
			if w.WidgetType == model.WidgetText || w.ChartID == nil {
				continue
			}
			var n int64
			if err := tx.Model(&model.Chart{}).Where("id = ?", *w.ChartID).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("图表 #%d 不存在", *w.ChartID)
			}
		}

		// Upsert widgets: rows that already belong to this dashboard keep their IDs,
		// so filter bindings survive a layout save. Orphans are deleted.
		keepWidgets := make(map[uint]bool, len(in.Widgets))
		for _, w := range in.Widgets {
			wt := w.WidgetType
			if wt != model.WidgetText {
				wt = model.WidgetChart
			}
			values := map[string]any{
				"dashboard_id": dashboardID, "widget_type": wt, "chart_id": w.ChartID,
				"title": w.Title, "x": w.X, "y": w.Y, "w": nonZero(w.W, 6), "h": nonZero(w.H, 6),
				"options_json": w.OptionsJSON,
			}
			if w.ID != 0 {
				res := tx.Model(&model.DashboardWidget{}).
					Where("id = ? AND dashboard_id = ?", w.ID, dashboardID).Updates(values)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					keepWidgets[w.ID] = true
					continue
				}
			}
			row := model.DashboardWidget{
				DashboardID: dashboardID, WidgetType: wt, ChartID: w.ChartID, Title: w.Title,
				X: w.X, Y: w.Y, W: nonZero(w.W, 6), H: nonZero(w.H, 6), OptionsJSON: w.OptionsJSON,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			keepWidgets[row.ID] = true
		}

		keepFilters := make(map[uint]bool, len(in.Filters))
		for _, f := range in.Filters {
			ft := f.FilterType
			if ft == "" {
				ft = "text"
			}
			values := map[string]any{
				"dashboard_id": dashboardID, "name": f.Name, "label": f.Label, "filter_type": ft,
				"dataset_id": f.DatasetID, "value_column": f.ValueColumn, "label_column": f.LabelColumn,
				"default_value_json": f.DefaultValueJSON, "multiple": f.Multiple,
				"x": f.X, "y": f.Y, "w": nonZero(f.W, 3), "h": nonZero(f.H, 1),
			}
			if f.ID != 0 {
				res := tx.Model(&model.DashboardFilter{}).
					Where("id = ? AND dashboard_id = ?", f.ID, dashboardID).Updates(values)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					keepFilters[f.ID] = true
					continue
				}
			}
			row := model.DashboardFilter{
				DashboardID: dashboardID, Name: f.Name, Label: f.Label, FilterType: ft,
				DatasetID: f.DatasetID, ValueColumn: f.ValueColumn, LabelColumn: f.LabelColumn,
				DefaultValueJSON: f.DefaultValueJSON, Multiple: f.Multiple,
				X: f.X, Y: f.Y, W: nonZero(f.W, 3), H: nonZero(f.H, 1),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			keepFilters[row.ID] = true
		}

		// Remove widgets/filters the client no longer references, plus their bindings.
		var staleWidgets []uint
		if err := tx.Model(&model.DashboardWidget{}).Where("dashboard_id = ?", dashboardID).Pluck("id", &staleWidgets).Error; err != nil {
			return err
		}
		var staleFilters []uint
		if err := tx.Model(&model.DashboardFilter{}).Where("dashboard_id = ?", dashboardID).Pluck("id", &staleFilters).Error; err != nil {
			return err
		}
		for _, id := range staleWidgets {
			if keepWidgets[id] {
				continue
			}
			if err := tx.Where("widget_id = ?", id).Delete(&model.FilterBinding{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&model.DashboardWidget{}, id).Error; err != nil {
				return err
			}
		}
		for _, id := range staleFilters {
			if keepFilters[id] {
				continue
			}
			if err := tx.Where("filter_id = ?", id).Delete(&model.FilterBinding{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&model.DashboardFilter{}, id).Error; err != nil {
				return err
			}
		}
		// Bindings pointing at rows that no longer exist are cleaned defensively.
		if err := tx.Exec(`DELETE FROM filter_bindings WHERE filter_id NOT IN (SELECT id FROM dashboard_filters) OR widget_id NOT IN (SELECT id FROM dashboard_widgets)`).Error; err != nil {
			return err
		}

		d.Version++
		return tx.Model(&model.Dashboard{}).Where("id = ?", dashboardID).Update("version", d.Version).Error
	})
	if err != nil {
		return nil, err
	}
	return s.repo.DashboardByID(dashboardID)
}

// Duplicate copies a dashboard, its widgets, filters and bindings.
func (s *DashboardService) Duplicate(srcID uint, userID uint) (*model.Dashboard, error) {
	src, err := s.repo.DashboardByID(srcID)
	if err != nil {
		return nil, err
	}
	var created model.Dashboard
	err = s.repo.DB.Transaction(func(tx *gorm.DB) error {
		d := model.Dashboard{
			Name: src.Name + "（副本）", Description: src.Description,
			OwnerID: userID, CreatedByID: userID, Version: 1,
		}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		widgetMap := make(map[uint]uint, len(src.Widgets))
		for _, w := range src.Widgets {
			nw := model.DashboardWidget{
				DashboardID: d.ID, WidgetType: w.WidgetType, ChartID: w.ChartID, Title: w.Title,
				X: w.X, Y: w.Y, W: w.W, H: w.H, OptionsJSON: w.OptionsJSON,
			}
			if err := tx.Create(&nw).Error; err != nil {
				return err
			}
			widgetMap[w.ID] = nw.ID
		}
		filterMap := make(map[uint]uint, len(src.Filters))
		for _, f := range src.Filters {
			nf := model.DashboardFilter{
				DashboardID: d.ID, Name: f.Name, Label: f.Label, FilterType: f.FilterType,
				DatasetID: f.DatasetID, ValueColumn: f.ValueColumn, LabelColumn: f.LabelColumn,
				DefaultValueJSON: f.DefaultValueJSON, Multiple: f.Multiple,
				X: f.X, Y: f.Y, W: f.W, H: f.H,
			}
			if err := tx.Create(&nf).Error; err != nil {
				return err
			}
			filterMap[f.ID] = nf.ID
		}
		bindings, err := s.repo.BindingsByDashboard(srcID)
		if err != nil {
			return err
		}
		for _, b := range bindings {
			nb := model.FilterBinding{
				FilterID: filterMap[b.FilterID], WidgetID: widgetMap[b.WidgetID],
				ParamName: b.ParamName, ValueIndex: b.ValueIndex,
			}
			if err := tx.Create(&nb).Error; err != nil {
				return err
			}
		}
		created = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// Bindings returns saved filter → widget parameter bindings.
func (s *DashboardService) Bindings(dashboardID uint) ([]model.FilterBinding, error) {
	return s.repo.BindingsByDashboard(dashboardID)
}

// ---------- Grants ----------

func (s *DashboardService) ListGrants(dashboardID uint, out *[]model.DashboardGrant) error {
	return s.repo.DB.Where("dashboard_id = ?", dashboardID).Order("id").Find(out).Error
}

func (s *DashboardService) AddGrant(dashboardID, userID uint, permission string) error {
	var existing model.DashboardGrant
	err := s.repo.DB.Where("dashboard_id = ? AND user_id = ?", dashboardID, userID).First(&existing).Error
	if err == nil {
		return s.repo.DB.Model(&existing).Update("permission", permission).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	g := model.DashboardGrant{DashboardID: dashboardID, UserID: userID, Permission: permission}
	return s.repo.DB.Create(&g).Error
}

func (s *DashboardService) RemoveGrant(dashboardID, grantID uint) error {
	return s.repo.DB.Where("id = ? AND dashboard_id = ?", grantID, dashboardID).Delete(&model.DashboardGrant{}).Error
}

// SaveBindings stores filter→widget param bindings for a dashboard.
func (s *DashboardService) SaveBindings(dashboardID uint, bindings []BindingInput) error {
	return s.repo.DB.Transaction(func(tx *gorm.DB) error {
		var filterIDs []uint
		if err := tx.Model(&model.DashboardFilter{}).Where("dashboard_id = ?", dashboardID).Pluck("id", &filterIDs).Error; err != nil {
			return err
		}
		if len(filterIDs) > 0 {
			if err := tx.Where("filter_id IN ?", filterIDs).Delete(&model.FilterBinding{}).Error; err != nil {
				return err
			}
		}
		if len(bindings) == 0 {
			return nil
		}
		set := make(map[uint]bool, len(filterIDs))
		for _, id := range filterIDs {
			set[id] = true
		}
		var rows []model.FilterBinding
		for _, b := range bindings {
			if !set[b.FilterID] {
				return errors.New("筛选器不属于该仪表盘")
			}
			if !query.ValidIdentifier(b.ParamName) {
				return errors.New("非法的参数名: " + b.ParamName)
			}
			rows = append(rows, model.FilterBinding{
				FilterID: b.FilterID, WidgetID: b.WidgetID, ParamName: b.ParamName, ValueIndex: b.ValueIndex,
			})
		}
		return tx.Create(&rows).Error
	})
}

func nonZero(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// ---------- Data execution ----------

type FilterValues map[string]any // filter name -> scalar or [from,to]

type DashboardDataRequest struct {
	FilterValues FilterValues `json:"filterValues"`
}

type WidgetResult struct {
	Result *query.ResultSet `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type DashboardData struct {
	Widgets   map[string]WidgetResult `json:"widgets"`
	ElapsedMs int64                   `json:"elapsedMs"`
}

var numRe = regexp.MustCompile(`^\d+$`)

func (s *DashboardService) ExecuteData(ctx context.Context, dashboardID uint, req DashboardDataRequest) (*DashboardData, error) {
	d, err := s.repo.DashboardByID(dashboardID)
	if err != nil {
		return nil, ErrDashboardNotFound
	}
	bindings, err := s.repo.BindingsByDashboard(dashboardID)
	if err != nil {
		return nil, err
	}

	// widgetID -> param values derived from filter bindings
	paramByWidget := make(map[uint]map[string]string)
	// filter lookup by ID
	filterByID := make(map[uint]model.DashboardFilter, len(d.Filters))
	for _, f := range d.Filters {
		filterByID[f.ID] = f
	}
	for _, b := range bindings {
		f, ok := filterByID[b.FilterID]
		if !ok {
			continue
		}
		val, ok := lookupFilterValue(f, req.FilterValues, b.ValueIndex)
		if !ok {
			continue
		}
		if paramByWidget[b.WidgetID] == nil {
			paramByWidget[b.WidgetID] = map[string]string{}
		}
		paramByWidget[b.WidgetID][b.ParamName] = val
	}

	start := time.Now()
	result := DashboardData{Widgets: make(map[string]WidgetResult)}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for _, w := range d.Widgets {
		if w.WidgetType != model.WidgetChart || w.ChartID == nil {
			continue
		}
		wg.Add(1)
		go func(w model.DashboardWidget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			wc, err := s.charts.Get(*w.ChartID)
			var wr WidgetResult
			if err != nil {
				wr.Error = "图表不存在"
			} else {
				rs, err := s.charts.Execute(ctx, wc, paramByWidget[w.ID])
				if err != nil {
					wr.Error = err.Error()
				} else {
					wr.Result = rs
				}
			}
			mu.Lock()
			result.Widgets[uintToString(w.ID)] = wr
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	result.ElapsedMs = time.Since(start).Milliseconds()
	return &result, nil
}

// KeyMetricsSummary returns short "label: value" lines for the first few KPI
// widgets, used as the body of a WeCom push message.
func (s *DashboardService) KeyMetricsSummary(ctx context.Context, dashboardID uint) ([]string, error) {
	d, err := s.repo.DashboardByID(dashboardID)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, w := range d.Widgets {
		if w.WidgetType != model.WidgetChart || w.ChartID == nil {
			continue
		}
		ch, err := s.charts.Get(*w.ChartID)
		if err != nil || ch.Type != model.ChartKPI {
			continue
		}
		rs, err := s.charts.Execute(ctx, ch, nil)
		if err != nil || rs == nil || len(rs.Rows) == 0 || len(rs.Rows[0]) == 0 {
			continue
		}
		cfg, err := s.charts.Config(ch)
		if err != nil {
			continue
		}
		idx := len(cfg.Dimensions) + len(cfg.Metrics) - 1
		if idx < 0 || idx >= len(rs.Rows[0]) {
			idx = 0
		}
		label := w.Title
		if label == "" {
			label = ch.Name
		}
		lines = append(lines, fmt.Sprintf("%s：%s", label, formatMetricValue(rs.Rows[0][idx], rs.Columns, idx)))
		if len(lines) >= 5 {
			break
		}
	}
	return lines, nil
}

// formatMetricValue renders a KPI value using its column format hint.
func formatMetricValue(v any, cols []model.ColumnMeta, idx int) string {
	if v == nil {
		return "-"
	}
	f, ok := v.(float64)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	format := ""
	if idx >= 0 && idx < len(cols) {
		format = cols[idx].Format
	}
	switch format {
	case "percent":
		return fmt.Sprintf("%.2f%%", f)
	case "currency":
		return fmt.Sprintf("¥%.2f", f)
	default:
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
}

// ---------- Public (share token) view ----------

type PublicWidget struct {
	ID         uint   `json:"id"`
	WidgetType string `json:"widgetType"`
	ChartID    *uint  `json:"chartId"`
	Title      string `json:"title"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	W          int    `json:"w"`
	H          int    `json:"h"`
	OptionsJSON string `json:"optionsJson"`
	ChartType  string `json:"chartType,omitempty"`
	ChartConfig string `json:"chartConfig,omitempty"` // chart display config only
}

type PublicFilter struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Label        string `json:"label"`
	FilterType   string `json:"filterType"`
	HasOptions   bool   `json:"hasOptions"`
	Multiple     bool   `json:"multiple"`
	DefaultValue string `json:"defaultValueJson"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	W            int    `json:"w"`
	H            int    `json:"h"`
}

type DashboardPublicView struct {
	Name    string         `json:"name"`
	Widgets []PublicWidget `json:"widgets"`
	Filters []PublicFilter `json:"filters"`
	Bindings []model.FilterBinding `json:"bindings"`
}

// PublicView resolves a share token and returns a sanitized dashboard definition.
func (s *DashboardService) PublicView(token string, share *ShareService) (*DashboardPublicView, error) {
	d, err := share.Resolve(token)
	if err != nil {
		return nil, err
	}
	view := &DashboardPublicView{Name: d.Name}
	for _, w := range d.Widgets {
		pw := PublicWidget{
			ID: w.ID, WidgetType: w.WidgetType, ChartID: w.ChartID, Title: w.Title,
			X: w.X, Y: w.Y, W: w.W, H: w.H, OptionsJSON: w.OptionsJSON,
		}
		if w.ChartID != nil {
			if ch, err := s.charts.Get(*w.ChartID); err == nil {
				pw.ChartType = ch.Type
				pw.ChartConfig = ch.ConfigJSON
			}
		}
		view.Widgets = append(view.Widgets, pw)
	}
	for _, f := range d.Filters {
		view.Filters = append(view.Filters, PublicFilter{
			ID: f.ID, Name: f.Name, Label: f.Label, FilterType: f.FilterType,
			HasOptions: f.DatasetID != nil, Multiple: f.Multiple, DefaultValue: f.DefaultValueJSON,
			X: f.X, Y: f.Y, W: f.W, H: f.H,
		})
	}
	if view.Widgets == nil {
		view.Widgets = []PublicWidget{}
	}
	if view.Filters == nil {
		view.Filters = []PublicFilter{}
	}
	view.Bindings, _ = s.repo.BindingsByDashboard(d.ID)
	if view.Bindings == nil {
		view.Bindings = []model.FilterBinding{}
	}
	return view, nil
}

// lookupFilterValue extracts the value for a filter. valueIndex selects the
// start (0) or end (1) element of a date range; multi-select arrays bind as a
// JSON list so dataset `list` params expand into an IN clause.
func lookupFilterValue(f model.DashboardFilter, values FilterValues, valueIndex int) (string, bool) {
	v, ok := values[f.Name]
	if !ok || v == nil {
		if f.DefaultValueJSON == "" {
			return "", false
		}
		var def any
		if err := json.Unmarshal([]byte(f.DefaultValueJSON), &def); err != nil {
			return "", false
		}
		v = def
	}
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			return "", false
		}
		// A range filter binds its two ends; a multi-select binds the whole list.
		if f.FilterType == "daterange" {
			if valueIndex >= len(x) {
				return "", false
			}
			return toString(x[valueIndex]), true
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if item != nil {
				parts = append(parts, toString(item))
			}
		}
		if len(parts) == 0 {
			return "", false
		}
		raw, _ := json.Marshal(parts)
		return string(raw), true
	case []string:
		if len(x) == 0 {
			return "", false
		}
		raw, _ := json.Marshal(x)
		return string(raw), true
	case string:
		return x, x != ""
	case float64:
		return trimFloat(x), true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

// FilterOptions returns distinct values for a select-type dashboard filter.
func (s *DashboardService) FilterOptions(ctx context.Context, dashboardID uint, filterID uint) ([]map[string]string, error) {
	var f model.DashboardFilter
	if err := s.repo.DB.Where("dashboard_id = ? AND id = ?", dashboardID, filterID).First(&f).Error; err != nil {
		return nil, errors.New("筛选器不存在")
	}
	if f.DatasetID == nil {
		return nil, errors.New("该筛选器未配置选项数据集")
	}
	valueCol := f.ValueColumn
	labelCol := f.LabelColumn
	if valueCol == "" {
		valueCol = "value"
	}
	if labelCol == "" {
		labelCol = valueCol
	}
	if !query.ValidIdentifier(valueCol) || !query.ValidIdentifier(labelCol) {
		return nil, errors.New("筛选器列名不合法")
	}
	ds, err := s.repo.DatasetByID(*f.DatasetID)
	if err != nil {
		return nil, errors.New("筛选器数据集不存在")
	}
	dialect, err := s.charts.Dialect(ctx, ds)
	if err != nil {
		return nil, err
	}
	vq, err := query.QuoteIdentifier(valueCol, dialect)
	if err != nil {
		return nil, err
	}
	lq, err := query.QuoteIdentifier(labelCol, dialect)
	if err != nil {
		return nil, err
	}
	sqlText, args, err := query.BindParams(ds.SQLText, query.SpecsFromModel(ds.Params), map[string]string{}, dialect)
	if err != nil {
		return nil, err
	}
	built := "SELECT DISTINCT " + vq + " AS value, " + lq + " AS label FROM (" + sqlText + ") AS bi_d ORDER BY 1 LIMIT 500"
	rs, err := s.executor.ExecuteRaw(ctx, ds.DataSourceID, built, args, 500, 15*time.Second)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(rs.Rows))
	for _, row := range rs.Rows {
		if len(row) < 2 || row[0] == nil {
			continue
		}
		out = append(out, map[string]string{"value": stringify(row[0]), "label": stringify(row[1])})
	}
	return out, nil
}

func toString(v any) string { return stringify(v) }

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return trimFloat(x)
	default:
		b, _ := json.Marshal(x)
		return strings.Trim(string(b), `"`)
	}
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
