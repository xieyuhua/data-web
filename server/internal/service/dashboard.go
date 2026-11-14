package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/sqlguard"
	"bi-platform/internal/repository"

	"gorm.io/gorm"
)

var filterKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)

// DashboardWidget 看板上的单个组件（图表 / 表格 / 指标卡）。
// X/Y/W/H 为 12 栅格布局坐标，Config 保存指标映射、单位等可视化配置。
type DashboardWidget struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Subtitle  string         `json:"subtitle"`
	ChartType string         `json:"chart_type"`
	DatasetID uint64         `json:"dataset_id"`
	X         int            `json:"x"`
	Y         int            `json:"y"`
	W         int            `json:"w"`
	H         int            `json:"h"`
	Config    map[string]any `json:"config"`
}

// DashboardLayout 看板布局：全局设置 + 组件列表。
type DashboardLayout struct {
	Settings map[string]any    `json:"settings"`
	Widgets  []DashboardWidget `json:"widgets"`
}

// DashboardFilter 是看板级全局筛选器定义，阅读页据此渲染筛选栏。
// 数据集 SQL 通过 {{filter:key}} 占位符接收筛选值。
type DashboardFilter struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Options     []string `json:"options"`
	Default     string   `json:"default"`
	Placeholder string   `json:"placeholder"`
	HideOnShare bool     `json:"hide_on_share"`
}

// DashboardView 看板对外视图结构。
type DashboardView struct {
	ID             uint64          `json:"id"`
	Name           string          `json:"name"`
	Code           string          `json:"code"`
	Description    string          `json:"description"`
	Layout         json.RawMessage `json:"layout"`
	Theme          string          `json:"theme"`
	Status         string          `json:"status"`
	Category       string          `json:"category"`
	CreatedBy      uint64          `json:"created_by"`
	VisibilityMode string          `json:"visibility_mode"`
	VisibleRoles   []string        `json:"visible_roles"`
	VisibleUserIDs []uint64        `json:"visible_user_ids"`
	VisibleOrgIDs  []uint64        `json:"visible_org_ids"`
	ScopeMode      string          `json:"scope_mode"`
	DrillEnabled   bool            `json:"drill_enabled"`
	DrillMode      string          `json:"drill_mode"`
	PublishedAt    *time.Time      `json:"published_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// DashboardInput 新建 / 更新看板的入参。
type DashboardInput struct {
	Name           string          `json:"name"`
	Code           string          `json:"code"`
	Description    string          `json:"description"`
	Layout         json.RawMessage `json:"layout"`
	Theme          string          `json:"theme"`
	Status         string          `json:"status"`
	Category       string          `json:"category"`
	VisibilityMode string          `json:"visibility_mode"`
	VisibleRoles   []string        `json:"visible_roles"`
	VisibleUserIDs []uint64        `json:"visible_user_ids"`
	VisibleOrgIDs  []uint64        `json:"visible_org_ids"`
	ScopeMode      string          `json:"scope_mode"`
	DrillEnabled   *bool           `json:"drill_enabled"`
	DrillMode      string          `json:"drill_mode"`
}

// DatasetDashboardInput 由数据集一键生成看板的入参。
type DatasetDashboardInput struct {
	Name        string           `json:"name"`
	Code        string           `json:"code"`
	Category    string           `json:"category"`
	Theme       string           `json:"theme"`
	Title       string           `json:"title"`
	Subtitle    string           `json:"subtitle"`
	ChartType   string           `json:"chart_type"`
	XField      string           `json:"x_field"`
	YField      string           `json:"y_field"`
	RateField   string           `json:"rate_field"`
	ActualField string           `json:"actual_field"`
	TargetField string           `json:"target_field"`
	Unit        string           `json:"unit"`
	Publish     bool             `json:"publish"`
	Metrics     []map[string]any `json:"metrics"`
}

// WidgetQueryResult 单个组件的查询结果；单个组件失败只影响自身。
type WidgetQueryResult struct {
	WidgetID  string         `json:"widget_id"`
	Title     string         `json:"title"`
	ChartType string         `json:"chart_type"`
	Config    map[string]any `json:"config"`
	Data      *QueryResult   `json:"data,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// DashboardData 整块看板的查询结果。
type DashboardData struct {
	StartDate string              `json:"start_date"`
	EndDate   string              `json:"end_date"`
	Widgets   []WidgetQueryResult `json:"widgets"`
}

// DashboardService 看板业务服务。
type DashboardService struct {
	repo        *repository.DashboardRepository
	datasetRepo *repository.DatasetRepository
	dataset     *DatasetService
}

// NewDashboardService 创建看板服务。
func NewDashboardService(repo *repository.DashboardRepository, datasetRepo *repository.DatasetRepository, dataset *DatasetService) *DashboardService {
	return &DashboardService{repo: repo, datasetRepo: datasetRepo, dataset: dataset}
}

// List 查询看板列表。
func (s *DashboardService) List(keyword, category string) ([]DashboardView, error) {
	items, err := s.repo.List(keyword, category)
	if err != nil {
		return nil, err
	}
	result := make([]DashboardView, 0, len(items))
	for _, item := range items {
		result = append(result, toDashboardView(item))
	}
	return result, nil
}

// Get 查询看板详情。
func (s *DashboardService) Get(id uint64) (*DashboardView, error) {
	item, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	view := toDashboardView(*item)
	return &view, nil
}

// Create 新建看板。
func (s *DashboardService) Create(input DashboardInput, userID uint64) (*DashboardView, error) {
	item, err := s.fromInput(input)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = userID
	now := time.Now()
	if item.Status == "published" {
		item.PublishedAt = &now
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	return s.Get(item.ID)
}

// Update 更新看板。
func (s *DashboardService) Update(id uint64, input DashboardInput) (*DashboardView, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	item, err := s.fromInput(input)
	if err != nil {
		return nil, err
	}
	item.ID = id
	item.CreatedBy = existing.CreatedBy
	item.CreatedAt = existing.CreatedAt
	if item.Status == "published" && existing.PublishedAt == nil {
		now := time.Now()
		item.PublishedAt = &now
	} else {
		item.PublishedAt = existing.PublishedAt
	}
	if err := s.repo.Update(item); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Delete 删除看板。
func (s *DashboardService) Delete(id uint64) error { return s.repo.Delete(id) }

// Duplicate 复制看板（生成新的编码并归属指定用户）。
func (s *DashboardService) Duplicate(id uint64, userID uint64) (*DashboardView, error) {
	source, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	suffix := strconv.FormatInt(time.Now().Unix(), 10)
	item := *source
	item.ID = 0
	item.Name = source.Name + "（副本）"
	item.Code = source.Code + "_copy_" + suffix
	item.Status = "draft"
	item.CreatedBy = userID
	item.PublishedAt = nil
	item.CreatedAt = time.Time{}
	item.UpdatedAt = time.Time{}
	if err := s.repo.Create(&item); err != nil {
		return nil, err
	}
	return s.Get(item.ID)
}

// CreateFromDataset 用 SQL 数据集一键生成单组件看板。
func (s *DashboardService) CreateFromDataset(datasetID uint64, input DatasetDashboardInput, userID uint64) (*DashboardView, error) {
	dataset, err := s.datasetRepo.FindByID(datasetID)
	if err != nil {
		return nil, errors.New("数据集不存在")
	}
	chartType := normalizeDatasetChartType(input.ChartType)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = dataset.Name + "看板"
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		code = "dash_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	config := map[string]any{}
	if input.XField != "" {
		config["xField"] = input.XField
	}
	if input.YField != "" {
		config["yField"] = input.YField
		config["yFields"] = []map[string]any{{"field": input.YField, "name": input.YField, "type": seriesTypeForChart(chartType)}}
	}
	if input.RateField != "" {
		config["rateField"] = input.RateField
	}
	if input.ActualField != "" {
		config["actualField"] = input.ActualField
	}
	if input.TargetField != "" {
		config["targetField"] = input.TargetField
	}
	if input.Unit != "" {
		config["unit"] = input.Unit
	}
	if len(input.Metrics) > 0 {
		config["columns"] = input.Metrics
	}
	widgetTitle := strings.TrimSpace(input.Title)
	if widgetTitle == "" {
		widgetTitle = dataset.Name
	}
	layout := DashboardLayout{
		Settings: map[string]any{"refresh_seconds": 0, "show_title": true},
		Widgets: []DashboardWidget{{
			ID: "widget_1", Title: widgetTitle, Subtitle: input.Subtitle,
			ChartType: chartType, DatasetID: datasetID,
			X: 0, Y: 0, W: 12, H: 8, Config: config,
		}},
	}
	raw, err := json.Marshal(layout)
	if err != nil {
		return nil, err
	}
	status := "draft"
	if input.Publish {
		status = "published"
	}
	return s.Create(DashboardInput{
		Name: name, Code: code, Description: dataset.Description, Layout: raw,
		Theme: input.Theme, Status: status, Category: input.Category,
	}, userID)
}

func (s *DashboardService) fromInput(input DashboardInput) (*model.Dashboard, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Code) == "" {
		return nil, errors.New("看板名称和编码不能为空")
	}
	var layout DashboardLayout
	if len(input.Layout) == 0 {
		input.Layout = json.RawMessage(`{"settings":{},"widgets":[]}`)
	}
	if err := json.Unmarshal(input.Layout, &layout); err != nil {
		return nil, errors.New("看板布局 JSON 格式不正确")
	}
	if len(layout.Widgets) > 30 {
		return nil, errors.New("单个看板最多支持 30 个组件")
	}
	if layout.Settings == nil {
		layout.Settings = map[string]any{}
	}
	filters, err := normalizeDashboardFilters(layout.Settings["filters"])
	if err != nil {
		return nil, err
	}
	if len(filters) > 0 {
		layout.Settings["filters"] = filters
	} else {
		delete(layout.Settings, "filters")
	}
	normalizeWidgetFilterLinks(layout.Widgets, filters)
	normalized, _ := json.Marshal(layout)
	theme := input.Theme
	if theme == "" {
		theme = "light"
	}
	status := input.Status
	if status == "" {
		status = "draft"
	}
	if status != "draft" && status != "published" {
		return nil, errors.New("看板状态不正确")
	}
	category := input.Category
	if category == "" {
		category = "经营分析"
	}
	visibilityMode := normalizeVisibilityMode(input.VisibilityMode)
	scopeMode := strings.TrimSpace(input.ScopeMode)
	if scopeMode != "all" {
		scopeMode = "role_org"
	}
	drillMode := strings.TrimSpace(input.DrillMode)
	if drillMode != "all" && drillMode != "employees" {
		drillMode = "level"
	}
	drillEnabled := true
	if input.DrillEnabled != nil {
		drillEnabled = *input.DrillEnabled
	}
	visibleRoles := normalizeRoleCodes(input.VisibleRoles)
	visibleUsers := normalizeIDList(input.VisibleUserIDs)
	visibleOrgs := normalizeIDList(input.VisibleOrgIDs)
	return &model.Dashboard{
		Name: input.Name, Code: strings.TrimSpace(input.Code), Description: input.Description,
		Layout: string(normalized), Theme: theme, Status: status, Category: category,
		VisibilityMode: visibilityMode,
		VisibleRoles:   marshalJSON(visibleRoles),
		VisibleUsers:   marshalJSON(visibleUsers),
		VisibleOrgs:    marshalJSON(visibleOrgs),
		ScopeMode:      scopeMode,
		DrillEnabled:   drillEnabled,
		DrillMode:      drillMode,
	}, nil
}

// normalizeDashboardFilters 校验并清洗全局筛选器配置，限制数量与字段长度。
func normalizeDashboardFilters(value any) ([]DashboardFilter, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("全局筛选器格式不正确")
	}
	var filters []DashboardFilter
	if err := json.Unmarshal(raw, &filters); err != nil {
		return nil, errors.New("全局筛选器格式不正确")
	}
	if len(filters) > 8 {
		return nil, errors.New("单个看板最多配置 8 个全局筛选器")
	}
	seen := make(map[string]bool, len(filters))
	result := make([]DashboardFilter, 0, len(filters))
	for _, item := range filters {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		if !filterKeyPattern.MatchString(key) {
			return nil, errors.New("筛选器字段名只能使用字母、数字与下划线，且不超过 32 个字符")
		}
		if seen[key] {
			return nil, errors.New("筛选器字段名不能重复: " + key)
		}
		seen[key] = true
		label := strings.TrimSpace(item.Label)
		if label == "" {
			label = key
		}
		if len([]rune(label)) > 40 {
			return nil, errors.New("筛选器名称不能超过 40 个字符")
		}
		filterType := strings.ToLower(strings.TrimSpace(item.Type))
		switch filterType {
		case "select", "multiselect", "date":
		default:
			filterType = "input"
		}
		options := make([]string, 0, len(item.Options))
		optionSeen := make(map[string]bool, len(item.Options))
		for _, option := range item.Options {
			option = strings.TrimSpace(option)
			if option == "" || optionSeen[option] {
				continue
			}
			if len([]rune(option)) > 60 {
				return nil, errors.New("筛选项不能超过 60 个字符")
			}
			optionSeen[option] = true
			options = append(options, option)
		}
		if len(options) > 50 {
			return nil, errors.New("单个筛选器最多 50 个选项")
		}
		defaultValue := strings.TrimSpace(item.Default)
		if len([]rune(defaultValue)) > 200 {
			return nil, errors.New("筛选器默认值不能超过 200 个字符")
		}
		placeholder := strings.TrimSpace(item.Placeholder)
		if len([]rune(placeholder)) > 60 {
			placeholder = string([]rune(placeholder)[:60])
		}
		result = append(result, DashboardFilter{
			Key: key, Label: label, Type: filterType, Options: options,
			Default: defaultValue, Placeholder: placeholder, HideOnShare: item.HideOnShare,
		})
	}
	return result, nil
}

// sanitizeFilterValues 只允许看板定义过的筛选器参与查询，并按类型校验取值。
// 分享模式下会丢弃配置为不在分享页显示的筛选器，避免通过 URL 绕过展示开关。
func sanitizeFilterValues(defs []DashboardFilter, values map[string]string, shareMode bool) map[string]string {
	if len(values) == 0 || len(defs) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for _, def := range defs {
		if shareMode && def.HideOnShare {
			continue
		}
		raw, ok := values[def.Key]
		if !ok {
			continue
		}
		switch def.Type {
		case "select":
			if optionAllowed(def.Options, raw) {
				result[def.Key] = raw
			}
		case "multiselect":
			selected := make([]string, 0, 4)
			for _, item := range splitFilterValue(raw) {
				if optionAllowed(def.Options, item) {
					selected = append(selected, item)
				}
			}
			if len(selected) > 0 {
				result[def.Key] = joinFilterValue(selected)
			}
		case "date":
			if _, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil {
				result[def.Key] = strings.TrimSpace(raw)
			}
		default:
			if strings.TrimSpace(raw) != "" {
				result[def.Key] = raw
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func optionAllowed(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func splitFilterValue(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, sqlguard.FilterValueSeparator)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func joinFilterValue(values []string) string {
	return strings.Join(values, sqlguard.FilterValueSeparator)
}

// normalizeWidgetFilterLinks 清理组件上失效的联动筛选器配置。
func normalizeWidgetFilterLinks(widgets []DashboardWidget, filters []DashboardFilter) {
	allowed := make(map[string]bool, len(filters))
	for _, item := range filters {
		allowed[item.Key] = true
	}
	for index := range widgets {
		config := widgets[index].Config
		if config == nil {
			continue
		}
		value, ok := config["linkFilter"].(string)
		if !ok {
			continue
		}
		key := strings.TrimSpace(value)
		if key == "" || !allowed[key] {
			delete(config, "linkFilter")
			continue
		}
		config["linkFilter"] = key
	}
}

// Query 并发执行看板内全部组件的查询，并注入组织数据权限与全局筛选。
func (s *DashboardService) Query(ctx context.Context, dashboardID uint64, startDate, endDate string, orgIDs []uint64, options QueryOptions) (*DashboardData, error) {
	dashboard, err := s.repo.FindByID(dashboardID)
	if err != nil {
		return nil, err
	}
	var layout DashboardLayout
	if err := json.Unmarshal([]byte(dashboard.Layout), &layout); err != nil {
		return nil, errors.New("看板布局数据损坏")
	}
	var filterDefs []DashboardFilter
	if raw, ok := layout.Settings["filters"]; ok {
		if encoded, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(encoded, &filterDefs)
		}
	}
	// 只允许看板已配置的筛选器参与 SQL 查询，并按类型/选项清洗取值。
	options.Filters = sanitizeFilterValues(filterDefs, options.Filters, options.ShareMode)
	startDate, endDate = normalizeDateRange(startDate, endDate)
	result := &DashboardData{StartDate: startDate, EndDate: endDate, Widgets: make([]WidgetQueryResult, 0, len(layout.Widgets))}
	result.Widgets = make([]WidgetQueryResult, len(layout.Widgets))
	// 组件查询并发执行（最多 4 个），避免多图表看板串行等待；结果仍按布局顺序返回。
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	for index, widget := range layout.Widgets {
		result.Widgets[index] = WidgetQueryResult{
			WidgetID: widget.ID, Title: widget.Title, ChartType: widget.ChartType, Config: widget.Config,
		}
		if widget.ChartType == "text" || widget.DatasetID == 0 {
			continue
		}
		wait.Add(1)
		go func(index int, widget DashboardWidget) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			item := WidgetQueryResult{
				WidgetID: widget.ID, Title: widget.Title, ChartType: widget.ChartType, Config: widget.Config,
			}
			dataset, err := s.datasetRepo.FindByID(widget.DatasetID)
			if err != nil {
				item.Error = "关联数据集不存在"
				result.Widgets[index] = item
				return
			}
			data, err := s.dataset.QueryWithOptions(ctx, dataset, orgIDs, startDate, endDate, options)
			if err != nil {
				item.Error = err.Error()
			} else {
				item.Data = data
			}
			result.Widgets[index] = item
		}(index, widget)
	}
	wait.Wait()
	return result, nil
}

func toDashboardView(item model.Dashboard) DashboardView {
	layout := json.RawMessage(item.Layout)
	if len(layout) == 0 || !json.Valid(layout) {
		layout = json.RawMessage(`{"settings":{},"widgets":[]}`)
	}
	visibilityMode := normalizeVisibilityMode(item.VisibilityMode)
	scopeMode := item.ScopeMode
	if scopeMode != "all" {
		scopeMode = "role_org"
	}
	drillMode := item.DrillMode
	if drillMode != "all" && drillMode != "employees" {
		drillMode = "level"
	}
	return DashboardView{
		ID: item.ID, Name: item.Name, Code: item.Code, Description: item.Description,
		Layout: layout, Theme: item.Theme, Status: item.Status, Category: item.Category,
		VisibilityMode: visibilityMode,
		VisibleRoles:   parseStringList(item.VisibleRoles),
		VisibleUserIDs: parseUintList(item.VisibleUsers),
		VisibleOrgIDs:  parseUintList(item.VisibleOrgs),
		ScopeMode:      scopeMode,
		DrillEnabled:   item.DrillEnabled,
		DrillMode:      drillMode,
		CreatedBy:      item.CreatedBy, PublishedAt: item.PublishedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// normalizeDatasetChartType 统一校验数据集一键生成看板时的图表类型。
func normalizeDatasetChartType(value string) string {
	switch strings.TrimSpace(value) {
	case "kpi", "line", "area", "bar", "bar_h", "bar_stacked", "combo", "scatter", "donut", "table", "gauge":
		return strings.TrimSpace(value)
	default:
		return "bar"
	}
}

func seriesTypeForChart(chartType string) string {
	if chartType == "line" || chartType == "area" {
		return "line"
	}
	return "bar"
}

func normalizeVisibilityMode(value string) string {
	switch strings.TrimSpace(value) {
	case "roles", "users", "orgs", "custom":
		return strings.TrimSpace(value)
	default:
		return "all"
	}
}

func normalizeRoleCodes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		code := strings.TrimSpace(value)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		result = append(result, code)
	}
	return result
}

func normalizeIDList(values []uint64) []uint64 {
	result := make([]uint64, 0, len(values))
	seen := make(map[uint64]bool)
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func marshalJSON(values any) string {
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func parseStringList(value string) []string {
	result := make([]string, 0)
	if strings.TrimSpace(value) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return make([]string, 0)
	}
	return result
}

func parseUintList(value string) []uint64 {
	result := make([]uint64, 0)
	if strings.TrimSpace(value) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return make([]uint64, 0)
	}
	return result
}

func normalizeDateRange(startDate, endDate string) (string, string) {
	now := time.Now()
	if endDate == "" {
		endDate = now.Format("2006-01-02")
	}
	if startDate == "" {
		startDate = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	start, errStart := time.Parse("2006-01-02", startDate)
	end, errEnd := time.Parse("2006-01-02", endDate)
	if errStart != nil {
		startDate = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	if errEnd != nil {
		endDate = now.Format("2006-01-02")
	}
	if errStart == nil && errEnd == nil && start.After(end) {
		startDate, endDate = endDate, startDate
	}
	return startDate, endDate
}

// TemplateView 模板对外视图。
type TemplateView struct {
	ID          uint64          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Cover       string          `json:"cover"`
	Layout      json.RawMessage `json:"layout"`
	Theme       string          `json:"theme"`
	BuiltIn     bool            `json:"built_in"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// TemplateService 看板模板服务。
type TemplateService struct {
	repo      *repository.TemplateRepository
	dashboard *repository.DashboardRepository
}

// NewTemplateService 创建模板服务。
func NewTemplateService(repo *repository.TemplateRepository, dashboard *repository.DashboardRepository) *TemplateService {
	return &TemplateService{repo: repo, dashboard: dashboard}
}

// List 查询模板列表。
func (s *TemplateService) List() ([]TemplateView, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make([]TemplateView, 0, len(items))
	for _, item := range items {
		result = append(result, toTemplateView(item))
	}
	return result, nil
}

// Create 将布局保存为模板。
func (s *TemplateService) Create(name, description, category, theme, cover string, layout json.RawMessage, userID uint64) (*TemplateView, error) {
	if name == "" || len(layout) == 0 {
		return nil, errors.New("模板名称和布局不能为空")
	}
	var payload DashboardLayout
	if err := json.Unmarshal(layout, &payload); err != nil {
		return nil, errors.New("模板布局格式不正确")
	}
	item := &model.Template{Name: name, Description: description, Category: category, Theme: theme, Cover: cover, Layout: string(layout), CreatedBy: userID}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	view := toTemplateView(*item)
	return &view, nil
}

// CreateDashboard 基于模板创建看板。
func (s *TemplateService) CreateDashboard(templateID, userID uint64) (*DashboardView, error) {
	template, err := s.repo.FindByID(templateID)
	if err != nil {
		return nil, err
	}
	suffix := strconv.FormatInt(time.Now().Unix(), 10)
	item := &model.Dashboard{
		Name: template.Name, Code: "dash_" + suffix, Description: template.Description,
		Layout: template.Layout, Theme: template.Theme, Status: "draft",
		Category: template.Category, CreatedBy: userID,
	}
	if err := s.dashboard.Create(item); err != nil {
		return nil, err
	}
	layout := json.RawMessage(item.Layout)
	return &DashboardView{
		ID: item.ID, Name: item.Name, Code: item.Code, Description: item.Description,
		Layout: layout, Theme: item.Theme, Status: item.Status, Category: item.Category,
		CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}, nil
}

// Delete 删除模板（内置模板不可删除）。
func (s *TemplateService) Delete(id uint64) error {
	item, err := s.repo.FindByID(id)
	if err != nil {
		return errors.New("模板不存在")
	}
	if item.BuiltIn {
		return errors.New("内置模板不可删除")
	}
	return s.repo.Delete(id)
}

func toTemplateView(item model.Template) TemplateView {
	layout := json.RawMessage(item.Layout)
	if len(layout) == 0 || !json.Valid(layout) {
		layout = json.RawMessage(`{"settings":{},"widgets":[]}`)
	}
	return TemplateView{
		ID: item.ID, Name: item.Name, Description: item.Description, Category: item.Category,
		Cover: item.Cover, Layout: layout, Theme: item.Theme, BuiltIn: item.BuiltIn,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

var _ = gorm.ErrRecordNotFound
