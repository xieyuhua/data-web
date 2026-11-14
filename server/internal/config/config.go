package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	DBDriver        string
	DSN             string
	JWTSecret       string
	JWTExpire       time.Duration
	APIBaseURL      string
	WebBaseURL      string
	CORSOrigins     string
	Timezone        string
	AutoMigrate     bool
	SeedDemoData    bool
	AllowMockPush   bool
	SQLQueryTimeout time.Duration
}

func Load() Config {
	loadDotEnv()
	port := env("PORT", "8486")
	return Config{
		Port:      port,
		DBDriver:  env("DB_DRIVER", "sqlite"),
		DSN:       env("DB_DSN", "data/bi.db"),
		JWTSecret: env("JWT_SECRET", "change-me-in-production-bi-platform-2026"),
		JWTExpire: time.Duration(envInt("JWT_EXPIRE_HOURS", 24)) * time.Hour,
		// WebBaseURL 留空表示按访问请求自动识别（后端同时托管前端静态资源），
		// 如需固定外网域名可通过 WEB_BASE_URL 显式指定。
		APIBaseURL:      env("API_BASE_URL", "http://localhost:"+port),
		WebBaseURL:      env("WEB_BASE_URL", ""),
		CORSOrigins:     env("CORS_ORIGINS", "http://localhost:"+port+",http://127.0.0.1:"+port+",http://localhost:5173,http://127.0.0.1:5173"),
		Timezone:        env("APP_TIMEZONE", "Asia/Shanghai"),
		AutoMigrate:     envBool("AUTO_MIGRATE", true),
		SeedDemoData:    envBool("SEED_DEMO_DATA", true),
		AllowMockPush:   envBool("ALLOW_MOCK_PUSH", true),
		SQLQueryTimeout: time.Duration(envInt("SQL_QUERY_TIMEOUT_SECONDS", 10)) * time.Second,
	}
}

// loadDotEnv 读取当前目录下的 .env 文件，已存在的环境变量优先。
func loadDotEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func (c Config) Address() string { return ":" + c.Port }

func (c Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("PORT 不能为空")
	}
	if len(c.JWTSecret) < 16 {
		return fmt.Errorf("JWT_SECRET 至少需要 16 个字符")
	}
	switch c.DBDriver {
	case "sqlite", "mysql", "postgres", "sqlserver":
	default:
		return fmt.Errorf("暂不支持数据库驱动 %s", c.DBDriver)
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return result
}
