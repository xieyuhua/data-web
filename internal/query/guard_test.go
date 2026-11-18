package query

import "testing"

func TestValidateReadOnly(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{"plain select", "SELECT * FROM sales", false},
		{"with cte", "WITH t AS (SELECT 1) SELECT * FROM t", false},
		{"explain", "EXPLAIN SELECT 1", false},
		{"show", "SHOW TABLES", false},
		{"trailing semicolon", "SELECT 1;", false},
		{"lowercase", "select * from sales", false},
		{"delete", "DELETE FROM sales", true},
		{"update", "UPDATE sales SET amount = 0", true},
		{"insert", "INSERT INTO sales VALUES (1)", true},
		{"drop", "DROP TABLE sales", true},
		{"alter", "ALTER TABLE sales ADD COLUMN x INT", true},
		{"truncate", "TRUNCATE TABLE sales", true},
		{"multi statement", "SELECT 1; DROP TABLE sales", true},
		{"multi statement commented", "SELECT 1 -- x\n; DELETE FROM sales", true},
		{"keyword in string literal", "SELECT 'DELETE FROM x' AS s", false},
		{"keyword in comment only", "SELECT * FROM sales /* DELETE */", false},
		{"dollar quoted literal", "SELECT $$DROP TABLE x$$ AS s", false},
		{"quoted identifier", "SELECT `delete` FROM t", false},
		{"union into outfile", "SELECT * FROM t INTO OUTFILE '/tmp/x'", true},
		{"pragma", "PRAGMA table_info(sales)", true},
		{"attach", "ATTACH DATABASE 'x.db' AS x", true},
		{"empty", "   ", true},
		{"unclosed quote", "SELECT 'abc FROM t", true},
		{"unclosed comment", "SELECT 1 /* abc", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateReadOnly(c.sql)
			if c.wantErr && err == nil {
				t.Fatalf("expected error for %q", c.sql)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", c.sql, err)
			}
		})
	}
}

func TestBindParams(t *testing.T) {
	specs := []ParamSpec{
		{Name: "from_date", Type: "date", Required: true},
		{Name: "region", Type: "string"},
	}
	sqlText := "SELECT * FROM sales WHERE order_date >= {{from_date}} AND region = {{region}}"

	out, args, err := BindParams(sqlText, specs, map[string]string{"from_date": "2026-01-01", "region": "east"}, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if out != "SELECT * FROM sales WHERE order_date >= ? AND region = ?" {
		t.Fatalf("unexpected sql: %s", out)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}

	pgSQL, _, err := BindParams(sqlText, specs, map[string]string{"from_date": "2026-01-01", "region": "east"}, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM sales WHERE order_date >= $1 AND region = $2"
	if pgSQL != want {
		t.Fatalf("pg placeholders wrong: %s", pgSQL)
	}

	// Repeated token reuses the value and appends each placeholder arg.
	rep := "SELECT * FROM t WHERE a={{region}} OR b={{region}}"
	outRep, argsRep, err := BindParams(rep, specs, map[string]string{"region": "x"}, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if outRep != "SELECT * FROM t WHERE a=? OR b=?" || len(argsRep) != 2 {
		t.Fatalf("repeat binding wrong: %s %v", outRep, argsRep)
	}

	if _, _, err := BindParams(sqlText, specs, map[string]string{"region": "x"}, "sqlite"); err == nil {
		t.Fatal("expected missing required param error")
	}
	if _, _, err := BindParams("SELECT {{undeclared}}", specs, nil, "sqlite"); err == nil {
		t.Fatal("expected undeclared param error")
	}
}

func TestParamNames(t *testing.T) {
	names := ParamNames("SELECT {{a}}, {{b}}, {{a}}")
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("unexpected names: %v", names)
	}
}
