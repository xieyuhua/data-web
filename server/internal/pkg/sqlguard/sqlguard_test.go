package sqlguard

import (
	"strings"
	"testing"
)

func baseParams() Params {
	return Params{OrgIDs: []uint64{1, 2}, StartDate: "2026-01-01", EndDate: "2026-01-31"}
}

func TestRenderForDialects(t *testing.T) {
	raw := "select org_id, sum(amount) as total from sales where org_id in ({{org_scope}}) group by org_id"
	cases := []struct {
		driver string
		want   string
	}{
		{"mysql", "LIMIT 2000"},
		{"postgres", "LIMIT 2000"},
		{"sqlite", "LIMIT 2000"},
		{"sqlserver", "SELECT TOP 2000 * FROM ("},
		{"oracle", "ROWNUM <= 2000"},
	}
	for _, item := range cases {
		result, err := RenderFor(item.driver, raw, true, baseParams())
		if err != nil {
			t.Fatalf("%s: %v", item.driver, err)
		}
		if !strings.Contains(result.SQL, item.want) {
			t.Fatalf("%s: 期望包含 %q，实际 %q", item.driver, item.want, result.SQL)
		}
		if !strings.Contains(result.SQL, "1,2") {
			t.Fatalf("%s: 组织范围未展开: %q", item.driver, result.SQL)
		}
	}
}

func TestRenderForOracleRejectsSysPackages(t *testing.T) {
	raw := "select utl_http.request('http://x') from dual"
	if _, err := RenderFor("oracle", raw, false, baseParams()); err == nil {
		t.Fatal("期望拒绝 Oracle 系统包调用")
	}
}

func TestRenderRequiresScope(t *testing.T) {
	raw := "select org_id from sales"
	if _, err := Render(raw, true, baseParams()); err == nil {
		t.Fatal("启用数据权限时必须包含 {{org_scope}}")
	}
}

func TestRenderEscapesFilterValues(t *testing.T) {
	params := baseParams()
	params.Filters = map[string]string{
		"channel": "线下' OR 1=1 --",
		"region":  `华东\`,
	}
	raw := "select * from sales where org_id in ({{org_scope}}) " +
		"and ({{filter:channel}} = '' or channel = {{filter:channel}}) and region = {{filter:region}}"
	result, err := Render(raw, true, params)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if !strings.Contains(result.SQL, `'线下'' OR 1=1 --'`) {
		t.Fatalf("单引号未正确转义: %q", result.SQL)
	}
	if !strings.Contains(result.SQL, `'华东\\'`) {
		t.Fatalf("反斜杠未正确转义: %q", result.SQL)
	}
	if strings.Contains(result.SQL, "{{filter:") {
		t.Fatalf("占位符未展开: %q", result.SQL)
	}
}

func TestRenderFilterValidation(t *testing.T) {
	params := baseParams()
	params.Filters = map[string]string{strings.Repeat("a", 40): "x"}
	if _, err := Render("select 1", false, params); err == nil {
		t.Fatal("非法的筛选器字段名应被拒绝")
	}
	params.Filters = map[string]string{"channel": strings.Repeat("超", MaxFilterValueLength+1)}
	if _, err := Render("select 1", false, params); err == nil {
		t.Fatal("超长筛选值应被拒绝")
	}
}

func TestRenderFilterIn(t *testing.T) {
	params := baseParams()
	params.Filters = map[string]string{
		"channel": JoinFilterValues([]string{"到店", "外卖' OR 1=1 --"}),
	}
	raw := "select * from sales where org_id in ({{org_scope}}) " +
		"and ({{filter_has:channel}} = 0 or channel in ({{filter_in:channel}}))"
	result, err := Render(raw, true, params)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if !strings.Contains(result.SQL, "(1 = 0 or channel in (") {
		t.Fatalf("filter_has 应展开为 1: %q", result.SQL)
	}
	want := "('到店','外卖'' OR 1=1 --')"
	if !strings.Contains(result.SQL, want) {
		t.Fatalf("多选值展开不正确: %q", result.SQL)
	}
	if strings.Count(result.SQL, "外卖'' OR 1=1 --") != 1 {
		t.Fatalf("恶意值应只出现一次且已转义: %q", result.SQL)
	}
}

func TestRenderFilterInEmpty(t *testing.T) {
	params := baseParams()
	params.Filters = map[string]string{"channel": ""}
	raw := "select * from sales where org_id in ({{org_scope}}) " +
		"and ({{filter_has:channel}} = 0 or channel in ({{filter_in:channel}}))"
	result, err := Render(raw, true, params)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if !strings.Contains(result.SQL, `(0 = 0 or channel in (''))`) {
		t.Fatalf("空多选应展开为 filter_has=0 且 IN (''): %q", result.SQL)
	}
}

func TestRenderFilterInRejectsTooManyValues(t *testing.T) {
	values := make([]string, 51)
	for index := range values {
		values[index] = "v"
	}
	params := baseParams()
	params.Filters = map[string]string{"channel": JoinFilterValues(values)}
	if _, err := Render("select 1", false, params); err == nil {
		t.Fatal("多选值数量超限应被拒绝")
	}
}
