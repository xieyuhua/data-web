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
		if err := os.MkdirAll(filepath.Dir(cfg.DSN), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
		dialector = sqlite.Open(cfg.DSN + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
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
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if cfg.AutoMigrate {
		if err := db.AutoMigrate(model.AllModels()...); err != nil {
			return nil, fmt.Errorf("数据库迁移失败: %w", err)
		}
		log.Println("database migration completed")
	}
	return db, nil
}
