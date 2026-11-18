package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"

	"gorm.io/gorm"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

// SettingService persists runtime configuration overrides and applies them
// without restarting the server.
type SettingService struct {
	repo *repository.Repo
	rt   *config.Runtime
	// hooks invoked after a successful update, keyed by feature.
	hooks map[string][]func(*config.Config)
}

func NewSettingService(repo *repository.Repo, rt *config.Runtime) *SettingService {
	return &SettingService{repo: repo, rt: rt, hooks: map[string][]func(*config.Config){}}
}

// OnApply registers a callback for a configuration section (see apply()).
func (s *SettingService) OnApply(section string, fn func(*config.Config)) {
	s.hooks[section] = append(s.hooks[section], fn)
}

// settingSpec describes one editable setting: validation + whether it is secret.
type settingSpec struct {
	Key      string
	Label    string
	Section  string
	Kind     string // string|int|bool|float
	Secret   bool
	Min, Max int
	Hint     string
	Options  []string
}

// editableSettings is the allowlist. Anything not listed cannot be changed at
// runtime, which keeps file-only concerns (ports, DB DSNs, JWT secret) immutable.
var editableSettings = []settingSpec{
	{Key: "logging.level", Label: "日志级别", Section: "logging", Kind: "string",
		Options: []string{"debug", "info", "warn", "error"}, Hint: "立即生效"},

	{Key: "datasource.queryTimeoutSec", Label: "查询超时（秒）", Section: "datasource", Kind: "int", Min: 1, Max: 60,
		Hint: "1-60 秒，影响新发起的查询"},
	{Key: "datasource.maxResultRows", Label: "结果行上限", Section: "datasource", Kind: "int", Min: 100, Max: 100000,
		Hint: "图表单次取数的最大行数"},
	{Key: "datasource.maxOpen", Label: "连接池上限", Section: "datasource", Kind: "int", Min: 1, Max: 200,
		Hint: "新建连接池时使用"},
	{Key: "datasource.maxIdle", Label: "空闲连接数", Section: "datasource", Kind: "int", Min: 0, Max: 100},

	{Key: "cache.ttlSeconds", Label: "结果缓存 TTL（秒）", Section: "cache", Kind: "int", Min: 0, Max: 3600,
		Hint: "0 表示禁用缓存"},
	{Key: "cache.maxEntries", Label: "缓存条目上限", Section: "cache", Kind: "int", Min: 10, Max: 10000},

	{Key: "smtp.enabled", Label: "启用邮件", Section: "smtp", Kind: "bool"},
	{Key: "smtp.host", Label: "SMTP 主机", Section: "smtp", Kind: "string"},
	{Key: "smtp.port", Label: "SMTP 端口", Section: "smtp", Kind: "int", Min: 1, Max: 65535},
	{Key: "smtp.username", Label: "SMTP 用户名", Section: "smtp", Kind: "string"},
	{Key: "smtp.password", Label: "SMTP 密码", Section: "smtp", Kind: "string", Secret: true},
	{Key: "smtp.from", Label: "发件人", Section: "smtp", Kind: "string"},
	{Key: "smtp.tlsMode", Label: "TLS 模式", Section: "smtp", Kind: "string",
		Options: []string{"none", "starttls", "ssl"}},

	{Key: "scheduler.enabled", Label: "启用定时报表", Section: "scheduler", Kind: "bool"},
	{Key: "scheduler.retentionDays", Label: "报表保留天数", Section: "scheduler", Kind: "int", Min: 1, Max: 365},

	{Key: "wecom.enabled", Label: "启用企业微信", Section: "wecom", Kind: "bool"},
	{Key: "wecom.corpId", Label: "企业 ID", Section: "wecom", Kind: "string"},
	{Key: "wecom.agentId", Label: "AgentId", Section: "wecom", Kind: "int", Min: 0, Max: 1 << 30},
	{Key: "wecom.secret", Label: "应用 Secret", Section: "wecom", Kind: "string", Secret: true},
	{Key: "wecom.contactSecret", Label: "通讯录 Secret", Section: "wecom", Kind: "string", Secret: true},
	{Key: "wecom.redirectUri", Label: "回调地址", Section: "wecom", Kind: "string"},
	{Key: "wecom.apiBase", Label: "API 地址", Section: "wecom", Kind: "string"},
	{Key: "wecom.autoCreateUser", Label: "自动开通账号", Section: "wecom", Kind: "bool"},
	{Key: "wecom.syncIntervalHours", Label: "通讯录同步间隔（小时）", Section: "wecom", Kind: "int", Min: 0, Max: 168},

	{Key: "ai.enabled", Label: "启用 AI 辅助", Section: "ai", Kind: "bool",
		Hint: "启用后数据集编辑器可使用 AI 生成 SQL / 补全参数"},
	{Key: "ai.baseUrl", Label: "接口地址", Section: "ai", Kind: "string",
		Hint: "OpenAI 兼容地址，如 https://api.deepseek.com/v1 或 http://127.0.0.1:11434/v1"},
	{Key: "ai.apiKey", Label: "API Key", Section: "ai", Kind: "string", Secret: true},
	{Key: "ai.model", Label: "模型", Section: "ai", Kind: "string",
		Hint: "如 deepseek-chat、qwen2.5-coder、llama3.1"},
	{Key: "ai.includeSchema", Label: "附带表结构", Section: "ai", Kind: "bool",
		Hint: "生成 SQL 时把数据源表结构一并发送（推荐开启）"},
	{Key: "ai.maxSchemaTables", Label: "表结构最大表数", Section: "ai", Kind: "int", Min: 5, Max: 200},
	{Key: "ai.temperature", Label: "温度", Section: "ai", Kind: "string",
		Hint: "0-2，越低越稳定；SQL 生成建议 0.1-0.3"},

	{Key: "server.publicUrl", Label: "对外访问地址", Section: "server", Kind: "string",
		Hint: "用于分享链接与企微推送，如 https://bi.example.com"},
}

func specByKey(key string) (settingSpec, bool) {
	for _, s := range editableSettings {
		if s.Key == key {
			return s, true
		}
	}
	return settingSpec{}, false
}

// Field describes one setting for the admin UI.
type Field struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Section string   `json:"section"`
	Kind    string   `json:"kind"`
	Value   string   `json:"value"`
	Secret  bool     `json:"secret"`
	Set     bool     `json:"set"` // true when an override is stored
	Hint    string   `json:"hint,omitempty"`
	Options []string `json:"options,omitempty"`
}

// Sections groups fields for rendering.
type Section struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

var sectionLabels = map[string]string{
	"logging": "日志", "datasource": "查询与数据源", "cache": "缓存",
	"smtp": "邮件", "scheduler": "定时报表", "wecom": "企业微信", "ai": "AI 辅助", "server": "服务",
}

// List returns every editable field with its current effective value.
// Secrets are masked unless they were never set.
func (s *SettingService) List() ([]Section, error) {
	var rows []model.Setting
	if err := s.repo.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	overrides := make(map[string]string, len(rows))
	for _, r := range rows {
		overrides[r.Key] = r.Value
	}
	cfg := s.rt.Get()

	order := []string{"server", "datasource", "cache", "logging", "smtp", "scheduler", "wecom", "ai"}
	grouped := map[string][]Field{}
	for _, spec := range editableSettings {
		val := readConfigValue(cfg, spec.Key)
		if spec.Secret && val != "" {
			val = "******"
		}
		_, hasOverride := overrides[spec.Key]
		grouped[spec.Section] = append(grouped[spec.Section], Field{
			Key: spec.Key, Label: spec.Label, Section: spec.Section, Kind: spec.Kind,
			Value: val, Secret: spec.Secret, Set: hasOverride, Hint: spec.Hint, Options: spec.Options,
		})
	}
	out := make([]Section, 0, len(order))
	for _, name := range order {
		fields, ok := grouped[name]
		if !ok {
			continue
		}
		out = append(out, Section{Name: name, Label: sectionLabels[name], Fields: fields})
	}
	return out, nil
}

// Update validates and persists the given key/value pairs, then applies them.
func (s *SettingService) Update(values map[string]string) error {
	if len(values) == 0 {
		return errors.New("没有需要保存的配置")
	}
	// Validate everything first so a partial failure never half-applies.
	normalized := make(map[string]string, len(values))
	for key, raw := range values {
		spec, ok := specByKey(key)
		if !ok {
			return fmt.Errorf("配置项 %q 不允许在运行时修改", key)
		}
		val, err := validateSetting(spec, raw)
		if err != nil {
			return err
		}
		normalized[key] = val
	}

	err := s.repo.DB.Transaction(func(tx *gorm.DB) error {
		for key, val := range normalized {
			var row model.Setting
			err := tx.Where("key = ?", key).First(&row).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(&model.Setting{Key: key, Value: val}).Error; err != nil {
					return err
				}
			case err != nil:
				return err
			default:
				if err := tx.Model(&row).Update("value", val).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.apply(normalized)
	return nil
}

// Reset removes an override so the file/default value applies again.
func (s *SettingService) Reset(keys []string) error {
	if len(keys) == 0 {
		return errors.New("请选择要恢复的配置项")
	}
	for _, k := range keys {
		if _, ok := specByKey(k); !ok {
			return fmt.Errorf("配置项 %q 不可重置", k)
		}
	}
	if err := s.repo.DB.Where("key IN ?", keys).Delete(&model.Setting{}).Error; err != nil {
		return err
	}
	// Re-apply the whole runtime config to drop the overrides cleanly.
	cfg, err := s.LoadWithOverrides()
	if err != nil {
		return err
	}
	s.rt.Update(func(c *config.Config) { *c = *cfg })
	s.notifyAll()
	return nil
}

// LoadWithOverrides merges stored overrides onto a fresh config load. Called at
// startup so admin changes survive restarts.
func (s *SettingService) LoadWithOverrides() (*config.Config, error) {
	cfg, err := config.Load(configPath())
	if err != nil {
		return nil, err
	}
	var rows []model.Setting
	if err := s.repo.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if _, ok := specByKey(r.Key); !ok {
			continue
		}
		if err := writeConfigValue(cfg, r.Key, r.Value); err != nil {
			slog.Warn("忽略无效的配置覆盖", "key", r.Key, "error", err)
		}
	}
	return cfg, nil
}

// apply copies normalized values into the live config, section by section.
func (s *SettingService) apply(values map[string]string) {
	sections := map[string]bool{}
	s.rt.Update(func(c *config.Config) {
		for key, val := range values {
			spec, _ := specByKey(key)
			sections[spec.Section] = true
			_ = writeConfigValue(c, key, val)
		}
	})
	for section := range sections {
		for _, fn := range s.hooks[section] {
			fn(s.rt.Get())
		}
	}
	// Cache and logging are cheap to rebuild; make their hooks unconditional so
	// deleting a TTL override also takes effect.
	for _, name := range []string{"cache", "logging"} {
		if !sections[name] {
			for _, fn := range s.hooks[name] {
				fn(s.rt.Get())
			}
		}
	}
}

func (s *SettingService) notifyAll() {
	for _, fns := range s.hooks {
		for _, fn := range fns {
			fn(s.rt.Get())
		}
	}
}

// configPath mirrors the -config flag so reloads read the same file.
var configuredPath atomic.Value

func init() { configuredPath.Store("") }

func configPath() string {
	if v, ok := configuredPath.Load().(string); ok {
		return v
	}
	return ""
}

// SetConfigPath records the config file used at startup.
func SetConfigPath(p string) { configuredPath.Store(p) }

func validateSetting(spec settingSpec, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch spec.Kind {
	case "bool":
		if raw == "" {
			return "false", nil
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return "", fmt.Errorf("%s 必须是 true/false", spec.Label)
		}
		return strconv.FormatBool(b), nil
	case "int":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return "", fmt.Errorf("%s 必须是整数", spec.Label)
		}
		if spec.Min != 0 || spec.Max != 0 {
			if n < spec.Min || n > spec.Max {
				return "", fmt.Errorf("%s 需在 %d-%d 之间", spec.Label, spec.Min, spec.Max)
			}
		}
		return strconv.Itoa(n), nil
	default:
		if len(spec.Options) > 0 {
			for _, o := range spec.Options {
				if strings.EqualFold(o, raw) {
					return strings.ToLower(o), nil
				}
			}
			return "", fmt.Errorf("%s 只能是 %s", spec.Label, strings.Join(spec.Options, "/"))
		}
		if spec.Key == "server.publicUrl" && raw != "" &&
			!strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			return "", errors.New("对外访问地址需以 http:// 或 https:// 开头")
		}
		return raw, nil
	}
}

func readConfigValue(cfg *config.Config, key string) string {
	switch key {
	case "logging.level":
		return cfg.Logging.Level
	case "datasource.queryTimeoutSec":
		return strconv.Itoa(cfg.DataSource.QueryTimeoutSec)
	case "datasource.maxResultRows":
		return strconv.Itoa(cfg.DataSource.MaxResultRows)
	case "datasource.maxOpen":
		return strconv.Itoa(cfg.DataSource.MaxOpen)
	case "datasource.maxIdle":
		return strconv.Itoa(cfg.DataSource.MaxIdle)
	case "cache.ttlSeconds":
		return strconv.Itoa(cfg.Cache.TTLSeconds)
	case "cache.maxEntries":
		return strconv.Itoa(cfg.Cache.MaxEntries)
	case "smtp.enabled":
		return strconv.FormatBool(cfg.SMTP.Enabled)
	case "smtp.host":
		return cfg.SMTP.Host
	case "smtp.port":
		return strconv.Itoa(cfg.SMTP.Port)
	case "smtp.username":
		return cfg.SMTP.Username
	case "smtp.password":
		return cfg.SMTP.Password
	case "smtp.from":
		return cfg.SMTP.From
	case "smtp.tlsMode":
		return cfg.SMTP.TLSMode
	case "scheduler.enabled":
		return strconv.FormatBool(cfg.Scheduler.Enabled)
	case "scheduler.retentionDays":
		return strconv.Itoa(cfg.Scheduler.RetentionDays)
	case "wecom.enabled":
		return strconv.FormatBool(cfg.WeCom.Enabled)
	case "wecom.corpId":
		return cfg.WeCom.CorpID
	case "wecom.agentId":
		return strconv.Itoa(cfg.WeCom.AgentID)
	case "wecom.secret":
		return cfg.WeCom.Secret
	case "wecom.contactSecret":
		return cfg.WeCom.ContactSecret
	case "wecom.redirectUri":
		return cfg.WeCom.RedirectURI
	case "wecom.apiBase":
		return cfg.WeCom.APIBase
	case "wecom.autoCreateUser":
		return strconv.FormatBool(cfg.WeCom.AutoCreateUser)
	case "wecom.syncIntervalHours":
		return strconv.Itoa(cfg.WeCom.SyncIntervalHours)
	case "ai.enabled":
		return strconv.FormatBool(cfg.AI.Enabled)
	case "ai.baseUrl":
		return cfg.AI.BaseURL
	case "ai.apiKey":
		return cfg.AI.APIKey
	case "ai.model":
		return cfg.AI.Model
	case "ai.includeSchema":
		return strconv.FormatBool(cfg.AI.IncludeSchema)
	case "ai.maxSchemaTables":
		return strconv.Itoa(cfg.AI.MaxSchemaTables)
	case "ai.temperature":
		return strconv.FormatFloat(cfg.AI.Temperature, 'f', -1, 64)
	case "server.publicUrl":
		return cfg.Server.PublicURL
	}
	return ""
}

func writeConfigValue(cfg *config.Config, key, val string) error {
	switch key {
	case "logging.level":
		cfg.Logging.Level = val
	case "datasource.queryTimeoutSec":
		v, _ := strconv.Atoi(val)
		cfg.DataSource.QueryTimeoutSec = v
	case "datasource.maxResultRows":
		v, _ := strconv.Atoi(val)
		cfg.DataSource.MaxResultRows = v
	case "datasource.maxOpen":
		v, _ := strconv.Atoi(val)
		cfg.DataSource.MaxOpen = v
	case "datasource.maxIdle":
		v, _ := strconv.Atoi(val)
		cfg.DataSource.MaxIdle = v
	case "cache.ttlSeconds":
		v, _ := strconv.Atoi(val)
		cfg.Cache.TTLSeconds = v
	case "cache.maxEntries":
		v, _ := strconv.Atoi(val)
		cfg.Cache.MaxEntries = v
	case "smtp.enabled":
		v, _ := strconv.ParseBool(val)
		cfg.SMTP.Enabled = v
	case "smtp.host":
		cfg.SMTP.Host = val
	case "smtp.port":
		v, _ := strconv.Atoi(val)
		cfg.SMTP.Port = v
	case "smtp.username":
		cfg.SMTP.Username = val
	case "smtp.password":
		cfg.SMTP.Password = val
	case "smtp.from":
		cfg.SMTP.From = val
	case "smtp.tlsMode":
		cfg.SMTP.TLSMode = val
	case "scheduler.enabled":
		v, _ := strconv.ParseBool(val)
		cfg.Scheduler.Enabled = v
	case "scheduler.retentionDays":
		v, _ := strconv.Atoi(val)
		cfg.Scheduler.RetentionDays = v
	case "wecom.enabled":
		v, _ := strconv.ParseBool(val)
		cfg.WeCom.Enabled = v
	case "wecom.corpId":
		cfg.WeCom.CorpID = val
	case "wecom.agentId":
		v, _ := strconv.Atoi(val)
		cfg.WeCom.AgentID = v
	case "wecom.secret":
		cfg.WeCom.Secret = val
	case "wecom.contactSecret":
		cfg.WeCom.ContactSecret = val
	case "wecom.redirectUri":
		cfg.WeCom.RedirectURI = val
	case "wecom.apiBase":
		cfg.WeCom.APIBase = val
	case "wecom.autoCreateUser":
		v, _ := strconv.ParseBool(val)
		cfg.WeCom.AutoCreateUser = v
	case "wecom.syncIntervalHours":
		v, _ := strconv.Atoi(val)
		cfg.WeCom.SyncIntervalHours = v
	case "ai.enabled":
		v, _ := strconv.ParseBool(val)
		cfg.AI.Enabled = v
	case "ai.baseUrl":
		cfg.AI.BaseURL = val
	case "ai.apiKey":
		cfg.AI.APIKey = val
	case "ai.model":
		cfg.AI.Model = val
	case "ai.includeSchema":
		v, _ := strconv.ParseBool(val)
		cfg.AI.IncludeSchema = v
	case "ai.maxSchemaTables":
		v, _ := strconv.Atoi(val)
		cfg.AI.MaxSchemaTables = v
	case "ai.temperature":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("温度必须是数字")
		}
		cfg.AI.Temperature = v
	case "server.publicUrl":
		cfg.Server.PublicURL = val
	default:
		return fmt.Errorf("未知配置项 %q", key)
	}
	return nil
}

