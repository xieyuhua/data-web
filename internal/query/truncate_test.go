package query

import (
	"strings"
	"testing"

	"biaoge-bi/internal/model"
)

func TestTruncateDateDialects(t *testing.T) {
	cases := []struct {
		dialect string
		grain   string
		want    string
	}{
		{model.DSSQLite, "day", `DATE("d")`},
		{model.DSSQLite, "month", `STRFTIME('%Y-%m', "d")`},
		{model.DSSQLite, "year", `STRFTIME('%Y', "d")`},
		{model.DSMySQL, "month", `DATE_FORMAT("d", '%Y-%m')`},
		{model.DSMySQL, "quarter", `CONCAT(YEAR("d"), '-Q', QUARTER("d"))`},
		{model.DSPostgres, "week", `DATE_TRUNC('week', "d")`},
		{model.DSPostgres, "month", `TO_CHAR(DATE_TRUNC('month', "d"), 'YYYY-MM')`},
	}
	for _, c := range cases {
		got, err := truncateDate(`"d"`, c.grain, c.dialect)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.dialect, c.grain, err)
		}
		if got != c.want {
			t.Errorf("%s/%s = %s, want %s", c.dialect, c.grain, got, c.want)
		}
	}

	if _, err := truncateDate(`"d"`, "hour", model.DSSQLite); err == nil {
		t.Fatal("expected unsupported grain error")
	}
}

func TestBuildChartSQLWithGrain(t *testing.T) {
	ds := testDataset()
	cols := []model.ColumnMeta{{Name: "region"}, {Name: "product"}, {Name: "amount"}}
	cfg := &model.ChartConfig{
		ChartType:  model.ChartLine,
		DataMode:   model.ModeAggregate,
		Dimensions: []model.FieldRef{{Field: "region", Alias: "月", Grain: "month"}},
		Metrics:    []model.MetricRef{{Field: "amount", Agg: "sum", Alias: "总额"}},
		Limit:      100,
	}
	sqlText, _, err := BuildChartSQL(ds, cfg, nil, model.DSSQLite, cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, `GROUP BY STRFTIME('%Y-%m', "region")`) {
		t.Fatalf("grain not applied in GROUP BY: %s", sqlText)
	}
	if !strings.Contains(sqlText, `AS "月"`) {
		t.Fatalf("alias missing: %s", sqlText)
	}
}

func TestBindListParam(t *testing.T) {
	specs := []ParamSpec{{Name: "regions", Type: "list"}}
	sqlText := "SELECT * FROM sales WHERE region IN ({{regions}})"

	out, args, err := BindParams(sqlText, specs, map[string]string{"regions": `["华东","华南"]`}, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if out != "SELECT * FROM sales WHERE region IN (?, ?)" {
		t.Fatalf("unexpected sql: %s", out)
	}
	if len(args) != 2 || args[0] != "华东" || args[1] != "华南" {
		t.Fatalf("unexpected args: %v", args)
	}

	pgOut, _, err := BindParams(sqlText, specs, map[string]string{"regions": "华东,华南"}, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if pgOut != "SELECT * FROM sales WHERE region IN ($1, $2)" {
		t.Fatalf("unexpected pg sql: %s", pgOut)
	}

	// Empty selection must not produce invalid SQL.
	emptyOut, emptyArgs, err := BindParams(sqlText, specs, map[string]string{"regions": "[]"}, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emptyOut, "IN (NULL)") || len(emptyArgs) != 0 {
		t.Fatalf("empty list binding wrong: %s %v", emptyOut, emptyArgs)
	}
}

func TestSplitList(t *testing.T) {
	cases := map[string]int{
		`["a","b","c"]`: 3,
		"a,b,c":         3,
		" a , b ":       2,
		"":              0,
		"[]":            0,
		`["a"]`:         1,
	}
	for in, want := range cases {
		if got := len(splitList(in)); got != want {
			t.Errorf("splitList(%q) = %d items, want %d", in, got, want)
		}
	}
}
