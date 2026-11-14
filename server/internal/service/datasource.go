package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/secretbox"
	"bi-platform/internal/repository"

	"github.com/glebarez/sqlite"
	_ "github.com/sijms/go-ora/v2"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var SupportedDrivers = []string{"mysql", "postgres", "sqlserver", "oracle", "sqlite"}

// DatasourceInput 数据源新建 / 更新入参。
type DatasourceInput struct {
	Name         string `json:"name"`
	Code         string `json:"code"`
	Driver       string `json:"driver"`
	DSN          string `json:"dsn"`
	Status       string `json:"status"`
	ReadOnly     bool   `json:"read_only"`
	MaxOpenConns int    `json:"max_open_conns"`
	MaxIdleConns int    `json:"max_idle_conns"`
	Remark       string `json:"remark"`
}

// DatasourceView 数据源对外视图（DSN 已脱敏）。
type DatasourceView struct {
	ID           uint64     `json:"id"`
	Name         string     `json:"name"`
	Code         string     `json:"code"`
	Driver       string     `json:"driver"`
	DSNMasked    string     `json:"dsn_masked"`
	Status       string     `json:"status"`
	ReadOnly     bool       `json:"read_only"`
	MaxOpenConns int        `json:"max_open_conns"`
	MaxIdleConns int        `json:"max_idle_conns"`
	Remark       string     `json:"remark"`
	LastTestAt   *time.Time `json:"last_test_at"`
	LastTestOK   bool       `json:"last_test_ok"`
	LastTestMsg  string     `json:"last_test_msg"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// DatasourceTestResult 数据源连通性测试结果。
type DatasourceTestResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

// DatasourceService 数据源服务：负责连接池缓存、DSN 加解密与连通性测试。
type DatasourceService struct {
	repo    *repository.DatasourceRepository
	box     *secretbox.Box
	mu      sync.Mutex
	pool    map[uint64]*gorm.DB
	timeout time.Duration
}

// NewDatasourceService 创建数据源服务，默认超时 8 秒。
func NewDatasourceService(repo *repository.DatasourceRepository, box *secretbox.Box, timeout time.Duration) *DatasourceService {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &DatasourceService{repo: repo, box: box, pool: make(map[uint64]*gorm.DB), timeout: timeout}
}

// List 查询数据源列表（密码脱敏）。
func (s *DatasourceService) List() ([]DatasourceView, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make([]DatasourceView, 0, len(items))
	for _, item := range items {
		result = append(result, s.toView(item))
	}
	return result, nil
}

// Create 新增数据源，加密保存 DSN。
func (s *DatasourceService) Create(input DatasourceInput, userID uint64) (*DatasourceView, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Code = strings.TrimSpace(input.Code)
	input.Driver = strings.ToLower(strings.TrimSpace(input.Driver))
	if input.Name == "" || input.Code == "" {
		return nil, errors.New("数据源名称和编码不能为空")
	}
	if err := validateDriver(input.Driver); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.DSN) == "" {
		return nil, errors.New("请填写连接串 DSN")
	}
	if _, err := s.repo.FindByCode(input.Code); err == nil {
		return nil, errors.New("数据源编码已存在")
	}
	encrypted, err := s.box.Encrypt(input.DSN)
	if err != nil {
		return nil, err
	}
	item := &model.Datasource{
		Name: input.Name, Code: input.Code, Driver: input.Driver, DSN: encrypted,
		Status: defaultString(input.Status, "active"), ReadOnly: input.ReadOnly,
		MaxOpenConns: defaultInt(input.MaxOpenConns, 10), MaxIdleConns: defaultInt(input.MaxIdleConns, 3),
		Remark: input.Remark, CreatedBy: userID,
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	view := s.toView(*item)
	return &view, nil
}

// Update 更新数据源；DSN 留空表示不修改原密码。
func (s *DatasourceService) Update(id uint64, input DatasourceInput) (*DatasourceView, error) {
	item, err := s.repo.FindByID(id)
	if err != nil {
		return nil, errors.New("数据源不存在")
	}
	if input.Name != "" {
		item.Name = strings.TrimSpace(input.Name)
	}
	if input.Code != "" && input.Code != item.Code {
		if existing, err := s.repo.FindByCode(input.Code); err == nil && existing.ID != id {
			return nil, errors.New("数据源编码已存在")
		}
		item.Code = strings.TrimSpace(input.Code)
	}
	if input.Driver != "" {
		driver := strings.ToLower(strings.TrimSpace(input.Driver))
		if err := validateDriver(driver); err != nil {
			return nil, err
		}
		item.Driver = driver
	}
	if input.DSN != "" && !strings.HasPrefix(input.DSN, "••") {
		encrypted, err := s.box.Encrypt(input.DSN)
		if err != nil {
			return nil, err
		}
		item.DSN = encrypted
		s.Invalidate(id)
	}
	if input.Status != "" {
		item.Status = input.Status
	}
	item.ReadOnly = input.ReadOnly
	if input.MaxOpenConns > 0 {
		item.MaxOpenConns = input.MaxOpenConns
	}
	if input.MaxIdleConns > 0 {
		item.MaxIdleConns = input.MaxIdleConns
	}
	item.Remark = input.Remark
	if err := s.repo.Update(item); err != nil {
		return nil, err
	}
	view := s.toView(*item)
	return &view, nil
}

// Delete 删除数据源并关闭缓存的连接池。
func (s *DatasourceService) Delete(id uint64) error {
	s.Invalidate(id)
	return s.repo.Delete(id)
}

// Test 使用给定连接参数做连通性检查；id 用于编辑时复用旧 DSN。
func (s *DatasourceService) Test(id uint64, input DatasourceInput) (*DatasourceTestResult, error) {
	driver := strings.ToLower(strings.TrimSpace(input.Driver))
	dsn := strings.TrimSpace(input.DSN)
	if id > 0 {
		item, err := s.repo.FindByID(id)
		if err != nil {
			return nil, errors.New("数据源不存在")
		}
		if driver == "" {
			driver = item.Driver
		}
		if dsn == "" || strings.HasPrefix(dsn, "••") {
			dsn, err = s.box.Decrypt(item.DSN)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := validateDriver(driver); err != nil {
		return nil, err
	}
	if dsn == "" {
		return nil, errors.New("请填写连接串 DSN")
	}
	start := time.Now()
	db, err := openDatasource(driver, dsn, 3, 1)
	if err != nil {
		return &DatasourceTestResult{OK: false, Message: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return &DatasourceTestResult{OK: false, Message: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return &DatasourceTestResult{OK: false, Message: "连接失败: " + err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	latency := time.Since(start).Milliseconds()
	if id > 0 {
		now := time.Now()
		_ = s.repo.DB().Model(&model.Datasource{}).Where("id = ?", id).Updates(map[string]any{
			"last_test_at": &now, "last_test_ok": true, "last_test_msg": "连接成功",
		}).Error
	}
	return &DatasourceTestResult{OK: true, Message: "连接成功", LatencyMS: latency}, nil
}

// DB 返回带连接池的数据源句柄，句柄按数据源 ID 缓存复用。
func (s *DatasourceService) DB(id uint64) (*gorm.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.pool[id]; ok {
		return db, nil
	}
	item, err := s.repo.FindByID(id)
	if err != nil {
		return nil, errors.New("数据源不存在")
	}
	if item.Status != "active" {
		return nil, errors.New("数据源已停用")
	}
	dsn, err := s.box.Decrypt(item.DSN)
	if err != nil {
		return nil, err
	}
	db, err := openDatasource(item.Driver, dsn, item.MaxOpenConns, item.MaxIdleConns)
	if err != nil {
		return nil, fmt.Errorf("连接数据源失败: %w", err)
	}
	s.pool[id] = db
	return db, nil
}

// Invalidate 关闭并移除指定数据源的缓存连接。
func (s *DatasourceService) Invalidate(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.pool[id]; ok {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		delete(s.pool, id)
	}
}

// Driver 返回数据源使用的驱动名称，用于生成对应方言的分页 SQL。
func (s *DatasourceService) Driver(id uint64) (string, error) {
	item, err := s.repo.FindByID(id)
	if err != nil {
		return "", errors.New("数据源不存在")
	}
	return strings.ToLower(strings.TrimSpace(item.Driver)), nil
}

// CloseAll 关闭全部缓存连接（进程退出时调用）。
func (s *DatasourceService) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, db := range s.pool {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		delete(s.pool, id)
	}
}

func (s *DatasourceService) toView(item model.Datasource) DatasourceView {
	plain, err := s.box.Decrypt(item.DSN)
	if err != nil {
		plain = ""
	}
	return DatasourceView{
		ID: item.ID, Name: item.Name, Code: item.Code, Driver: item.Driver,
		DSNMasked: maskDSN(plain), Status: item.Status, ReadOnly: item.ReadOnly,
		MaxOpenConns: item.MaxOpenConns, MaxIdleConns: item.MaxIdleConns, Remark: item.Remark,
		LastTestAt: item.LastTestAt, LastTestOK: item.LastTestOK, LastTestMsg: item.LastTestMsg,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func openDatasource(driver, dsn string, maxOpen, maxIdle int) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch driver {
	case "mysql":
		dialector = mysql.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	case "sqlserver":
		dialector = sqlserver.Open(dsn)
	case "oracle":
		// go-ora 已注册 database/sql 驱动 "oracle"。
		// 复用 postgres 方言的 GORM 适配层（仅用 Raw SQL 查询，标识符使用双引号，与 Oracle 兼容）。
		dialector = postgres.New(postgres.Config{DriverName: "oracle", DSN: dsn})
	case "sqlite":
		if dir := filepath.Dir(dsn); dir != "." && dir != "" {
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("SQLite 数据文件目录不存在: %s", dir)
			}
		}
		dialector = sqlite.Open(dsn + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	default:
		return nil, fmt.Errorf("不支持的数据源驱动 %s", driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if maxOpen <= 0 {
		maxOpen = 10
	}
	if maxIdle <= 0 {
		maxIdle = 3
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func validateDriver(driver string) error {
	for _, item := range SupportedDrivers {
		if item == driver {
			return nil
		}
	}
	return fmt.Errorf("数据源驱动仅支持 %s", strings.Join(SupportedDrivers, " / "))
}

var passwordInURL = regexp.MustCompile(`(://[^:/@]+:)[^@]+@`)
var passwordInKV = regexp.MustCompile(`(?i)(password|pwd)(\s*=\s*)[^;\s]+`)

func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	masked := passwordInURL.ReplaceAllString(dsn, "$1••••@")
	masked = passwordInKV.ReplaceAllString(masked, "$1$2••••")
	runes := []rune(masked)
	if len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return masked
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
