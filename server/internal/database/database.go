package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 按配置打开数据库连接。
// 支持 sqlite / mysql / postgres / sqlserver，并统一设置连接池参数。
func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.DBDriver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "sqlserver":
		dialector = sqlserver.Open(cfg.DSN)
	default:
		// 默认使用 SQLite，并确保数据目录存在
		if err := os.MkdirAll(filepath.Dir(cfg.DSN), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
		dialector = sqlite.Open(cfg.DSN + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		// 只打印慢查询与错误，避免日志膨胀
		Logger: logger.Default.LogMode(logger.Warn),
		// 统一使用配置时区，保证定时推送与统计口径一致
		NowFunc: func() time.Time {
			loc, _ := time.LoadLocation(cfg.Timezone)
			return time.Now().In(loc)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 连接池参数按通用 OLTP 场景设置
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if cfg.AutoMigrate {
		// 启动时自动迁移全部模型表结构
		if err := db.AutoMigrate(model.AllModels()...); err != nil {
			return nil, fmt.Errorf("数据库迁移失败: %w", err)
		}
		log.Println("database migration completed")
	}
	return db, nil
}
