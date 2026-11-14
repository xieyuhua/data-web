package sqlguard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)
	filterPattern      = regexp.MustCompile(`\{\{\s*filter:([a-zA-Z0-9_]{1,32})\s*\}\}`)
	filterInPattern    = regexp.MustCompile(`\{\{\s*filter_in:([a-zA-Z0-9_]{1,32})\s*\}\}`)
	filterHasPattern   = regexp.MustCompile(`\{\{\s*filter_has:([a-zA-Z0-9_]{1,32})\s*\}\}`)
	filterKeyPattern   = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)
	selectPattern      = regexp.MustCompile(`(?is)^\s*(select|with)\b`)
	dangerousPattern   = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|truncate|alter|create|attach|detach|pragma|vacuum|replace|grant|revoke|exec|execute|call|copy|load_file|outfile|dumpfile|sleep|benchmark|shutdown)\b`)
	// Oracle 高危系统包调用（DBMS_* / UTL_* / OWA_* / XDB_*），只拦截函数调用形式，避免误伤普通字段名。
	oraclePackagePattern = regexp.MustCompile(`(?i)\b(dbms_[a-z0-9_]+|utl_[a-z0-9_]+|owa_[a-z0-9_]+|xdb_[a-z0-9_]+)(\.[a-z0-9_]+)*\s*\(`)
)

// MaxRows 是单次查询允许返回的最大行数。
const MaxRows = 2000

// MaxFilterValueLength 限制单个筛选值的长度，避免超长字符串拖慢查询。
const MaxFilterValueLength = 200

// FilterValueSeparator 用于在单个 filters 字段中编码多选值。
// 使用不可见的分隔符，避免与业务值中的逗号、分号等字符冲突。
const FilterValueSeparator = "\x1f"

type Params struct {
	OrgIDs    []uint64
	StartDate string
	EndDate   string
	// Filters 是看板级全局筛选器的取值，用于展开 {{filter:key}} 占位符。
	// 值会被安全转义为 SQL 字符串字面量；未提供时按空字符串处理。
	// 多选值使用 FilterValueSeparator 连接，配合 {{filter_in:key}} 使用。
	Filters map[string]string
}

type Result struct {
	SQL    string
	Params map[string]any
}

// Render validates a read-only SQL statement and safely expands the supported placeholders.
// It deliberately rejects multiple statements and SQL comments rather than trying to parse them.
// 默认按 MySQL 兼容方言生成分页语句，需要区分数据库时使用 RenderFor。
func Render(raw string, requireScope bool, params Params) (Result, error) {
	return RenderFor("", raw, requireScope, params)
}

// RenderFor 与 Render 相同，但会按目标数据库（mysql/postgres/sqlserver/sqlite/oracle）
// 生成各自支持的行数限制语法，避免 Oracle 因 LIMIT 语法报错。
func RenderFor(driver, raw string, requireScope bool, params Params) (Result, error) {
	sqlText := strings.TrimSpace(raw)
	if sqlText == "" {
		return Result{}, fmt.Errorf("SQL 不能为空")
	}
	if strings.HasSuffix(sqlText, ";") {
		sqlText = strings.TrimSpace(strings.TrimSuffix(sqlText, ";"))
	}
	if strings.Contains(sqlText, ";") {
		return Result{}, fmt.Errorf("不允许执行多条 SQL")
	}
	if strings.Contains(sqlText, "--") || strings.Contains(sqlText, "/*") || strings.Contains(sqlText, "*/") {
		return Result{}, fmt.Errorf("SQL 中不允许使用注释")
	}
	if !selectPattern.MatchString(sqlText) {
		return Result{}, fmt.Errorf("只允许 SELECT 或 WITH 查询")
	}
	if dangerousPattern.MatchString(sqlText) {
		return Result{}, fmt.Errorf("SQL 包含不允许的关键字")
	}
	if oraclePackagePattern.MatchString(sqlText) {
		return Result{}, fmt.Errorf("SQL 包含不允许的 Oracle 系统包调用")
	}
	if requireScope && !strings.Contains(sqlText, "{{org_scope}}") {
		return Result{}, fmt.Errorf("启用数据权限的数据集必须包含 {{org_scope}} 占位符")
	}
	if params.StartDate == "" || params.EndDate == "" {
		return Result{}, fmt.Errorf("日期范围不能为空")
	}
	start, err := time.Parse("2006-01-02", params.StartDate)
	if err != nil {
		return Result{}, fmt.Errorf("开始日期格式不正确")
	}
	end, err := time.Parse("2006-01-02", params.EndDate)
	if err != nil {
		return Result{}, fmt.Errorf("结束日期格式不正确")
	}
	if start.After(end) {
		return Result{}, fmt.Errorf("开始日期不能晚于结束日期")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return Result{}, fmt.Errorf("单次查询时间跨度不能超过 366 天")
	}

	// 先展开全局筛选器占位符，再校验剩余占位符，避免 {{filter:xxx}} 被误判为非法格式。
	if err := validateFilters(params.Filters); err != nil {
		return Result{}, err
	}
	sqlText = filterHasPattern.ReplaceAllStringFunc(sqlText, func(match string) string {
		parts := filterHasPattern.FindStringSubmatch(match)
		if len(SplitFilterValues(params.Filters[parts[1]])) > 0 {
			return "1"
		}
		return "0"
	})
	sqlText = filterInPattern.ReplaceAllStringFunc(sqlText, func(match string) string {
		parts := filterInPattern.FindStringSubmatch(match)
		values := SplitFilterValues(params.Filters[parts[1]])
		if len(values) == 0 {
			// 空选择展开为空字符串字面量，配合 {{filter_has:key}} = 0 判断“未筛选”。
			return "''"
		}
		quoted := make([]string, 0, len(values))
		for _, value := range values {
			quoted = append(quoted, quoteLiteral(value))
		}
		return strings.Join(quoted, ",")
	})
	sqlText = filterPattern.ReplaceAllStringFunc(sqlText, func(match string) string {
		parts := filterPattern.FindStringSubmatch(match)
		values := SplitFilterValues(params.Filters[parts[1]])
		if len(values) == 0 {
			return "''"
		}
		// 单选占位符遇到多选值时取第一个值，保持可预期的行为。
		return quoteLiteral(values[0])
	})

	allowed := map[string]bool{
		"org_scope":  true,
		"start_date": true,
		"end_date":   true,
	}
	matches := placeholderPattern.FindAllStringSubmatch(sqlText, -1)
	for _, match := range matches {
		if !allowed[match[1]] {
			return Result{}, fmt.Errorf("不支持的 SQL 占位符: %s", match[1])
		}
	}
	if strings.Contains(sqlText, "{{") || strings.Contains(sqlText, "}}") {
		// Reject malformed placeholders that the regexp cannot safely expand.
		cleaned := placeholderPattern.ReplaceAllString(sqlText, "")
		if strings.Contains(cleaned, "{{") || strings.Contains(cleaned, "}}") {
			return Result{}, fmt.Errorf("SQL 占位符格式不正确")
		}
	}

	scope := orgScope(params.OrgIDs)
	sqlText = placeholderPattern.ReplaceAllStringFunc(sqlText, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		switch parts[1] {
		case "org_scope":
			return scope
		case "start_date":
			return "'" + params.StartDate + "'"
		case "end_date":
			return "'" + params.EndDate + "'"
		default:
			return match
		}
	})

	return Result{
		SQL: applyRowLimit(sqlText, driver),
		Params: map[string]any{
			"org_count": len(params.OrgIDs),
			"start":     params.StartDate,
			"end":       params.EndDate,
		},
	}, nil
}

// validateFilters 校验全局筛选器取值，防止超长或控制字符进入 SQL。
func validateFilters(filters map[string]string) error {
	if len(filters) > 16 {
		return fmt.Errorf("筛选条件数量不能超过 16 个")
	}
	for key, value := range filters {
		if !filterKeyPattern.MatchString(key) {
			return fmt.Errorf("筛选条件名称不合法: %s", key)
		}
		values := SplitFilterValues(value)
		if len(values) > 50 {
			return fmt.Errorf("筛选条件 %s 的取值数量不能超过 50 个", key)
		}
		for _, item := range values {
			if len([]rune(item)) > MaxFilterValueLength {
				return fmt.Errorf("筛选条件 %s 的取值过长", key)
			}
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("筛选条件 %s 的取值包含非法字符", key)
		}
	}
	return nil
}

// SplitFilterValues 将单个筛选字段拆分为多选值列表。
func SplitFilterValues(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, FilterValueSeparator)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

// JoinFilterValues 将多选值编码为单个筛选字段。
func JoinFilterValues(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return strings.Join(cleaned, FilterValueSeparator)
}

// quoteLiteral 将筛选值转成标准 SQL 字符串字面量。
// 同时转义反斜杠与单引号，兼容 MySQL（默认反斜杠转义）与标准 SQL 方言。
func quoteLiteral(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `''`)
	return "'" + escaped + "'"
}

// applyRowLimit 按数据库方言包裹子查询并限制返回行数。
func applyRowLimit(sqlText, driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "oracle":
		// Oracle 不支持 LIMIT，也不能为派生表加 AS 关键字。
		return fmt.Sprintf("SELECT * FROM (%s) bi_query WHERE ROWNUM <= %d", sqlText, MaxRows)
	case "sqlserver":
		return fmt.Sprintf("SELECT TOP %d * FROM (%s) AS bi_query", MaxRows, sqlText)
	default:
		return fmt.Sprintf("SELECT * FROM (%s) AS bi_query LIMIT %d", sqlText, MaxRows)
	}
}

func orgScope(ids []uint64) string {
	if len(ids) == 0 {
		return "-1"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatUint(id, 10))
		}
	}
	if len(parts) == 0 {
		return "-1"
	}
	return strings.Join(parts, ",")
}
