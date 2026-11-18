package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Metadata   MetadataConfig   `mapstructure:"metadata"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Security   SecurityConfig   `mapstructure:"security"`
	DataSource DataSourceConfig `mapstructure:"datasource"`
	Cache      CacheConfig      `mapstructure:"cache"`
	SMTP       SMTPConfig       `mapstructure:"smtp"`
	Scheduler  SchedulerConfig  `mapstructure:"scheduler"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	WeCom      WeComConfig      `mapstructure:"wecom"`
	AI         AIConfig         `mapstructure:"ai"`
}

// AIConfig configures an OpenAI-compatible endpoint for SQL authoring help.
type AIConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	BaseURL     string  `mapstructure:"baseUrl"` // e.g. https://api.deepseek.com/v1
	APIKey      string  `mapstructure:"apiKey"`
	Model       string  `mapstructure:"model"`
	Temperature float64 `mapstructure:"temperature"`
	MaxTokens   int     `mapstructure:"maxTokens"`
	// MaxSchemaTables caps how many tables are sent as context.
	MaxSchemaTables int  `mapstructure:"maxSchemaTables"`
	IncludeSchema   bool `mapstructure:"includeSchema"`
}

// WeComConfig configures 企业微信 OAuth login and contacts synchronization.
type WeComConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CorpID       string `mapstructure:"corpId"`
	AgentID      int    `mapstructure:"agentId"`
	Secret       string `mapstructure:"secret"`
	ContactSecret string `mapstructure:"contactSecret"` // 通讯录同步 secret（可省略，默认用 Secret）
	RedirectURI  string `mapstructure:"redirectUri"`    // 例如 http://bi.example.com/wecom/callback
	APIBase      string `mapstructure:"apiBase"`        // 私有化部署可覆盖，默认 https://qyapi.weixin.qq.com
	AutoCreateUser bool `mapstructure:"autoCreateUser"`
	SuperAdminIDs  []string `mapstructure:"superAdminIds"` // 企业微信 userid 列表
	SyncIntervalHours int `mapstructure:"syncIntervalHours"`
}

type ServerConfig struct {
	Port         int      `mapstructure:"port"`
	CorsOrigins  []string `mapstructure:"corsOrigins"`
	PublicURL    string   `mapstructure:"publicUrl"` // 对外访问地址，用于分享/推送链接
	ReadTimeout  int      `mapstructure:"readTimeoutSec"`
	BodyLimitMB  int      `mapstructure:"bodyLimitMB"`
	RequestLogging bool   `mapstructure:"requestLogging"`
}

type MetadataConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type AuthConfig struct {
	JWTSecret     string `mapstructure:"jwtSecret"`
	TokenTTLHours int    `mapstructure:"tokenTTLHours"`
	AdminUsername string `mapstructure:"adminUsername"`
	AdminPassword string `mapstructure:"adminPassword"`
}

type SecurityConfig struct {
	SecretKey string `mapstructure:"secretKey"`
}

type DataSourceConfig struct {
	MaxOpen         int `mapstructure:"maxOpen"`
	MaxIdle         int `mapstructure:"maxIdle"`
	QueryTimeoutSec int `mapstructure:"queryTimeoutSec"`
	MaxResultRows   int `mapstructure:"maxResultRows"`
}

type CacheConfig struct {
	TTLSeconds int `mapstructure:"ttlSeconds"`
	MaxEntries int `mapstructure:"maxEntries"`
}

type SMTPConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	TLSMode  string `mapstructure:"tlsMode"`
	From     string `mapstructure:"from"`
}

type SchedulerConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	ReportDir     string `mapstructure:"reportDir"`
	RetentionDays int    `mapstructure:"retentionDays"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

// Load reads YAML config (if present) with BI_* environment overrides.
func Load(configFile string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}
	v.SetEnvPrefix("BI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	// Unmarshal does not consult AutomaticEnv for individual keys; bind known keys
	// explicitly so BI_* variables always override defaults and YAML.
	for _, key := range knownKeys {
		_ = v.BindEnv(key)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Explicitly configured file must exist; implicitly looked-up file may not.
			if configFile != "" {
				return nil, fmt.Errorf("read config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

var knownKeys = []string{
	"server.port", "server.corsOrigins", "server.publicUrl", "server.readTimeoutSec", "server.bodyLimitMB", "server.requestLogging",
	"metadata.driver", "metadata.dsn",
	"auth.jwtSecret", "auth.tokenTTLHours", "auth.adminUsername", "auth.adminPassword",
	"security.secretKey",
	"datasource.maxOpen", "datasource.maxIdle", "datasource.queryTimeoutSec", "datasource.maxResultRows",
	"cache.ttlSeconds", "cache.maxEntries",
	"smtp.enabled", "smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.tlsMode", "smtp.from",
	"scheduler.enabled", "scheduler.reportDir", "scheduler.retentionDays",
	"logging.level",
	"wecom.enabled", "wecom.corpId", "wecom.agentId", "wecom.secret", "wecom.contactSecret",
	"wecom.redirectUri", "wecom.apiBase", "wecom.autoCreateUser", "wecom.superAdminIds",
	"wecom.syncIntervalHours",
	"ai.enabled", "ai.baseUrl", "ai.apiKey", "ai.model", "ai.temperature",
	"ai.maxTokens", "ai.maxSchemaTables", "ai.includeSchema",
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 2345)
	v.SetDefault("server.corsOrigins", []string{"http://localhost:5173", "http://127.0.0.1:5173"})
	v.SetDefault("server.readTimeoutSec", 60)
	v.SetDefault("server.bodyLimitMB", 5)
	v.SetDefault("server.publicUrl", "")
	v.SetDefault("server.requestLogging", true)

	v.SetDefault("metadata.driver", "sqlite")
	v.SetDefault("metadata.dsn", "data/bi.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")

	v.SetDefault("auth.jwtSecret", "")
	v.SetDefault("auth.tokenTTLHours", 12)
	v.SetDefault("auth.adminUsername", "admin")

	v.SetDefault("security.secretKey", "")

	v.SetDefault("datasource.maxOpen", 5)
	v.SetDefault("datasource.maxIdle", 2)
	v.SetDefault("datasource.queryTimeoutSec", 15)
	v.SetDefault("datasource.maxResultRows", 5000)

	v.SetDefault("cache.ttlSeconds", 60)
	v.SetDefault("cache.maxEntries", 500)

	v.SetDefault("smtp.enabled", false)
	v.SetDefault("smtp.port", 587)
	v.SetDefault("smtp.tlsMode", "starttls")
	v.SetDefault("smtp.from", "bi@example.com")

	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.reportDir", "data/reports")
	v.SetDefault("scheduler.retentionDays", 30)

	v.SetDefault("logging.level", "info")

	v.SetDefault("wecom.enabled", false)
	v.SetDefault("wecom.apiBase", "https://qyapi.weixin.qq.com")
	v.SetDefault("wecom.autoCreateUser", true)
	v.SetDefault("wecom.syncIntervalHours", 12)

	v.SetDefault("ai.enabled", false)
	v.SetDefault("ai.baseUrl", "")
	v.SetDefault("ai.apiKey", "")
	v.SetDefault("ai.model", "")
	v.SetDefault("ai.temperature", 0.2)
	v.SetDefault("ai.maxTokens", 1500)
	v.SetDefault("ai.maxSchemaTables", 40)
	v.SetDefault("ai.includeSchema", true)
}

func applyDefaults(cfg *Config) {
	// Generate an ephemeral dev secret when none is configured, so the app still
	// boots cleanly. Sessions/credentials encrypted with it do not survive restarts.
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = randomSecret()
	}
	if cfg.Security.SecretKey == "" {
		// Derive a stable key from the JWT secret so encrypted datasource passwords
		// remain readable across restarts when only auth.jwtSecret is configured.
		cfg.Security.SecretKey = deriveKey(cfg.Auth.JWTSecret)
	}
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func deriveKey(secret string) string {
	// SHA-256 is applied in the crypto package; here we just provide stable material.
	return "derived:" + secret
}

// EnsureDir is a small helper for SQLite metadata paths.
func EnsureDir(path string) error {
	if path == "" || strings.HasPrefix(path, "file:") || strings.Contains(path, "://") || strings.Contains(path, "@tcp(") {
		return nil
	}
	// Strip query part of sqlite DSNs.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return nil
	}
	return os.MkdirAll(path[:idx], 0o755)
}

// TokenTTL returns the configured JWT lifetime.
func (c *Config) TokenTTL() time.Duration {
	h := c.Auth.TokenTTLHours
	if h <= 0 {
		h = 12
	}
	return time.Duration(h) * time.Hour
}

// QueryTimeout returns the configured external query timeout with sane bounds.
func (c *Config) QueryTimeout() time.Duration {
	s := c.DataSource.QueryTimeoutSec
	if s <= 0 {
		s = 15
	}
	if s > 60 {
		s = 60
	}
	return time.Duration(s) * time.Second
}
