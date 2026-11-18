package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/query"
	"biaoge-bi/internal/repository"
)

var paramNameRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

type DatasetService struct {
	repo     *repository.Repo
	executor *query.Executor
}

func NewDatasetService(repo *repository.Repo, executor *query.Executor) *DatasetService {
	return &DatasetService{repo: repo, executor: executor}
}

type DatasetParamInput struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	DefaultValue string `json:"defaultValue"`
}

type DatasetInput struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	DataSourceID uint                `json:"dataSourceId"`
	SQLText      string              `json:"sqlText"`
	DefaultLimit int                 `json:"defaultLimit"`
	Params       []DatasetParamInput `json:"params"`
}

func (s *DatasetService) validate(in *DatasetInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("数据集名称不能为空")
	}
	if in.DataSourceID == 0 {
		return errors.New("请选择数据源")
	}
	if strings.TrimSpace(in.SQLText) == "" {
		return errors.New("SQL 不能为空")
	}
	if err := query.ValidateReadOnly(in.SQLText); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range in.Params {
		p := &in.Params[i]
		p.Name = strings.ToLower(strings.TrimSpace(p.Name))
		if !paramNameRe.MatchString(p.Name) {
			return errors.New("参数名只能包含小写字母、数字和下划线，且以字母或下划线开头")
		}
		if seen[p.Name] {
			return errors.New("参数名重复: " + p.Name)
		}
		seen[p.Name] = true
		switch p.Type {
		case "", "string", "number", "bool", "date", "list":
		default:
			return errors.New("不支持的参数类型: " + p.Type)
		}
		if p.Type == "" {
			p.Type = "string"
		}
	}
	if in.DefaultLimit <= 0 {
		in.DefaultLimit = 1000
	}
	if in.DefaultLimit > 5000 {
		in.DefaultLimit = 5000
	}

	// Every {{placeholder}} in the SQL must have a declared parameter, so missing
	// values fail fast at save time instead of at query time.
	refs := query.ParamNames(in.SQLText)
	for _, name := range refs {
		if !seen[name] {
			return errors.New("SQL 中使用了未声明的参数 {{" + name + "}}")
		}
	}
	return nil
}

func (s *DatasetService) Create(ctx context.Context, in DatasetInput, userID uint) (*model.Dataset, error) {
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	ds := model.Dataset{
		Name:         in.Name,
		Description:  in.Description,
		DataSourceID: in.DataSourceID,
		SQLText:      in.SQLText,
		DefaultLimit: in.DefaultLimit,
		CreatedByID:  userID,
		Version:      1,
	}
	for _, p := range in.Params {
		ds.Params = append(ds.Params, model.DatasetParam{
			Name: p.Name, Label: p.Label, Type: p.Type, Required: p.Required, DefaultValue: p.DefaultValue,
		})
	}
	if err := s.repo.DB.Create(&ds).Error; err != nil {
		return nil, err
	}
	// Infer columns lazily; failure must not block saving.
	if cols, err := s.InferColumns(ctx, &ds); err == nil {
		raw, _ := json.Marshal(cols)
		_ = s.repo.DB.Model(&ds).Update("column_meta_json", string(raw)).Error
		ds.ColumnMetaJSON = string(raw)
	}
	return &ds, nil
}

func (s *DatasetService) Update(ctx context.Context, id uint, in DatasetInput) (*model.Dataset, error) {
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	ds, err := s.repo.DatasetByID(id)
	if err != nil {
		return nil, err
	}
	ds.Name = in.Name
	ds.Description = in.Description
	ds.DataSourceID = in.DataSourceID
	ds.SQLText = in.SQLText
	ds.DefaultLimit = in.DefaultLimit
	ds.Version++

	if err := s.repo.DB.Save(ds).Error; err != nil {
		return nil, err
	}
	if err := s.repo.DB.Where("dataset_id = ?", id).Delete(&model.DatasetParam{}).Error; err != nil {
		return nil, err
	}
	var params []model.DatasetParam
	for _, p := range in.Params {
		params = append(params, model.DatasetParam{
			DatasetID: id, Name: p.Name, Label: p.Label, Type: p.Type,
			Required: p.Required, DefaultValue: p.DefaultValue,
		})
	}
	if len(params) > 0 {
		if err := s.repo.DB.Create(&params).Error; err != nil {
			return nil, err
		}
	}
	ds.Params = params
	if cols, err := s.InferColumns(ctx, ds); err == nil {
		raw, _ := json.Marshal(cols)
		_ = s.repo.DB.Model(&model.Dataset{}).Where("id = ?", id).Update("column_meta_json", string(raw)).Error
		ds.ColumnMetaJSON = string(raw)
	}
	return ds, nil
}

func (s *DatasetService) List(keyword string) ([]model.Dataset, error) {
	return s.repo.ListDatasets(keyword)
}

func (s *DatasetService) Get(id uint) (*model.Dataset, error) {
	return s.repo.DatasetByID(id)
}

// Delete removes a dataset, refusing while charts or filters reference it.
func (s *DatasetService) Delete(id uint) error {
	var chartRefs int64
	if err := s.repo.DB.Model(&model.Chart{}).Where("dataset_id = ?", id).Count(&chartRefs).Error; err != nil {
		return err
	}
	var filterRefs int64
	if err := s.repo.DB.Model(&model.DashboardFilter{}).Where("dataset_id = ?", id).Count(&filterRefs).Error; err != nil {
		return err
	}
	if chartRefs > 0 || filterRefs > 0 {
		return fmt.Errorf("该数据集被 %d 个图表、%d 个筛选器引用，请先解除引用", chartRefs, filterRefs)
	}
	return s.repo.DB.Delete(&model.Dataset{}, id).Error
}

// InferColumns runs the dataset with defaults and derives column metadata.
func (s *DatasetService) InferColumns(ctx context.Context, ds *model.Dataset) ([]model.ColumnMeta, error) {
	rs, err := s.executor.ExecuteDataset(ctx, query.ExecuteOptions{
		Dataset: ds, MaxRows: 1, Timeout: 10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return rs.Columns, nil
}

type PreviewRequest struct {
	Params map[string]string `json:"params"`
	Limit  int               `json:"limit"`
}

func (s *DatasetService) Preview(ctx context.Context, id uint, req PreviewRequest) (*query.ResultSet, error) {
	ds, err := s.repo.DatasetByID(id)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > query.PreviewMaxRows {
		limit = query.PreviewMaxRows
	}
	return s.executor.ExecuteDataset(ctx, query.ExecuteOptions{
		Dataset: ds, Params: req.Params, MaxRows: limit, Timeout: 15 * time.Second,
	})
}

// ParamSuggestion is a parameter definition derived from the SQL text.
type ParamSuggestion struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	DefaultValue string `json:"defaultValue"`
	Reason       string `json:"reason,omitempty"`
}

// InferParams scans {{placeholders}} in the SQL and suggests definitions.
// Existing params are preserved so re-running never overwrites user edits.
func (s *DatasetService) InferParams(sqlText string, existing []DatasetParamInput) []ParamSuggestion {
	seen := make(map[string]DatasetParamInput, len(existing))
	for _, p := range existing {
		seen[strings.ToLower(p.Name)] = p
	}
	names := query.ParamNames(sqlText)
	out := make([]ParamSuggestion, 0, len(names))
	for _, name := range names {
		if p, ok := seen[name]; ok {
			out = append(out, ParamSuggestion{
				Name: p.Name, Label: p.Label, Type: p.Type,
				Required: p.Required, DefaultValue: p.DefaultValue, Reason: "已存在",
			})
			continue
		}
		typ, label, required, def := guessParam(name, sqlText)
		out = append(out, ParamSuggestion{
			Name: name, Label: label, Type: typ, Required: required,
			DefaultValue: def, Reason: "自动推断",
		})
	}
	return out
}

// guessParam infers a parameter's type from its name and the SQL context.
func guessParam(name, sqlText string) (typ, label string, required bool, def string) {
	lower := strings.ToLower(name)
	typ = "string"
	required = true

	// Context: the placeholder's surrounding comparison often reveals the type.
	idx := strings.Index(strings.ToLower(sqlText), "{{"+lower+"}}")
	ctx := ""
	if idx >= 0 {
		start := idx - 40
		if start < 0 {
			start = 0
		}
		end := idx + len(lower) + 40
		if end > len(sqlText) {
			end = len(sqlText)
		}
		ctx = strings.ToLower(sqlText[start:end])
	}

	switch {
	case strings.Contains(lower, "date"), strings.Contains(lower, "day"),
		strings.Contains(lower, "time"), strings.HasSuffix(lower, "_at"),
		strings.Contains(lower, "month"), strings.Contains(lower, "start"), strings.Contains(lower, "end"),
		strings.Contains(ctx, "date") && strings.Contains(ctx, ">="):
		typ = "date"
	case strings.Contains(lower, "amount"), strings.Contains(lower, "count"),
		strings.Contains(lower, "price"), strings.Contains(lower, "qty"),
		strings.Contains(lower, "num"), strings.Contains(lower, "rate"),
		strings.Contains(lower, "min"), strings.Contains(lower, "max"),
		strings.Contains(lower, "top"), strings.Contains(lower, "limit"):
		typ = "number"
	case strings.HasPrefix(lower, "is_"), strings.HasPrefix(lower, "has_"):
		typ = "bool"
	}

	if strings.Contains(lower, "limit") || strings.Contains(lower, "top") ||
		strings.Contains(lower, "size") || strings.Contains(lower, "count") {
		required = false
	}

	// Human-friendly label: snake_case -> "首字母大写 + 空格".
	label = humanize(name)
	switch typ {
	case "date":
		if def == "" && (strings.Contains(lower, "start") || strings.Contains(lower, "from")) {
			def = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
		}
	case "number":
		if strings.Contains(lower, "limit") || strings.Contains(lower, "top") {
			def = "100"
		}
	}
	return typ, label, required, def
}

func humanize(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// TestSQL executes an unsaved SQL against a datasource without persisting
// anything, returning columns and sample rows for the editor.
type TestSQLRequest struct {
	DataSourceID uint              `json:"dataSourceId"`
	SQLText      string            `json:"sqlText"`
	Params       []DatasetParamInput `json:"params"`
	ParamValues  map[string]string `json:"paramValues"`
	Limit        int               `json:"limit"`
}

type TestSQLResult struct {
	*query.ResultSet
	Params []ParamSuggestion `json:"paramSuggestions"`
	ElapsedMs int64 `json:"elapsedMs"`
}

func (s *DatasetService) TestSQL(ctx context.Context, req TestSQLRequest) (*TestSQLResult, error) {
	if req.DataSourceID == 0 {
		return nil, errors.New("请选择数据源")
	}
	if strings.TrimSpace(req.SQLText) == "" {
		return nil, errors.New("SQL 不能为空")
	}
	if err := query.ValidateReadOnly(req.SQLText); err != nil {
		return nil, err
	}
	suggestions := s.InferParams(req.SQLText, req.Params)

	// Build a throwaway dataset so the normal executor path (guard, binding,
	// row caps) applies exactly as it will at runtime.
	tmp := &model.Dataset{
		DataSourceID: req.DataSourceID,
		SQLText:      req.SQLText,
	}
	values := map[string]string{}
	for _, sg := range suggestions {
		tmp.Params = append(tmp.Params, model.DatasetParam{
			Name: sg.Name, Type: sg.Type, Required: sg.Required, DefaultValue: sg.DefaultValue,
		})
		if v, ok := req.ParamValues[sg.Name]; ok && v != "" {
			values[sg.Name] = v
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > query.PreviewMaxRows {
		limit = query.PreviewMaxRows
	}
	start := time.Now()
	rs, err := s.executor.ExecuteDataset(ctx, query.ExecuteOptions{
		Dataset: tmp, Params: values, MaxRows: limit, Timeout: 15 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &TestSQLResult{
		ResultSet: rs,
		Params:    suggestions,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *DatasetService) Columns(id uint) ([]model.ColumnMeta, error) {
	ds, err := s.repo.DatasetByID(id)
	if err != nil {
		return nil, err
	}
	if ds.ColumnMetaJSON != "" {
		var cols []model.ColumnMeta
		if err := json.Unmarshal([]byte(ds.ColumnMetaJSON), &cols); err == nil && len(cols) > 0 {
			return cols, nil
		}
	}
	return nil, errors.New("尚未推断列信息，请先保存或预览数据集")
}
