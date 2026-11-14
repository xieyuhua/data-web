package service

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/sqlguard"
	"bi-platform/internal/repository"

	"gorm.io/gorm"
)

// QueryResult 数据集查询结果：列名 + 行数据 + 耗时。
type QueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Count     int      `json:"count"`
	ElapsedMS int64    `json:"elapsed_ms"`
	Cached    bool     `json:"cached,omitempty"`
}

// QueryOptions 控制单次查询行为。看板手动刷新可以跳过缓存，定时刷新默认复用短时缓存。
type QueryOptions struct {
	NoCache bool
	// Filters 是看板级全局筛选器取值，用于展开 SQL 中的 {{filter:key}} 占位符。
	Filters map[string]string
	// ShareMode 表示当前查询来自分享链接；分享模式会隐藏配置为“不在分享页显示”的筛选器。
	ShareMode bool
}

const (
	datasetCacheTTL        = 20 * time.Second
	datasetCacheMaxEntries = 256
)

type datasetCacheItem struct {
	result    *QueryResult
	expiresAt time.Time
}

// DatasetService 数据集服务：负责 SQL 校验、权限注入、执行与短时缓存。
type DatasetService struct {
	repo    *repository.DatasetRepository
	sources *DatasourceService
	timeout time.Duration
	cacheMu sync.Mutex
	cache   map[string]datasetCacheItem
}

// NewDatasetService 创建数据集服务。
func NewDatasetService(repo *repository.DatasetRepository, sources *DatasourceService, timeout time.Duration) *DatasetService {
	return &DatasetService{repo: repo, sources: sources, timeout: timeout, cache: make(map[string]datasetCacheItem)}
}

// List 查询数据集列表。
func (s *DatasetService) List() ([]model.Dataset, error) { return s.repo.List() }

// Create 新增数据集（校验必填字段与 SQL 安全性）。
func (s *DatasetService) Create(item *model.Dataset) error {
	if item.Name == "" || item.Code == "" || item.SQLText == "" {
		return fmt.Errorf("数据集名称、编码和 SQL 不能为空")
	}
	if item.ScopeColumn == "" {
		item.ScopeColumn = "org_id"
	}
	if item.Status == "" {
		item.Status = "active"
	}
	driver, _ := s.driverOf(item.DatasourceID)
	if _, err := sqlguard.RenderFor(driver, item.SQLText, item.RequireScope, sqlguard.Params{
		OrgIDs: []uint64{1}, StartDate: time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
		EndDate: time.Now().Format("2006-01-02"),
	}); err != nil {
		return err
	}
	return s.repo.Create(item)
}

// Update 更新数据集并清理缓存。
func (s *DatasetService) Update(item *model.Dataset) error {
	if item.ID == 0 {
		return fmt.Errorf("数据集 ID 不能为空")
	}
	driver, _ := s.driverOf(item.DatasourceID)
	if _, err := sqlguard.RenderFor(driver, item.SQLText, item.RequireScope, sqlguard.Params{
		OrgIDs: []uint64{1}, StartDate: time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
		EndDate: time.Now().Format("2006-01-02"),
	}); err != nil {
		return err
	}
	if err := s.repo.Update(item); err != nil {
		return err
	}
	s.invalidateDatasetCache(item.ID)
	return nil
}

// Delete 删除数据集并清理相关缓存。
func (s *DatasetService) Delete(id uint64) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.invalidateDatasetCache(id)
	return nil
}

// Query 执行数据集查询，自动注入组织数据权限。
func (s *DatasetService) Query(ctx context.Context, dataset *model.Dataset, orgIDs []uint64, startDate, endDate string) (*QueryResult, error) {
	return s.QueryWithOptions(ctx, dataset, orgIDs, startDate, endDate, QueryOptions{})
}

// QueryWithOptions 在 Query 基础上提供短时结果缓存，避免同一看板内多个组件重复查询。
func (s *DatasetService) QueryWithOptions(ctx context.Context, dataset *model.Dataset, orgIDs []uint64, startDate, endDate string, options QueryOptions) (*QueryResult, error) {
	cacheKey := s.cacheKey(dataset, orgIDs, startDate, endDate, options.Filters)
	if !options.NoCache && dataset.ID > 0 {
		if cached := s.cachedResult(cacheKey); cached != nil {
			return cached, nil
		}
	}
	db, driver, err := s.database(dataset.DatasourceID)
	if err != nil {
		return nil, err
	}
	rendered, err := sqlguard.RenderFor(driver, dataset.SQLText, dataset.RequireScope, sqlguard.Params{
		OrgIDs: orgIDs, StartDate: startDate, EndDate: endDate, Filters: options.Filters,
	})
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	started := time.Now()
	result, err := s.scanRows(queryCtx, db, rendered.SQL)
	if err != nil {
		return nil, err
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	if dataset.ID > 0 {
		s.storeCache(cacheKey, result)
	}
	return result, nil
}

func (s *DatasetService) cacheKey(dataset *model.Dataset, orgIDs []uint64, startDate, endDate string, filters map[string]string) string {
	ids := append([]uint64(nil), orgIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(id, 10))
	}
	filterParts := make([]string, 0, len(filters))
	filterKeys := make([]string, 0, len(filters))
	for key := range filters {
		filterKeys = append(filterKeys, key)
	}
	sort.Strings(filterKeys)
	for _, key := range filterKeys {
		filterParts = append(filterParts, key+"="+filters[key])
	}
	datasourceID := uint64(0)
	if dataset.DatasourceID != nil {
		datasourceID = *dataset.DatasourceID
	}
	return fmt.Sprintf("%d|%d|%d|%t|%s|%s|%s|%s",
		dataset.ID, datasourceID, dataset.UpdatedAt.UnixNano(), dataset.RequireScope,
		startDate, endDate, strings.Join(parts, ","), strings.Join(filterParts, "&"))
}

func (s *DatasetService) cachedResult(key string) *QueryResult {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	item, ok := s.cache[key]
	if !ok {
		return nil
	}
	if time.Now().After(item.expiresAt) {
		delete(s.cache, key)
		return nil
	}
	copied := *item.result
	copied.Cached = true
	copied.ElapsedMS = 0
	return &copied
}

func (s *DatasetService) storeCache(key string, result *QueryResult) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if len(s.cache) >= datasetCacheMaxEntries {
		now := time.Now()
		for itemKey, item := range s.cache {
			if now.After(item.expiresAt) {
				delete(s.cache, itemKey)
			}
		}
		// 极端情况下仍超过上限时直接清空，保证内存不会无限增长。
		if len(s.cache) >= datasetCacheMaxEntries {
			s.cache = make(map[string]datasetCacheItem)
		}
	}
	s.cache[key] = datasetCacheItem{result: result, expiresAt: time.Now().Add(datasetCacheTTL)}
}

func (s *DatasetService) invalidateDatasetCache(datasetID uint64) {
	if datasetID == 0 {
		return
	}
	prefix := strconv.FormatUint(datasetID, 10) + "|"
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	for key := range s.cache {
		if strings.HasPrefix(key, prefix) {
			delete(s.cache, key)
		}
	}
}

// PreviewDraft 直接执行尚未保存的 SQL 草稿，用于新建 / 编辑数据模型时的即时预览。
func (s *DatasetService) PreviewDraft(ctx context.Context, dataset *model.Dataset, orgIDs []uint64, startDate, endDate string) (*QueryResult, error) {
	if strings.TrimSpace(dataset.SQLText) == "" {
		return nil, fmt.Errorf("请填写 SQL 查询")
	}
	if dataset.ScopeColumn == "" {
		dataset.ScopeColumn = "org_id"
	}
	return s.Query(ctx, dataset, orgIDs, startDate, endDate)
}

// driverOf 返回数据集绑定数据源的驱动名，未绑定或查询失败时返回空字符串（按默认方言处理）。
func (s *DatasetService) driverOf(datasourceID *uint64) (string, error) {
	if datasourceID == nil || *datasourceID == 0 || s.sources == nil {
		return "", nil
	}
	return s.sources.Driver(*datasourceID)
}

func (s *DatasetService) database(datasourceID *uint64) (*gorm.DB, string, error) {
	db := s.repo.DB()
	if datasourceID != nil && *datasourceID > 0 {
		if s.sources == nil {
			return nil, "", fmt.Errorf("数据源服务未初始化")
		}
		driver, err := s.sources.Driver(*datasourceID)
		if err != nil {
			return nil, "", err
		}
		conn, err := s.sources.DB(*datasourceID)
		if err != nil {
			return nil, "", err
		}
		return conn, driver, nil
	}
	return db, "", nil
}

func (s *DatasetService) scanRows(queryCtx context.Context, db *gorm.DB, sqlText string) (*QueryResult, error) {
	rows, err := db.WithContext(queryCtx).Raw(sqlText).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	// CLOB / LONG 等大文本列需要扫描到 string，go-ora 会在扫描时展开 LOB 内容。
	lobColumns := make([]bool, len(columns))
	for i := 0; i < len(columns) && i < len(columnTypes); i++ {
		columnType := columnTypes[i]
		switch strings.ToUpper(strings.TrimSpace(columnType.DatabaseTypeName())) {
		case "CLOB", "NCLOB", "LONG", "LONG RAW", "LONGVARCHAR", "LONG VARBINARY":
			lobColumns[i] = true
		}
	}
	result := &QueryResult{Columns: columns, Rows: make([][]any, 0)}
	for rows.Next() {
		if len(result.Rows) >= sqlguard.MaxRows {
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			if lobColumns[i] {
				pointers[i] = new(string)
			} else {
				pointers[i] = &values[i]
			}
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		for i := range values {
			if lobColumns[i] {
				values[i] = *(pointers[i].(*string))
			}
			var columnType *sql.ColumnType
			if i < len(columnTypes) {
				columnType = columnTypes[i]
			}
			values[i] = normalizeValue(values[i], columnType)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result.Count = len(result.Rows)
	return result, nil
}

// Preview 预览已保存数据集，默认最近 7 天。
func (s *DatasetService) Preview(ctx context.Context, id uint64, orgIDs []uint64, startDate, endDate string) (*QueryResult, error) {
	dataset, err := s.repo.FindByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("数据集不存在")
		}
		return nil, err
	}
	return s.Query(ctx, dataset, orgIDs, startDate, endDate)
}

func normalizeValue(value any, columnType *sql.ColumnType) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format("2006-01-02 15:04:05")
	case string:
		// go-ora 会把 Oracle NUMBER 以字符串返回，这里按列的扫描类型还原为数值，
		// 否则前端无法识别指标列。
		if number, ok := parseNumericString(typed, columnType); ok {
			return number
		}
		return typed
	default:
		return typed
	}
}

func parseNumericString(value string, columnType *sql.ColumnType) (any, bool) {
	if columnType == nil || strings.TrimSpace(value) == "" {
		return nil, false
	}
	scanType := columnType.ScanType()
	if scanType == nil {
		return nil, false
	}
	switch scanType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed, true
		}
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed, true
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
			return parsed, true
		}
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed, true
		}
	case reflect.Float32, reflect.Float64:
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed, true
		}
	}
	return nil, false
}
