package datasource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/glebarez/sqlite" // sqlite driver (pure Go)
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/sijms/go-ora/v2"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/crypto"
	"biaoge-bi/internal/model"
	"gorm.io/gorm"
)

type pooled struct {
	db       *sql.DB
	dialect  string
	lastUsed time.Time
}

// Manager lazily opens and caches connection pools per datasource.
type Manager struct {
	meta   *gorm.DB
	cipher *crypto.Cipher
	cfg    *config.Config

	mu    sync.RWMutex
	pools map[uint]*pooled

	stop chan struct{}
}

func NewManager(meta *gorm.DB, cipher *crypto.Cipher, cfg *config.Config) *Manager {
	m := &Manager{
		meta:   meta,
		cipher: cipher,
		cfg:    cfg,
		pools:  make(map[uint]*pooled),
		stop:   make(chan struct{}),
	}
	go m.janitor()
	return m
}

func (m *Manager) janitor() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			cutoff := time.Now().Add(-15 * time.Minute)
			m.mu.Lock()
			for id, p := range m.pools {
				if p.lastUsed.Before(cutoff) {
					_ = p.db.Close()
					delete(m.pools, id)
				}
			}
			m.mu.Unlock()
		case <-m.stop:
			return
		}
	}
}

func (m *Manager) Close() {
	close(m.stop)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.pools {
		_ = p.db.Close()
		delete(m.pools, id)
	}
}

func (m *Manager) LoadDataSource(id uint) (*model.DataSource, error) {
	var ds model.DataSource
	if err := m.meta.First(&ds, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("数据源不存在")
		}
		return nil, err
	}
	return &ds, nil
}

// Get returns a live pool for the datasource, opening it on first use.
func (m *Manager) Get(ctx context.Context, id uint) (*sql.DB, string, error) {
	m.mu.RLock()
	p, ok := m.pools[id]
	if ok {
		p.lastUsed = time.Now()
		db, dialect := p.db, p.dialect
		m.mu.RUnlock()
		return db, dialect, nil
	}
	m.mu.RUnlock()

	ds, err := m.LoadDataSource(id)
	if err != nil {
		return nil, "", err
	}
	db, err := m.open(ds)
	if err != nil {
		return nil, "", err
	}
	m.mu.Lock()
	if existing, exists := m.pools[id]; exists {
		// lost the race; use existing and close ours
		m.mu.Unlock()
		_ = db.Close()
		return existing.db, existing.dialect, nil
	}
	m.pools[id] = &pooled{db: db, dialect: ds.Type, lastUsed: time.Now()}
	m.mu.Unlock()
	return db, ds.Type, nil
}

// Invalidate drops a cached pool (after datasource update/delete).
func (m *Manager) Invalidate(id uint) {
	m.mu.Lock()
	if p, ok := m.pools[id]; ok {
		_ = p.db.Close()
		delete(m.pools, id)
	}
	m.mu.Unlock()
}

func (m *Manager) passwordFor(ds *model.DataSource) (string, error) {
	if ds.PasswordEnc == "" {
		return "", nil
	}
	return m.cipher.Decrypt(ds.PasswordEnc)
}

func (m *Manager) open(ds *model.DataSource) (*sql.DB, error) {
	password, err := m.passwordFor(ds)
	if err != nil {
		return nil, fmt.Errorf("解密数据源密码失败: %w", err)
	}
	dsn, err := BuildDSN(ds, password)
	if err != nil {
		return nil, err
	}
	driver := driverName(ds.Type)
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	applyPool(db, ds, m.cfg)

	pingCtx, cancel := context.WithTimeout(context.Background(), PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接测试失败: %w", err)
	}
	return db, nil
}

func applyPool(db *sql.DB, ds *model.DataSource, cfg *config.Config) {
	maxOpen := cfg.DataSource.MaxOpen
	maxIdle := cfg.DataSource.MaxIdle
	if maxOpen <= 0 {
		maxOpen = 5
	}
	if maxIdle <= 0 {
		maxIdle = 2
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)
}

func driverName(t string) string {
	switch t {
	case model.DSMySQL:
		return "mysql"
	case model.DSPostgres:
		return "pgx"
	case model.DSOracle:
		return "oracle"
	default:
		return "sqlite"
	}
}

// Test opens a short-lived connection, runs SELECT 1 and closes it.
func (m *Manager) Test(ds *model.DataSource, password string) (string, error) {
	dsn, err := BuildDSN(ds, password)
	if err != nil {
		return "", err
	}
	db, err := sql.Open(driverName(ds.Type), dsn)
	if err != nil {
		return "", err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), PingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return "", err
	}
	var one int
	if err := db.QueryRowContext(ctx, TestQuery(ds.Type)).Scan(&one); err != nil {
		return "", err
	}
	return "连接成功", nil
}

// TestByID loads a stored datasource and tests it, updating status fields.
func (m *Manager) TestByID(ctx context.Context, id uint) error {
	ds, err := m.LoadDataSource(id)
	if err != nil {
		return err
	}
	password, err := m.passwordFor(ds)
	if err != nil {
		return err
	}
	msg, testErr := m.Test(ds, password)
	now := time.Now()
	status := "success"
	if testErr != nil {
		status = "failed"
		msg = truncate(testErr.Error(), 500)
	}
	m.meta.Model(&model.DataSource{}).Where("id = ?", id).Updates(map[string]any{
		"last_tested_at":    &now,
		"last_test_status":  status,
		"last_test_message": msg,
	})
	if testErr != nil {
		return fmt.Errorf("连接测试失败: %s", msg)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
