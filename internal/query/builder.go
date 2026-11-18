package query

import (
	"fmt"
	"strings"

	"biaoge-bi/internal/model"
)

var allowedAggs = map[string]string{
	"sum": "SUM", "avg": "AVG", "count": "COUNT", "count_distinct": "COUNT(DISTINCT %s)",
	"min": "MIN", "max": "MAX",
}

// BuildChartSQL wraps the dataset SQL and applies dimensions/metrics/sort/limit.
// It returns the SQL with placeholders intact and the parameter args for binding.
// Column references are validated against the dataset column metadata.
func BuildChartSQL(dataset *model.Dataset, cfg *model.ChartConfig, params map[string]string, dialect string, knownColumns []model.ColumnMeta) (string, []any, error) {
	if err := validateChartConfig(cfg); err != nil {
		return "", nil, err
	}
	columnSet := make(map[string]model.ColumnMeta, len(knownColumns))
	for _, c := range knownColumns {
		columnSet[c.Name] = c
	}

	checkField := func(field string) (string, error) {
		if _, ok := columnSet[field]; !ok {
			return "", fmt.Errorf("字段 %q 不在数据集列中", field)
		}
		return QuoteIdentifier(field, dialect)
	}

	// dimExpr resolves a dimension reference, applying optional time truncation.
	dimExpr := func(d model.FieldRef) (string, error) {
		quoted, err := checkField(d.Field)
		if err != nil {
			return "", err
		}
		if d.Grain == "" {
			return quoted, nil
		}
		truncated, err := truncateDate(quoted, d.Grain, dialect)
		if err != nil {
			return "", err
		}
		return truncated, nil
	}

	var selectParts []string
	var groupByParts []string
	var calcList []model.MetricRef
	aliases := make(map[string]struct{})

	if cfg.DataMode == model.ModeRaw {
		for _, d := range cfg.Dimensions {
			q, err := dimExpr(d)
			if err != nil {
				return "", nil, err
			}
			alias := d.Alias
			if alias == "" {
				alias = d.Field
			}
			if !ValidAlias(alias) {
				return "", nil, fmt.Errorf("非法列别名 %q", alias)
			}
			selectParts = append(selectParts, fmt.Sprintf("%s AS %s", q, quoteAlias(alias, dialect)))
			aliases[alias] = struct{}{}
		}
		if len(selectParts) == 0 {
			return "", nil, fmt.Errorf("明细模式至少需要一个字段")
		}
	} else {
		for _, d := range cfg.Dimensions {
			q, err := dimExpr(d)
			if err != nil {
				return "", nil, err
			}
			alias := d.Alias
			if alias == "" {
				alias = d.Field
			}
			if !ValidAlias(alias) {
				return "", nil, fmt.Errorf("非法列别名 %q", alias)
			}
			if _, dup := aliases[alias]; dup {
				return "", nil, fmt.Errorf("重复的列别名 %q", alias)
			}
			aliases[alias] = struct{}{}
			selectParts = append(selectParts, fmt.Sprintf("%s AS %s", q, quoteAlias(alias, dialect)))
			groupByParts = append(groupByParts, q)
		}
		// Pass 1: base aggregations. Calculated metrics reference these by alias,
		// so they must be declared before any expression is evaluated.
		var calcMetrics []model.MetricRef
		for _, m := range cfg.Metrics {
			if strings.TrimSpace(m.Expr) != "" {
				calcMetrics = append(calcMetrics, m)
				continue
			}
			agg, ok := allowedAggs[strings.ToLower(m.Agg)]
			if !ok {
				return "", nil, fmt.Errorf("不支持的聚合方式 %q", m.Agg)
			}
			alias := m.Alias
			if alias == "" {
				alias = m.Agg + "_" + m.Field
			}
			if !ValidAlias(alias) {
				return "", nil, fmt.Errorf("非法指标别名 %q", alias)
			}
			if _, dup := aliases[alias]; dup {
				return "", nil, fmt.Errorf("重复的列别名 %q", alias)
			}
			aliases[alias] = struct{}{}

			if strings.EqualFold(m.Field, "*") && strings.EqualFold(m.Agg, "count") {
				selectParts = append(selectParts, fmt.Sprintf("COUNT(*) AS %s", quoteAlias(alias, dialect)))
				continue
			}
			q, err := checkField(m.Field)
			if err != nil {
				return "", nil, err
			}
			if strings.Contains(agg, "%s") {
				selectParts = append(selectParts, fmt.Sprintf(agg+" AS %s", q, quoteAlias(alias, dialect)))
			} else {
				selectParts = append(selectParts, fmt.Sprintf("%s(%s) AS %s", agg, q, quoteAlias(alias, dialect)))
			}
		}

		// Pass 2: calculated metrics (ratios such as target attainment or gross
		// margin). Each expression is parsed, validated against known aliases,
		// and re-rendered as SQL — user text is never pasted in directly.
		baseAliases := make([]string, 0, len(aliases))
		for a := range aliases {
			baseAliases = append(baseAliases, a)
		}
		for _, m := range calcMetrics {
			alias := m.Alias
			if alias == "" {
				return "", nil, fmt.Errorf("计算指标必须填写别名")
			}
			if !ValidAlias(alias) {
				return "", nil, fmt.Errorf("非法指标别名 %q", alias)
			}
			if _, dup := aliases[alias]; dup {
				return "", nil, fmt.Errorf("重复的列别名 %q", alias)
			}
			// Validate now so errors surface even when we build a wrapper query.
			if _, err := BuildCalcSQL(m.Expr, baseAliases, dialect); err != nil {
				return "", nil, fmt.Errorf("计算指标 %q 表达式无效: %v", alias, err)
			}
			aliases[alias] = struct{}{}
		}
		calcList = calcMetrics
		if len(cfg.Metrics) == 0 {
			return "", nil, fmt.Errorf("至少需要一个指标")
		}
	}

	boundSQL, args, err := BindParams(dataset.SQLText, SpecsFromModel(dataset.Params), params, dialect)
	if err != nil {
		return "", nil, err
	}

	var b strings.Builder

	// Aggregate/selection layer. When calculated metrics exist, this becomes an
	// inner query and the expressions are evaluated on top of it: SQL does not
	// allow referencing a SELECT-list alias from the same SELECT.
	inner := &strings.Builder{}
	inner.WriteString("SELECT ")
	inner.WriteString(strings.Join(selectParts, ", "))
	inner.WriteString(" FROM (")
	inner.WriteString(boundSQL)
	inner.WriteString(") AS bi_d")
	if cfg.DataMode != model.ModeRaw && len(groupByParts) > 0 {
		inner.WriteString(" GROUP BY ")
		inner.WriteString(strings.Join(groupByParts, ", "))
	}

	if len(calcList) > 0 {
		calcParts := make([]string, 0, len(calcList))
		for _, m := range calcList {
			exprSQL, err := BuildCalcSQL(m.Expr, aliasesToList(aliases), dialect)
			if err != nil {
				return "", nil, fmt.Errorf("计算指标 %q 表达式无效: %v", m.Alias, err)
			}
			calcParts = append(calcParts, fmt.Sprintf("%s AS %s", exprSQL, quoteAlias(m.Alias, dialect)))
		}
		b.WriteString("SELECT bi_agg.*, ")
		b.WriteString(strings.Join(calcParts, ", "))
		b.WriteString(" FROM (")
		b.WriteString(inner.String())
		b.WriteString(") AS bi_agg")
	} else {
		b.WriteString(inner.String())
	}

	if len(cfg.Sort) > 0 {
		var sorts []string
		for _, s := range cfg.Sort {
			if _, ok := aliases[s.Alias]; !ok {
				return "", nil, fmt.Errorf("排序字段 %q 不在选择列中", s.Alias)
			}
			dir := "ASC"
			if s.Desc {
				dir = "DESC"
			}
			sorts = append(sorts, quoteAlias(s.Alias, dialect)+" "+dir)
		}
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(sorts, ", "))
	}

	limit := cfg.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > ChartMaxRows {
		limit = ChartMaxRows
	}
	fmt.Fprintf(&b, " LIMIT %d", limit)

	return b.String(), args, nil
}

// aliasesToList returns map keys as a slice (order is irrelevant for validation).
func aliasesToList(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func quoteAlias(alias, dialect string) string {
	if dialect == model.DSMySQL {
		return "`" + alias + "`"
	}
	return `"` + alias + `"`
}

func validateChartConfig(cfg *model.ChartConfig) error {
	switch cfg.ChartType {
	case model.ChartKPI:
		// A calculated KPI (e.g. an attainment ratio) needs its base metrics as
		// inputs, so require exactly one *displayed* metric and allow base ones.
		displayed := 0
		for _, m := range cfg.Metrics {
			if strings.TrimSpace(m.Expr) == "" {
				displayed++
			}
		}
		calc := len(cfg.Metrics) - displayed
		if calc == 0 && displayed != 1 {
			return fmt.Errorf("KPI 图表需要且仅需要一个指标")
		}
		if calc > 1 {
			return fmt.Errorf("KPI 图表只支持一个计算指标")
		}
		if calc == 1 && displayed == 0 {
			return fmt.Errorf("计算指标需要至少一个基础指标作为输入")
		}
	case model.ChartPie:
		if len(cfg.Dimensions) != 1 || len(cfg.Metrics) != 1 {
			return fmt.Errorf("饼图需要一个维度和一个指标")
		}
	case model.ChartBar, model.ChartLine:
		if len(cfg.Metrics) < 1 {
			return fmt.Errorf("柱状/折线图至少需要一个指标")
		}
	case model.ChartTable:
		// any combination
	default:
		return fmt.Errorf("未知图表类型 %q", cfg.ChartType)
	}
	if cfg.DataMode == model.ModeRaw && cfg.ChartType != model.ChartTable {
		return fmt.Errorf("仅表格支持明细模式")
	}
	if cfg.DataMode != model.ModeRaw && cfg.DataMode != "" && cfg.DataMode != model.ModeAggregate {
		return fmt.Errorf("未知数据模式 %q", cfg.DataMode)
	}
	return nil
}
