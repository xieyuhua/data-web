package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WidgetTrendPoint 单个日期的趋势点。
type WidgetTrendPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// WidgetTrend 看板组件的指标趋势。
// Source 取值为 dataset（直接对数据集结果按日期聚合）或 scope（按当前组织范围汇总销售事实）。
type WidgetTrend struct {
	WidgetID    string             `json:"widget_id"`
	WidgetTitle string             `json:"widget_title"`
	Field       string             `json:"field"`
	FieldLabel  string             `json:"field_label"`
	Unit        string             `json:"unit"`
	Format      string             `json:"format"`
	Source      string             `json:"source"`
	DateColumn  string             `json:"date_column"`
	StartDate   string             `json:"start_date"`
	EndDate     string             `json:"end_date"`
	Total       float64            `json:"total"`
	Average     float64            `json:"average"`
	Min         float64            `json:"min"`
	MinDate     string             `json:"min_date"`
	Peak        float64            `json:"peak"`
	PeakDate    string             `json:"peak_date"`
	Points      []WidgetTrendPoint `json:"points"`
}

// WidgetTrendRequest 组件趋势查询参数。
type WidgetTrendRequest struct {
	WidgetID   string
	Field      string
	GroupField string
	GroupValue string
	StartDate  string
	EndDate    string
}

// 已知销售指标别名，用于在数据集没有日期字段时回退到组织范围趋势。
var widgetTrendMetricAliases = map[string]string{
	"revenue":      "revenue",
	"sales":        "revenue",
	"amount":       "revenue",
	"gross_profit": "gross_profit",
	"profit":       "gross_profit",
	"orders":       "orders",
	"order_count":  "orders",
	"customers":    "customers",
	"customer":     "customers",
	"gross_margin": "gross_margin",
	"margin":       "gross_margin",
}

// WidgetTrend 计算看板内单个组件的指标趋势。
//
// 处理顺序：
//  1. 在数据集查询结果中寻找日期列，按日期对目标字段求和（适用于自定义 SQL 数据集）；
//  2. 数据集没有日期列时，若字段属于销售额 / 毛利 / 订单 / 客流 / 毛利率，
//     则回退为按当前组织数据范围聚合销售事实；
//  3. 两者都不满足时返回可读的错误提示，前端据此隐藏 / 提示趋势。
func (s *DashboardService) WidgetTrend(ctx context.Context, dashboardID uint64, req WidgetTrendRequest, orgIDs []uint64, options QueryOptions) (*WidgetTrend, error) {
	dashboard, err := s.repo.FindByID(dashboardID)
	if err != nil {
		return nil, errors.New("看板不存在")
	}
	layout, err := parseDashboardLayout(dashboard.Layout)
	if err != nil {
		return nil, err
	}
	widget, ok := findLayoutWidget(layout.Widgets, req.WidgetID)
	if !ok {
		return nil, errors.New("组件不存在")
	}
	if widget.ChartType == "text" || widget.DatasetID == 0 {
		return nil, errors.New("该组件没有可分析的指标数据")
	}
	start, end, err := normalizeTrendRange(req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}
	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")
	field := strings.TrimSpace(req.Field)
	if field == "" {
		field = defaultTrendField(widget.Config)
	}
	label, unit, format := widgetFieldMeta(widget.Config, field)

	// 趋势查询需要拿到最新数据，默认跳过 20 秒短时缓存。
	options.NoCache = true
	options.Filters = sanitizeFilterValues(layout.FilterDefs(), options.Filters, options.ShareMode)
	dataset, err := s.datasetRepo.FindByID(widget.DatasetID)
	if err != nil {
		return nil, errors.New("关联数据集不存在")
	}
	result, err := s.dataset.QueryWithOptions(ctx, dataset, orgIDs, startDate, endDate, options)
	if err != nil {
		return nil, err
	}
	if field == "" {
		field = detectTrendValueColumn(result.Columns, result.Rows, widget.Config)
	}
	if field == "" {
		return nil, errors.New("当前组件没有可做趋势的数值字段")
	}
	// 字段确定后再补读一次展示元信息：默认字段是在查询后推断出来的，
	// 首次读取时 field 为空，会漏掉组件上配置的单位 / 格式。
	if metaLabel, metaUnit, metaFormat := widgetFieldMeta(widget.Config, field); metaLabel != "" || metaUnit != "" || metaFormat != "" {
		if label == "" {
			label = metaLabel
		}
		if unit == "" {
			unit = metaUnit
		}
		if format == "" {
			format = metaFormat
		}
	}
	// 数据集本身没有中文列名时，用内置指标字典补一个可读名称。
	if fallbackLabel, fallbackUnit, fallbackFormat := trendFieldFallbackMeta(field); label == "" || unit == "" || format == "" {
		if label == "" {
			label = fallbackLabel
		}
		if unit == "" {
			unit = fallbackUnit
		}
		if format == "" {
			format = fallbackFormat
		}
	}
	if label == "" {
		label = field
	}
	// 毛利率 / 达成率等比率型指标统一按百分比展示，避免和金额混淆。
	if isRateField(field) {
		format = "percent"
		if unit == "" {
			unit = "%"
		}
	}
	if groupIndex := columnIndex(result.Columns, req.GroupField); groupIndex >= 0 && req.GroupValue != "" {
		result.Rows = filterTrendRows(result.Rows, groupIndex, req.GroupValue)
	}

	// 优先按数据集自身的日期列聚合，能完整支持任意自定义 SQL。
	if dateIndex := detectTrendDateColumn(result.Columns, result.Rows, widget.Config); dateIndex >= 0 {
		valueIndex := columnIndex(result.Columns, field)
		// 目标字段不在结果列中时不做数据集聚合，继续走组织范围回退
		if valueIndex >= 0 {
			var points []WidgetTrendPoint
			if isRateField(field) {
				// 比率型指标按日均聚合，直接求和会得到无意义的数值
				points = averageRowsByDate(result.Rows, dateIndex, valueIndex)
			} else {
				points = aggregateRowsByDate(result.Rows, dateIndex, valueIndex)
			}
			return buildWidgetTrend(widget, field, label, unit, format, "dataset",
				result.Columns[dateIndex], startDate, endDate, points), nil
		}
	}

	// 数据集无日期维度时，回退到组织范围销售事实的趋势。
	metricKey := widgetTrendMetricAliases[strings.ToLower(strings.TrimSpace(field))]
	if metricKey == "" || s.drill == nil {
		return nil, errors.New("当前数据集没有日期字段，无法生成趋势图，可在组件配置中指定日期字段")
	}
	trend, err := s.drill.ScopeTrend(orgIDs, metricKey, startDate, endDate)
	if err != nil {
		return nil, err
	}
	points := make([]WidgetTrendPoint, 0, len(trend.Points))
	for _, item := range trend.Points {
		points = append(points, WidgetTrendPoint{Date: item.Date, Value: item.Value})
	}
	if label == "" {
		label = trend.MetricLabel
	}
	if unit == "" {
		unit = trend.Unit
	}
	built := buildWidgetTrend(widget, field, label, unit, format, "scope",
		"stat_date", startDate, endDate, points)
	if built.Format == "" {
		built.Format = trendMetricFormat(metricKey)
	}
	return built, nil
}

// 内置指标展示字典：数据集列没有中文说明时用于兜底。
var trendFieldLabels = map[string][3]string{
	"revenue":      {"销售额", "元", "currency"},
	"sales":        {"销售额", "元", "currency"},
	"amount":       {"销售额", "元", "currency"},
	"gross_profit": {"毛利", "元", "currency"},
	"profit":       {"毛利", "元", "currency"},
	"orders":       {"订单数", "单", "integer"},
	"order_count":  {"订单数", "单", "integer"},
	"customers":    {"客流", "人", "integer"},
	"customer":     {"客流", "人", "integer"},
	"gross_margin": {"毛利率", "%", "percent"},
	"margin":       {"毛利率", "%", "percent"},
}

// trendFieldFallbackMeta 按字段名返回兜底的展示名称、单位与格式。
func trendFieldFallbackMeta(field string) (label, unit, format string) {
	if meta, ok := trendFieldLabels[strings.ToLower(strings.TrimSpace(field))]; ok {
		return meta[0], meta[1], meta[2]
	}
	return "", "", ""
}

// parseDashboardLayout 解析看板布局 JSON。
func parseDashboardLayout(raw string) (DashboardLayout, error) {
	var layout DashboardLayout
	if strings.TrimSpace(raw) == "" {
		return layout, nil
	}
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		return layout, errors.New("看板布局数据损坏")
	}
	return layout, nil
}

// FilterDefs 从布局设置中解析全局筛选器定义。
func (l DashboardLayout) FilterDefs() []DashboardFilter {
	raw, ok := l.Settings["filters"]
	if !ok {
		return nil
	}
	var defs []DashboardFilter
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(encoded, &defs); err != nil {
		return nil
	}
	return defs
}

// findLayoutWidget 按组件 ID 查找布局组件，兼容前端传入数字下标的场景。
func findLayoutWidget(widgets []DashboardWidget, id string) (DashboardWidget, bool) {
	id = strings.TrimSpace(id)
	for _, item := range widgets {
		if item.ID == id {
			return item, true
		}
	}
	if index, err := strconv.Atoi(id); err == nil && index >= 0 && index < len(widgets) {
		return widgets[index], true
	}
	return DashboardWidget{}, false
}

// defaultTrendField 在未显式指定字段时，优先取配置的主指标列。
func defaultTrendField(config map[string]any) string {
	if config == nil {
		return ""
	}
	for _, key := range []string{"yField", "valueField", "rateField", "actualField"} {
		if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	// KPI 组件配置的 columns 形如 [{field,label,unit,format}]，取第一个数值指标。
	if columns, ok := config["columns"].([]any); ok {
		for _, item := range columns {
			if entry, ok := item.(map[string]any); ok {
				if field, ok := entry["field"].(string); ok && strings.TrimSpace(field) != "" {
					return strings.TrimSpace(field)
				}
			}
		}
	}
	return ""
}

// widgetFieldMeta 从组件配置中读取字段的展示名称、单位与格式。
func widgetFieldMeta(config map[string]any, field string) (label, unit, format string) {
	if config == nil || field == "" {
		return "", "", ""
	}
	columns, _ := config["columns"].([]any)
	for _, item := range columns {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := entry["field"].(string); !ok || value != field {
			continue
		}
		if value, ok := entry["label"].(string); ok {
			label = value
		}
		if value, ok := entry["unit"].(string); ok {
			unit = value
		}
		if value, ok := entry["format"].(string); ok {
			format = value
		}
		return label, unit, format
	}
	if value, ok := config["unit"].(string); ok {
		unit = value
	}
	return label, unit, format
}

// detectTrendValueColumn 在未配置指标字段时，挑选第一个可用数值列。
func detectTrendValueColumn(columns []string, rows [][]any, config map[string]any) string {
	dateIndex := detectTrendDateColumn(columns, rows, config)
	for index, name := range columns {
		if index == dateIndex {
			continue
		}
		for _, row := range rows {
			if index >= len(row) {
				continue
			}
			if _, ok := numericValue(row[index]); ok {
				return name
			}
		}
	}
	return ""
}

// detectTrendDateColumn 识别数据集中的日期列。
// 优先使用组件配置（dateField / trendDateField），其次按列名关键词，最后按取值格式判断。
func detectTrendDateColumn(columns []string, rows [][]any, config map[string]any) int {
	if config != nil {
		for _, key := range []string{"dateField", "trendDateField", "xField"} {
			if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
				if index := columnIndex(columns, value); index >= 0 && looksLikeDateColumn(columns[index], rows, index) {
					return index
				}
			}
		}
	}
	for index, name := range columns {
		lower := strings.ToLower(name)
		for _, keyword := range []string{"date", "stat_day", "day", "dt", "日期", "时间"} {
			if strings.Contains(lower, keyword) && looksLikeDateColumn(name, rows, index) {
				return index
			}
		}
	}
	for index, name := range columns {
		if looksLikeDateColumn(name, rows, index) {
			return index
		}
	}
	return -1
}

// looksLikeDateColumn 通过前若干行的取值判断该列是否存放日期。
func looksLikeDateColumn(name string, rows [][]any, index int) bool {
	checked := 0
	for _, row := range rows {
		if index >= len(row) {
			continue
		}
		value := row[index]
		if value == nil {
			continue
		}
		if _, ok := parseTrendDate(value); !ok {
			return false
		}
		checked++
		if checked >= 5 {
			break
		}
	}
	if checked == 0 {
		// 没有数据时只依据列名判断，避免空结果导致无法识别。
		lower := strings.ToLower(strings.TrimSpace(name))
		return strings.Contains(lower, "date") || strings.Contains(lower, "日期") || lower == "day" || lower == "dt"
	}
	return true
}

// parseTrendDate 把多种日期格式统一转成 YYYY-MM-DD。
func parseTrendDate(value any) (string, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed.Format("2006-01-02"), true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return "", false
		}
		// 常见的时间戳文本，截取前 10 位即可
		if len(text) >= 10 {
			candidate := text[:10]
			if parsed, err := time.ParseInLocation("2006-01-02", candidate, time.Local); err == nil {
				return parsed.Format("2006-01-02"), true
			}
		}
		for _, layout := range []string{"2006/01/02", "2006-1-2", "2006/1/2", "2006-01", "20060102"} {
			if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
				return parsed.Format("2006-01-02"), true
			}
		}
	}
	return "", false
}

// aggregateRowsByDate 按日期对目标字段求和；无法解析的日期与空值会被忽略。
func aggregateRowsByDate(rows [][]any, dateIndex, valueIndex int) []WidgetTrendPoint {
	grouped := groupRowsByDate(rows, dateIndex, valueIndex)
	dates := make([]string, 0, len(grouped))
	for date := range grouped {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	points := make([]WidgetTrendPoint, 0, len(dates))
	for _, date := range dates {
		sum := 0.0
		for _, value := range grouped[date] {
			sum += value
		}
		points = append(points, WidgetTrendPoint{Date: date, Value: round2(sum)})
	}
	return points
}

// averageRowsByDate 按日期对目标字段取平均，用于毛利率等比率型指标。
func averageRowsByDate(rows [][]any, dateIndex, valueIndex int) []WidgetTrendPoint {
	grouped := groupRowsByDate(rows, dateIndex, valueIndex)
	dates := make([]string, 0, len(grouped))
	for date := range grouped {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	points := make([]WidgetTrendPoint, 0, len(dates))
	for _, date := range dates {
		// 单日多行时取算术平均，避免比率被重复相加
		values := grouped[date]
		sum := 0.0
		for _, value := range values {
			sum += value
		}
		points = append(points, WidgetTrendPoint{Date: date, Value: round2(sum / float64(len(values)))})
	}
	return points
}

// groupRowsByDate 把行数据按日期分组，只保留可解析的日期与数值。
func groupRowsByDate(rows [][]any, dateIndex, valueIndex int) map[string][]float64 {
	grouped := make(map[string][]float64)
	if dateIndex < 0 || valueIndex < 0 {
		return grouped
	}
	for _, row := range rows {
		if dateIndex >= len(row) || valueIndex >= len(row) {
			continue
		}
		date, ok := parseTrendDate(row[dateIndex])
		if !ok {
			continue
		}
		value, ok := numericValue(row[valueIndex])
		if !ok {
			continue
		}
		grouped[date] = append(grouped[date], value)
	}
	return grouped
}

// isRateField 判断字段是否为比率型指标（毛利率、达成率等）。
func isRateField(field string) bool {
	lower := strings.ToLower(strings.TrimSpace(field))
	if lower == "gross_margin" || lower == "margin" {
		return true
	}
	for _, keyword := range []string{"rate", "ratio", "percent", "margin", "率", "占比"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// filterTrendRows 按维度取值过滤行（例如只看某个区域）。
func filterTrendRows(rows [][]any, index int, value string) [][]any {
	result := make([][]any, 0, len(rows))
	for _, row := range rows {
		if index >= len(row) {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(row[index])) == value {
			result = append(result, row)
		}
	}
	return result
}

// buildWidgetTrend 汇总趋势点，计算合计 / 日均 / 峰值。
func buildWidgetTrend(widget DashboardWidget, field, label, unit, format, source, dateColumn, startDate, endDate string, points []WidgetTrendPoint) *WidgetTrend {
	result := &WidgetTrend{
		WidgetID: widget.ID, WidgetTitle: widget.Title,
		Field: field, FieldLabel: label, Unit: unit, Format: format,
		Source: source, DateColumn: dateColumn,
		StartDate: startDate, EndDate: endDate, Points: points,
	}
	for _, point := range points {
		result.Total += point.Value
		if point.Value > result.Peak {
			result.Peak = point.Value
			result.PeakDate = point.Date
		}
	}
	// 最低值用于比率型指标（毛利率、达成率）展示区间波动
	if len(points) > 0 {
		result.Min = points[0].Value
		result.MinDate = points[0].Date
		for _, point := range points[1:] {
			if point.Value < result.Min {
				result.Min = point.Value
				result.MinDate = point.Date
			}
		}
	}
	result.Total = round2(result.Total)
	result.Peak = round2(result.Peak)
	result.Min = round2(result.Min)
	if len(points) > 0 {
		result.Average = round2(result.Total / float64(len(points)))
	}
	return result
}

// trendMetricFormat 把指标映射为前端的格式化类型。
func trendMetricFormat(metric string) string {
	switch metric {
	case "gross_margin":
		return "percent"
	case "orders", "customers":
		return "integer"
	default:
		return "currency"
	}
}

// columnIndex 返回列名所在下标，未找到返回 -1。
func columnIndex(columns []string, name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for index, column := range columns {
		if strings.EqualFold(column, name) {
			return index
		}
	}
	return -1
}

// numericValue 把单元格值安全转换为数值。
func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
