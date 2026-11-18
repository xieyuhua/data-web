package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"biaoge-bi/internal/config"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/query"
	"biaoge-bi/internal/repository"
)

type ChartService struct {
	repo     *repository.Repo
	executor *query.Executor
	cache    *query.ResultCache
	limits   ChartLimits
	runtime  *config.Runtime
}

// ChartLimits carries configured query timeout and result row bounds.
type ChartLimits struct {
	Timeout time.Duration
	MaxRows int
}

// SetRuntime enables live limit changes from the admin settings page.
func (s *ChartService) SetRuntime(rt *config.Runtime) { s.runtime = rt }

// limitsNow returns the effective query bounds, preferring live config.
func (s *ChartService) limitsNow() ChartLimits {
	if s.runtime != nil {
		cfg := s.runtime.Get()
		maxRows := cfg.DataSource.MaxResultRows
		if maxRows <= 0 {
			maxRows = query.ChartMaxRows
		}
		return ChartLimits{Timeout: cfg.QueryTimeout(), MaxRows: maxRows}
	}
	return s.limits
}

func NewChartService(repo *repository.Repo, executor *query.Executor, cache *query.ResultCache, limits ChartLimits) *ChartService {
	if limits.Timeout <= 0 {
		limits.Timeout = 15 * time.Second
	}
	if limits.MaxRows <= 0 {
		limits.MaxRows = query.ChartMaxRows
	}
	return &ChartService{repo: repo, executor: executor, cache: cache, limits: limits}
}

// Dialect resolves the datasource SQL dialect for identifier quoting.
func (s *ChartService) Dialect(ctx context.Context, ds *model.Dataset) (string, error) {
	_, dialect, err := s.executor.Manager.Get(ctx, ds.DataSourceID)
	if err != nil {
		return "", err
	}
	return dialect, nil
}

type ChartInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	DatasetID   uint            `json:"datasetId"`
	Type        string          `json:"type"`
	Config      json.RawMessage `json:"config"`
}

func (s *ChartService) validate(in *ChartInput) (*model.ChartConfig, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("图表名称不能为空")
	}
	if in.DatasetID == 0 {
		return nil, errors.New("请选择数据集")
	}
	switch in.Type {
	case model.ChartBar, model.ChartLine, model.ChartPie, model.ChartTable, model.ChartKPI:
	default:
		return nil, errors.New("不支持的图表类型")
	}
	var cfg model.ChartConfig
	if len(in.Config) == 0 {
		return nil, errors.New("图表配置不能为空")
	}
	if err := json.Unmarshal(in.Config, &cfg); err != nil {
		return nil, errors.New("图表配置不是合法 JSON")
	}
	if cfg.ChartType == "" {
		cfg.ChartType = in.Type
	}
	if cfg.ChartType != in.Type {
		return nil, errors.New("图表类型与配置不一致")
	}
	if cfg.DataMode == "" {
		cfg.DataMode = model.ModeAggregate
	}
	return &cfg, nil
}

func (s *ChartService) Create(in ChartInput, userID uint) (*model.Chart, error) {
	if _, err := s.validate(&in); err != nil {
		return nil, err
	}
	c := model.Chart{
		Name: in.Name, Description: in.Description, DatasetID: in.DatasetID,
		Type: in.Type, ConfigJSON: string(in.Config), Version: 1, CreatedByID: userID,
	}
	if err := s.repo.DB.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ChartService) Update(id uint, in ChartInput) (*model.Chart, error) {
	if _, err := s.validate(&in); err != nil {
		return nil, err
	}
	c, err := s.repo.ChartByID(id)
	if err != nil {
		return nil, err
	}
	c.Name = in.Name
	c.Description = in.Description
	c.DatasetID = in.DatasetID
	c.Type = in.Type
	c.ConfigJSON = string(in.Config)
	c.Version++
	if err := s.repo.DB.Save(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ChartService) List(keyword string) ([]model.Chart, error) {
	return s.repo.ListCharts(keyword)
}

func (s *ChartService) Get(id uint) (*model.Chart, error) {
	return s.repo.ChartByID(id)
}

// Delete removes a chart, refusing while dashboards still reference it.
func (s *ChartService) Delete(id uint) error {
	var refs int64
	if err := s.repo.DB.Model(&model.DashboardWidget{}).Where("chart_id = ?", id).Count(&refs).Error; err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("该图表被 %d 个仪表盘组件引用，请先移除组件", refs)
	}
	return s.repo.DB.Delete(&model.Chart{}, id).Error
}

// Duplicate copies a chart definition.
func (s *ChartService) Duplicate(id uint, userID uint) (*model.Chart, error) {
	src, err := s.repo.ChartByID(id)
	if err != nil {
		return nil, err
	}
	c := model.Chart{
		Name: src.Name + "（副本）", Description: src.Description,
		DatasetID: src.DatasetID, Type: src.Type, ConfigJSON: src.ConfigJSON,
		Version: 1, CreatedByID: userID,
	}
	if err := s.repo.DB.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Execute runs a chart and returns its result set.
func (s *ChartService) Execute(ctx context.Context, chart *model.Chart, params map[string]string) (*query.ResultSet, error) {
	cfg, err := s.Config(chart)
	if err != nil {
		return nil, err
	}
	ds, err := s.repo.DatasetByID(chart.DatasetID)
	if err != nil {
		return nil, errors.New("图表引用的数据集不存在")
	}
	merged := mergeParams(cfg.ParamDefaults, params)

	key := query.CacheKey("chart", chart.ID, chart.Version, ds.Version, merged, cfg.DataMode, cfg.Limit)
	dialect, err := s.Dialect(ctx, ds)
	if err != nil {
		return nil, err
	}
	cached, _, err := s.cache.GetOrLoad(key, func() (*query.ResultSet, error) {
		cols, _ := s.datasetColumns(ds)
		sqlText, args, err := query.BuildChartSQL(ds, cfg, merged, dialect, cols)
		if err != nil {
			return nil, err
		}
		rs, err := s.executor.ExecuteRaw(ctx, ds.DataSourceID, sqlText, args, s.limits.MaxRows, s.limits.Timeout)
		if err != nil {
			return nil, err
		}
		applyMetricFormats(rs, cfg)
		return rs, nil
	})
	return cached, err
}

// applyMetricFormats tags result columns with the metric display format so the
// UI can render percentages and currency consistently with the configuration.
func applyMetricFormats(rs *query.ResultSet, cfg *model.ChartConfig) {
	if rs == nil {
		return
	}
	offset := len(cfg.Dimensions)
	for i, m := range cfg.Metrics {
		idx := offset + i
		if idx < 0 || idx >= len(rs.Columns) {
			continue
		}
		if m.Format != "" {
			rs.Columns[idx].Format = m.Format
		}
		if rs.Columns[idx].LogicalType == "string" {
			rs.Columns[idx].LogicalType = "number"
		}
	}
}

func (s *ChartService) datasetColumns(ds *model.Dataset) ([]model.ColumnMeta, error) {
	if ds.ColumnMetaJSON != "" {
		var cols []model.ColumnMeta
		if err := json.Unmarshal([]byte(ds.ColumnMetaJSON), &cols); err == nil {
			return cols, nil
		}
	}
	return nil, errors.New("数据集尚未推断列信息")
}

func (s *ChartService) Config(c *model.Chart) (*model.ChartConfig, error) {
	var cfg model.ChartConfig
	if err := json.Unmarshal([]byte(c.ConfigJSON), &cfg); err != nil {
		return nil, errors.New("图表配置损坏")
	}
	return &cfg, nil
}

func mergeParams(defaults map[string]string, override map[string]string) map[string]string {
	out := make(map[string]string, len(defaults)+len(override))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
