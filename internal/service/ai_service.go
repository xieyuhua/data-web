package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"biaoge-bi/internal/ai"
	"biaoge-bi/internal/config"
	"biaoge-bi/internal/datasource"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/query"
	"biaoge-bi/internal/repository"
)

// AIService turns natural language into SQL and keeps dataset metadata tidy.
// It always operates read-only and never sees datasource credentials.
type AIService struct {
	repo    *repository.Repo
	manager *datasource.Manager
	runtime *config.Runtime
	cfg     *config.Config
}

func NewAIService(repo *repository.Repo, manager *datasource.Manager, rt *config.Runtime, cfg *config.Config) *AIService {
	return &AIService{repo: repo, manager: manager, runtime: rt, cfg: cfg}
}

func (s *AIService) cfgNow() *config.Config {
	if s.runtime != nil {
		return s.runtime.Get()
	}
	return s.cfg
}

func (s *AIService) client() *ai.Client {
	return ai.NewClient(s.cfgNow().AI)
}

// Status reports whether AI assistance is available.
type AIStatus struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
}

func (s *AIService) ensureEnabled() error {
	if !s.client().Enabled() {
		return ErrAIDisabled
	}
	return nil
}

func (s *AIService) Status() AIStatus {
	cfg := s.cfgNow().AI
	c := ai.NewClient(cfg)
	return AIStatus{Enabled: c.Enabled(), Model: cfg.Model, BaseURL: cfg.BaseURL}
}

// Ping verifies connectivity for the admin settings page.
func (s *AIService) Ping(ctx context.Context) error {
	return s.client().Ping(ctx)
}

// ErrAIDisabled is returned when AI assistance is not configured.
var ErrAIDisabled = errors.New("AI 未启用，请先在「系统配置 → AI 辅助」中完成配置")

const systemPrompt = `你是资深数据分析师，负责为自托管 BI 平台编写只读 SQL。
规则：
1. 只输出单条 SELECT / WITH 查询，绝不生成 INSERT/UPDATE/DELETE/DDL。
2. 参数用双花括号占位，例如 {{from_date}}、{{min_amount}}；参数名只用小写字母、数字、下划线。
3. 使用下方给定的表名与列名，不要臆造字段。
4. 只返回 SQL，不要解释、不要 markdown 代码块。`

// schemaContext renders table/column metadata for the prompt.
func (s *AIService) schemaContext(ctx context.Context, dataSourceID uint) (string, error) {
	cfg := s.cfgNow()
	if !cfg.AI.IncludeSchema {
		return "", nil
	}
	ds, err := s.repo.DataSourceByID(dataSourceID)
	if err != nil {
		return "", errors.New("数据源不存在")
	}
	db, dialect, err := s.manager.Get(ctx, ds.ID)
	if err != nil {
		return "", err
	}
	tables, err := listTables(ctx, db, dialect)
	if err != nil {
		return "", err
	}
	limit := cfg.AI.MaxSchemaTables
	if limit <= 0 {
		limit = 40
	}
	if len(tables) > limit {
		tables = tables[:limit]
	}

	var b strings.Builder
	for _, t := range tables {
		cols, err := listColumns(ctx, db, ds.Database, t, dialect)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "表 %s(%s)\n", t, strings.Join(cols, ", "))
	}
	if b.Len() == 0 {
		return "", errors.New("未能读取表结构，请检查数据源连接")
	}
	return b.String(), nil
}

// listTables returns user tables for the three supported dialects.
func listTables(ctx context.Context, db *sql.DB, dialect string) ([]string, error) {
	var q string
	switch dialect {
	case model.DSMySQL:
		q = `SELECT table_name FROM information_schema.tables
		     WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`
	case model.DSPostgres:
		q = `SELECT table_name FROM information_schema.tables
		     WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_type = 'BASE TABLE'
		     ORDER BY table_name`
	case model.DSOracle:
		q = `SELECT table_name FROM user_tables ORDER BY table_name`
	default:
		q = `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("读取表列表失败: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// listColumns returns "name type" pairs for one table.
func listColumns(ctx context.Context, db *sql.DB, dbName, table, dialect string) ([]string, error) {
	var q string
	var args []any
	switch dialect {
	case model.DSMySQL:
		q = `SELECT column_name, column_type FROM information_schema.columns
		     WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`
		args = []any{table}
	case model.DSPostgres:
		q = `SELECT column_name, data_type FROM information_schema.columns
		     WHERE table_name = $1 ORDER BY ordinal_position`
		args = []any{table}
	case model.DSOracle:
		q = `SELECT column_name, data_type FROM user_tab_columns
		     WHERE table_name = :1 ORDER BY column_id`
		args = []any{strings.ToUpper(table)}
	default:
		// SQLite has no information_schema; PRAGMA is safe here because the
		// table name comes from sqlite_master, not user input.
		quoted, err := query.QuoteIdentifier(table, dialect)
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoted+")")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dflt any
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err == nil {
				out = append(out, name+" "+typ)
			}
		}
		return out, rows.Err()
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err == nil {
			out = append(out, name+" "+typ)
		}
	}
	return out, rows.Err()
}

// GenerateSQL turns a natural-language requirement into a read-only SQL query.
func (s *AIService) GenerateSQL(ctx context.Context, dataSourceID uint, requirement, existingSQL string) (string, error) {
	if err := s.ensureEnabled(); err != nil {
		return "", err
	}
	if strings.TrimSpace(requirement) == "" {
		return "", errors.New("请描述你想查询的内容")
	}
	schema, err := s.schemaContext(ctx, dataSourceID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if schema != "" {
		b.WriteString("可用表结构：\n")
		b.WriteString(schema)
		b.WriteString("\n")
	}
	if strings.TrimSpace(existingSQL) != "" {
		b.WriteString("当前 SQL（可以在其基础上修改）：\n")
		b.WriteString(existingSQL)
		b.WriteString("\n")
	}
	b.WriteString("需求：")
	b.WriteString(requirement)
	b.WriteString("\n请只输出 SQL。")

	out, err := s.client().Chat(ctx, systemPrompt, b.String())
	if err != nil {
		return "", err
	}
	out = stripCodeFence(out)
	if err := query.ValidateReadOnly(out); err != nil {
		return "", fmt.Errorf("AI 生成的 SQL 未通过只读校验: %v", err)
	}
	return out, nil
}

// CompleteSQL continues the SQL at the cursor position.
func (s *AIService) CompleteSQL(ctx context.Context, dataSourceID uint, sqlText string, cursor int) (string, error) {
	if err := s.ensureEnabled(); err != nil {
		return "", err
	}
	if dataSourceID == 0 {
		return "", errors.New("请先选择数据源")
	}
	if cursor < 0 || cursor > len(sqlText) {
		cursor = len(sqlText)
	}
	before := sqlText[:cursor]
	schema, err := s.schemaContext(ctx, dataSourceID)
	if err != nil {
		return "", err
	}
	prompt := "已知表结构：\n" + schema + "\n\n已输入的 SQL：\n" + before +
		"\n\n请接着光标处继续补全 SQL（只输出后续片段，不要重复已输入内容，不要 markdown）。"
	out, err := s.client().Chat(ctx, systemPrompt+"\n现在只补全后续片段。", prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripCodeFence(out)), nil
}

// SuggestParams asks the AI to refine parameter definitions generated locally.
func (s *AIService) SuggestParams(ctx context.Context, sqlText string, current []DatasetParamInput) ([]ParamSuggestion, error) {
	if err := s.ensureEnabled(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sqlText) == "" {
		return nil, errors.New("SQL 不能为空")
	}
	cur, _ := json.Marshal(current)
	prompt := fmt.Sprintf("SQL：\n%s\n\n已有参数定义（JSON）：%s\n\n"+
		"请为 SQL 中的每个 {{占位符}} 给出参数定义，输出 JSON 数组，元素字段：\n"+
		"name, label(中文显示名), type(string|number|bool|date|list), required(bool), defaultValue(string)。\n"+
		"只输出 JSON 数组，不要 markdown。", sqlText, string(cur))

	out, err := s.client().Chat(ctx, "你是 BI 平台的数据建模助手，只输出合法 JSON。", prompt)
	if err != nil {
		return nil, err
	}
	var parsed []struct {
		Name         string `json:"name"`
		Label        string `json:"label"`
		Type         string `json:"type"`
		Required     bool   `json:"required"`
		DefaultValue string `json:"defaultValue"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &parsed); err != nil {
		return nil, fmt.Errorf("AI 返回的参数格式无法解析: %v", err)
	}
	// Only accept names that actually appear in the SQL, so a hallucinated
	// parameter can never enter the model.
	valid := map[string]bool{}
	for _, n := range query.ParamNames(sqlText) {
		valid[n] = true
	}
	result := make([]ParamSuggestion, 0, len(parsed))
	for _, p := range parsed {
		if !valid[strings.ToLower(p.Name)] {
			continue
		}
		typ := strings.ToLower(p.Type)
		switch typ {
		case "string", "number", "bool", "date", "list":
		default:
			typ = "string"
		}
		result = append(result, ParamSuggestion{
			Name: strings.ToLower(p.Name), Label: p.Label, Type: typ,
			Required: p.Required, DefaultValue: p.DefaultValue, Reason: "AI 生成",
		})
	}
	return result, nil
}

// SuggestMetadata proposes a name and description from SQL plus result columns.
func (s *AIService) SuggestMetadata(ctx context.Context, sqlText string, columns []model.ColumnMeta) (name, description string, err error) {
	if err := s.ensureEnabled(); err != nil {
		return "", "", err
	}
	colNames := make([]string, 0, len(columns))
	for _, c := range columns {
		colNames = append(colNames, c.Name)
	}
	prompt := fmt.Sprintf("SQL：\n%s\n\n结果列：%s\n\n"+
		"请为这个数据集起一个简短中文名称（不超过 20 字）和一句描述（不超过 60 字）。\n"+
		`只输出 JSON：{"name":"...","description":"..."}`, sqlText, strings.Join(colNames, ", "))

	out, err := s.client().Chat(ctx, "你是 BI 平台的数据资产管理员，只输出合法 JSON。", prompt)
	if err != nil {
		return "", "", err
	}
	var parsed struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &parsed); err != nil {
		return "", "", fmt.Errorf("AI 返回格式无法解析: %v", err)
	}
	return strings.TrimSpace(parsed.Name), strings.TrimSpace(parsed.Description), nil
}

// stripCodeFence removes a leading/trailing markdown fence some models add.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
	}
	if n := len(lines); n > 0 && strings.HasPrefix(strings.TrimSpace(lines[n-1]), "```") {
		lines = lines[:n-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// extractJSON pulls the first JSON array/object out of a model response.
func extractJSON(s string) string {
	s = stripCodeFence(s)
	start := strings.IndexAny(s, "[{")
	if start < 0 {
		return s
	}
	open := s[start]
	var close byte = ']'
	if open == '{' {
		close = '}'
	}
	if end := strings.LastIndexByte(s, close); end > start {
		return s[start : end+1]
	}
	return s[start:]
}
