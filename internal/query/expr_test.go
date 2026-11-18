package query

import (
	"strings"
	"testing"

	"biaoge-bi/internal/model"
)

func TestBuildCalcSQL(t *testing.T) {
	aliases := []string{"sales_sum", "target_sum", "profit_sum"}

	cases := []struct{ expr, want string }{
		{"sales_sum / target_sum", `("sales_sum" / NULLIF("target_sum", 0))`},
		{"sales_sum / NULLIF(target_sum, 0) * 100", `(("sales_sum" / NULLIF("target_sum", 0)) * 100)`},
		{"(profit_sum / sales_sum) * 100", `(("profit_sum" / NULLIF("sales_sum", 0)) * 100)`},
		{"sales_sum - target_sum", `("sales_sum" - "target_sum")`},
		{"ROUND(profit_sum / sales_sum * 100, 2)", `ROUND((("profit_sum" / NULLIF("sales_sum", 0)) * 100), 2)`},
		{"-sales_sum", `(-"sales_sum")`},
	}
	for _, c := range cases {
		got, err := BuildCalcSQL(c.expr, aliases, model.DSSQLite)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("%s\n got: %s\nwant: %s", c.expr, got, c.want)
		}
	}

	// Postgres/MySQL quoting differs but must stay valid.
	pg, err := BuildCalcSQL("sales_sum / target_sum", aliases, model.DSPostgres)
	if err != nil || !strings.Contains(pg, `"sales_sum"`) {
		t.Fatalf("postgres render failed: %s %v", pg, err)
	}
	my, err := BuildCalcSQL("sales_sum / target_sum", aliases, model.DSMySQL)
	if err != nil || !strings.Contains(my, "`sales_sum`") {
		t.Fatalf("mysql render failed: %s %v", my, err)
	}
}

func TestBuildCalcSQLRejectsUnsafeInput(t *testing.T) {
	aliases := []string{"sales_sum"}
	bad := []string{
		"1; DROP TABLE users",
		"sales_sum'; DELETE FROM sales; --",
		"unknown_metric * 2",
		"sales_sum / (",
		"sales_sum ) extra",
		"SUM(sales_sum)",
		"sales_sum, target_sum",
		"* 100",
		"sales_sum / 0 --",
		"",
	}
	for _, expr := range bad {
		if _, err := BuildCalcSQL(expr, aliases, model.DSSQLite); err == nil {
			t.Errorf("expected rejection for %q", expr)
		}
	}
}

func TestBuildCalcSQLChineseAliases(t *testing.T) {
	aliases := []string{"实际销售", "基础目标", "调整后目标"}
	got, err := BuildCalcSQL("实际销售 / NULLIF(基础目标, 0) * 100", aliases, model.DSSQLite)
	if err != nil {
		t.Fatalf("chinese alias rejected: %v", err)
	}
	want := `(("实际销售" / NULLIF("基础目标", 0)) * 100)`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	// Unknown Chinese alias must still be rejected.
	if _, err := BuildCalcSQL("不存在 / 基础目标", aliases, model.DSSQLite); err == nil {
		t.Fatal("unknown chinese alias should be rejected")
	}
}

func TestValidateCalcExpr(t *testing.T) {
	aliases := []string{"a", "b"}
	if err := ValidateCalcExpr("a + b * 2", aliases); err != nil {
		t.Fatalf("valid expr rejected: %v", err)
	}
	if err := ValidateCalcExpr("c + 1", aliases); err == nil {
		t.Fatal("unknown alias should be rejected")
	}
}

func TestBuildChartSQLWithCalculatedMetric(t *testing.T) {
	ds := &model.Dataset{
		ID: 1, DataSourceID: 1,
		SQLText: "SELECT region, amount, target FROM region_sales",
		Params:  nil,
	}
	cols := []model.ColumnMeta{{Name: "region"}, {Name: "amount"}, {Name: "target"}}
	cfg := &model.ChartConfig{
		ChartType:  model.ChartTable,
		DataMode:   model.ModeAggregate,
		Dimensions: []model.FieldRef{{Field: "region", Alias: "地区"}},
		Metrics: []model.MetricRef{
			{Field: "amount", Agg: "sum", Alias: "sales_sum", Format: "currency"},
			{Field: "target", Agg: "sum", Alias: "target_sum", Format: "currency"},
			{Alias: "达成率", Expr: "sales_sum / NULLIF(target_sum, 0) * 100", Format: "percent"},
		},
		Limit: 100,
	}
	sqlText, args, err := BuildChartSQL(ds, cfg, nil, model.DSSQLite, cols)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`SUM("amount") AS "sales_sum"`,
		`SUM("target") AS "target_sum"`,
		`AS "达成率"`,
		`NULLIF("target_sum", 0)`,
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("missing %q in:\n%s", want, sqlText)
		}
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args: %v", args)
	}

	// Calculated metrics may be sorted like any other alias.
	cfg.Sort = []model.SortSpec{{Alias: "达成率", Desc: true}}
	if _, _, err := BuildChartSQL(ds, cfg, nil, model.DSSQLite, cols); err != nil {
		t.Fatalf("sorting by calculated metric failed: %v", err)
	}

	// A calculated metric referencing an unknown alias must fail at build time.
	cfg.Metrics[2].Expr = "missing_sum * 2"
	cfg.Sort = nil
	if _, _, err := BuildChartSQL(ds, cfg, nil, model.DSSQLite, cols); err == nil {
		t.Fatal("expected unknown-alias error")
	}
}
