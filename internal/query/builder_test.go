package query

import (
	"strings"
	"testing"

	"biaoge-bi/internal/model"
)

func testDataset() *model.Dataset {
	return &model.Dataset{
		ID: 1, DataSourceID: 1, SQLText: "SELECT region, product, amount FROM sales",
		ColumnMetaJSON: `[{"name":"region"},{"name":"product"},{"name":"amount"}]`,
	}
}

func testColumns() []model.ColumnMeta {
	return []model.ColumnMeta{
		{Name: "region"}, {Name: "product"}, {Name: "amount"},
	}
}

func TestBuildChartSQLAggregate(t *testing.T) {
	cfg := &model.ChartConfig{
		ChartType:  model.ChartBar,
		DataMode:   model.ModeAggregate,
		Dimensions: []model.FieldRef{{Field: "region", Alias: "地区"}},
		Metrics:    []model.MetricRef{{Field: "amount", Agg: "sum", Alias: "销售额"}},
		Sort:       []model.SortSpec{{Alias: "销售额", Desc: true}},
		Limit:      100,
	}
	sqlText, args, err := BuildChartSQL(testDataset(), cfg, nil, model.DSSQLite, testColumns())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`SELECT "region" AS "地区", SUM("amount") AS "销售额"`,
		"FROM (SELECT region, product, amount FROM sales) AS bi_d",
		`GROUP BY "region"`,
		`ORDER BY "销售额" DESC`,
		"LIMIT 100",
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("missing %q in SQL:\n%s", want, sqlText)
		}
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args: %v", args)
	}

	mysql, _, err := BuildChartSQL(testDataset(), cfg, nil, model.DSMySQL, testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mysql, "`region` AS `地区`") {
		t.Fatalf("mysql quoting wrong: %s", mysql)
	}
}

func TestBuildChartSQLRejectsInvalid(t *testing.T) {
	base := func() *model.ChartConfig {
		return &model.ChartConfig{
			ChartType:  model.ChartBar,
			DataMode:   model.ModeAggregate,
			Dimensions: []model.FieldRef{{Field: "region", Alias: "r"}},
			Metrics:    []model.MetricRef{{Field: "amount", Agg: "sum", Alias: "a"}},
			Limit:      10,
		}
	}

	bad := base()
	bad.Dimensions[0].Field = "missing_column"
	if _, _, err := BuildChartSQL(testDataset(), bad, nil, model.DSSQLite, testColumns()); err == nil {
		t.Fatal("expected unknown field error")
	}

	bad = base()
	bad.Metrics[0].Agg = "drop_table"
	if _, _, err := BuildChartSQL(testDataset(), bad, nil, model.DSSQLite, testColumns()); err == nil {
		t.Fatal("expected invalid aggregate error")
	}

	bad = base()
	bad.Metrics[0].Alias = "bad\"alias"
	if _, _, err := BuildChartSQL(testDataset(), bad, nil, model.DSSQLite, testColumns()); err == nil {
		t.Fatal("expected invalid alias error")
	}

	bad = base()
	bad.Sort = []model.SortSpec{{Alias: "not_selected", Desc: true}}
	if _, _, err := BuildChartSQL(testDataset(), bad, nil, model.DSSQLite, testColumns()); err == nil {
		t.Fatal("expected invalid sort alias error")
	}

	pie := base()
	pie.ChartType = model.ChartPie
	pie.Metrics = append(pie.Metrics, model.MetricRef{Field: "amount", Agg: "avg", Alias: "b"})
	if _, _, err := BuildChartSQL(testDataset(), pie, nil, model.DSSQLite, testColumns()); err == nil {
		t.Fatal("expected pie metric validation error")
	}

	kpi := &model.ChartConfig{ChartType: model.ChartKPI, DataMode: model.ModeAggregate,
		Metrics: []model.MetricRef{{Field: "amount", Agg: "sum", Alias: "总"}}, Limit: 1}
	sqlText, _, err := BuildChartSQL(testDataset(), kpi, nil, model.DSSQLite, testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "SUM(\"amount\")") || strings.Contains(sqlText, "GROUP BY") {
		t.Fatalf("kpi SQL wrong: %s", sqlText)
	}
}

func TestBuildChartSQLParams(t *testing.T) {
	ds := testDataset()
	ds.SQLText = "SELECT region, amount FROM sales WHERE region = {{region}}"
	ds.Params = []model.DatasetParam{{Name: "region", Type: "string"}}
	cfg := &model.ChartConfig{
		ChartType: model.ChartTable, DataMode: model.ModeRaw,
		Dimensions: []model.FieldRef{{Field: "region", Alias: "r"}, {Field: "amount", Alias: "a"}},
		Limit:      10,
	}
	sqlText, args, err := BuildChartSQL(ds, cfg, map[string]string{"region": "east"}, model.DSPostgres, testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "region = $1") || len(args) != 1 || args[0] != "east" {
		t.Fatalf("param binding wrong: %s %v", sqlText, args)
	}
	if strings.Contains(sqlText, "{{") {
		t.Fatalf("placeholder not replaced: %s", sqlText)
	}
}

func TestCacheKeyStability(t *testing.T) {
	a := CacheKey("chart", 1, 2, map[string]string{"a": "1", "b": "2"}, "aggregate", 10)
	b := CacheKey("chart", 1, 2, map[string]string{"b": "2", "a": "1"}, "aggregate", 10)
	if a != b {
		t.Fatal("cache key must be order-independent for maps")
	}
	c := CacheKey("chart", 1, 3, map[string]string{"a": "1", "b": "2"}, "aggregate", 10)
	if a == c {
		t.Fatal("cache key must change with version")
	}
}
