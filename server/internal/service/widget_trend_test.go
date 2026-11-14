package service

import "testing"

// TestParseTrendDate 校验多种日期写法都能统一成 YYYY-MM-DD。
func TestParseTrendDate(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"2026-11-14", "2026-11-14", true},
		{"2026-11-14 08:30:00", "2026-11-14", true},
		{"2026/11/09", "2026-11-09", true},
		{"2026-11", "2026-11-01", true},
		{"", "", false},
		{"华东大区", "", false},
	}
	for _, item := range cases {
		got, ok := parseTrendDate(item.input)
		if ok != item.ok || got != item.want {
			t.Fatalf("parseTrendDate(%q) = (%q, %v), 期望 (%q, %v)", item.input, got, ok, item.want, item.ok)
		}
	}
}

// TestDetectTrendDateColumn 校验日期列识别：优先配置，其次列名关键词。
func TestDetectTrendDateColumn(t *testing.T) {
	columns := []string{"region", "revenue", "stat_date"}
	rows := [][]any{
		{"华东", 1200.0, "2026-11-01"},
		{"华南", 900.0, "2026-11-02"},
	}
	if index := detectTrendDateColumn(columns, rows, map[string]any{}); index != 2 {
		t.Fatalf("未识别到日期列，实际下标 %d", index)
	}
	// 组件显式配置日期字段时应优先生效
	config := map[string]any{"dateField": "stat_date"}
	if index := detectTrendDateColumn(columns, rows, config); index != 2 {
		t.Fatalf("配置日期字段未被采用，实际下标 %d", index)
	}
	// 没有日期列时返回 -1，触发组织范围回退
	if index := detectTrendDateColumn([]string{"region", "revenue"}, [][]any{{"华东", 1.0}}, nil); index != -1 {
		t.Fatalf("不应识别出日期列，实际下标 %d", index)
	}
}

// TestAggregateRowsByDate 校验按日期求和与排序。
func TestAggregateRowsByDate(t *testing.T) {
	rows := [][]any{
		{"2026-11-02", 100.5},
		{"2026-11-01", 50.0},
		{"2026-11-02", 20.5},
		{"2026-11-03", "0"},
	}
	points := aggregateRowsByDate(rows, 0, 1)
	if len(points) != 3 {
		t.Fatalf("日期分组数量应为 3，实际 %d", len(points))
	}
	if points[0].Date != "2026-11-01" || points[0].Value != 50 {
		t.Fatalf("首日聚合不正确: %+v", points[0])
	}
	if points[1].Date != "2026-11-02" || points[1].Value != 121 {
		t.Fatalf("次日聚合不正确: %+v", points[1])
	}
}

// TestDefaultTrendField 校验未指定字段时从组件配置推断指标字段。
func TestDefaultTrendField(t *testing.T) {
	if got := defaultTrendField(map[string]any{"yField": "revenue"}); got != "revenue" {
		t.Fatalf("yField 推断失败: %s", got)
	}
	config := map[string]any{"columns": []any{map[string]any{"field": "gross_profit"}}}
	if got := defaultTrendField(config); got != "gross_profit" {
		t.Fatalf("KPI columns 推断失败: %s", got)
	}
	if got := defaultTrendField(nil); got != "" {
		t.Fatalf("空配置应返回空字符串，实际 %q", got)
	}
}

// TestWidgetFieldMeta 校验字段名称、单位、格式的读取。
func TestWidgetFieldMeta(t *testing.T) {
	config := map[string]any{
		"columns": []any{map[string]any{"field": "revenue", "label": "销售额", "unit": "元", "format": "currency"}},
	}
	label, unit, format := widgetFieldMeta(config, "revenue")
	if label != "销售额" || unit != "元" || format != "currency" {
		t.Fatalf("字段元信息不正确: %s / %s / %s", label, unit, format)
	}
}

// TestFilterTrendRows 校验按维度过滤行。
func TestFilterTrendRows(t *testing.T) {
	rows := [][]any{
		{"华东", 1.0},
		{"华南", 2.0},
		{"华东", 3.0},
	}
	filtered := filterTrendRows(rows, 0, "华东")
	if len(filtered) != 2 {
		t.Fatalf("过滤后应剩 2 行，实际 %d", len(filtered))
	}
}

// TestAverageRowsByDate 校验比率型指标按日均聚合。
func TestAverageRowsByDate(t *testing.T) {
	rows := [][]any{
		{"2026-11-02", 20.0},
		{"2026-11-02", 30.0},
		{"2026-11-01", 40.0},
	}
	points := averageRowsByDate(rows, 0, 1)
	if len(points) != 2 {
		t.Fatalf("日期分组数量应为 2，实际 %d", len(points))
	}
	if points[0].Date != "2026-11-01" || points[0].Value != 40 {
		t.Fatalf("首日均值不正确: %+v", points[0])
	}
	if points[1].Date != "2026-11-02" || points[1].Value != 25 {
		t.Fatalf("次日均值不正确: %+v", points[1])
	}
}

// TestIsRateField 校验比率字段的识别。
func TestIsRateField(t *testing.T) {
	for _, field := range []string{"gross_margin", "margin", "achieve_rate", "毛利率", "销售占比"} {
		if !isRateField(field) {
			t.Fatalf("%s 应识别为比率型字段", field)
		}
	}
	for _, field := range []string{"revenue", "orders", "customers"} {
		if isRateField(field) {
			t.Fatalf("%s 不应识别为比率型字段", field)
		}
	}
}
