package model

import (
	"time"
)

type Organization struct {
	ID          uint64         `json:"id" gorm:"primaryKey"`
	ParentID    *uint64        `json:"parent_id" gorm:"index"`
	Name        string         `json:"name" gorm:"size:100;not null"`
	Code        string         `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Type        string         `json:"type" gorm:"size:32;index;not null"`
	Level       int            `json:"level" gorm:"not null;default:1"`
	ManagerName string         `json:"manager_name" gorm:"size:64"`
	Phone       string         `json:"phone" gorm:"size:32"`
	Address     string         `json:"address" gorm:"size:255"`
	Status      string         `json:"status" gorm:"size:16;default:active"`
	Sort        int            `json:"sort" gorm:"default:0"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Children    []Organization `json:"children,omitempty" gorm:"-"`
}

type Role struct {
	ID          uint64    `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"size:48;uniqueIndex;not null"`
	Name        string    `json:"name" gorm:"size:64;not null"`
	DataScope   string    `json:"data_scope" gorm:"size:32;not null"`
	Permissions string    `json:"-" gorm:"type:text"`
	Description string    `json:"description" gorm:"size:255"`
	Level       int       `json:"level" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	ID             uint64       `json:"id" gorm:"primaryKey"`
	Name           string       `json:"name" gorm:"size:64;not null"`
	Username       string       `json:"username" gorm:"size:64;uniqueIndex;not null"`
	PasswordHash   string       `json:"-" gorm:"size:255;not null"`
	Phone          string       `json:"phone" gorm:"size:32"`
	Email          string       `json:"email" gorm:"size:128"`
	RoleCode       string       `json:"role_code" gorm:"size:48;index;not null"`
	OrganizationID uint64       `json:"organization_id" gorm:"index;not null"`
	WecomUserID    string       `json:"wecom_user_id" gorm:"size:128"`
	WechatOpenID   string       `json:"wechat_open_id" gorm:"size:128"`
	Status         string       `json:"status" gorm:"size:16;default:active"`
	LastLoginAt    *time.Time   `json:"last_login_at"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Organization   Organization `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	Role           Role         `json:"role,omitempty" gorm:"foreignKey:RoleCode;references:Code"`
}

type Dataset struct {
	ID           uint64    `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"size:100;not null"`
	Code         string    `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Description  string    `json:"description" gorm:"size:500"`
	SQLText      string    `json:"sql_text" gorm:"type:text;not null"`
	ScopeColumn  string    `json:"scope_column" gorm:"size:64;default:org_id"`
	RequireScope bool      `json:"require_scope" gorm:"default:true"`
	DatasourceID *uint64   `json:"datasource_id" gorm:"index"`
	Status       string    `json:"status" gorm:"size:16;default:active"`
	CreatedBy    uint64    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Datasource 业务数据源，支持 MySQL / PostgreSQL / Oracle / SQL Server / SQLite。
// DSN 使用 AES-GCM 加密后入库，接口只回传掩码。
type Datasource struct {
	ID           uint64     `json:"id" gorm:"primaryKey"`
	Name         string     `json:"name" gorm:"size:100;not null"`
	Code         string     `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Driver       string     `json:"driver" gorm:"size:32;not null"`
	DSN          string     `json:"-" gorm:"type:text;not null"`
	Status       string     `json:"status" gorm:"size:16;default:active"`
	ReadOnly     bool       `json:"read_only" gorm:"default:true"`
	MaxOpenConns int        `json:"max_open_conns" gorm:"default:10"`
	MaxIdleConns int        `json:"max_idle_conns" gorm:"default:3"`
	Remark       string     `json:"remark" gorm:"size:255"`
	LastTestAt   *time.Time `json:"last_test_at"`
	LastTestOK   bool       `json:"last_test_ok" gorm:"default:false"`
	LastTestMsg  string     `json:"last_test_msg" gorm:"size:255"`
	CreatedBy    uint64     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Target 经营目标，按组织 + 周期 + 指标维护，达成率由销售事实表实时计算。
type Target struct {
	ID          uint64    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	OrgID       uint64    `json:"org_id" gorm:"index;not null"`
	PeriodType  string    `json:"period_type" gorm:"size:16;index;not null"`
	Period      string    `json:"period" gorm:"size:32;index;not null"`
	Metric      string    `json:"metric" gorm:"size:32;index;not null"`
	TaskType    string    `json:"task_type" gorm:"size:16;index;default:base"`
	TargetValue float64   `json:"target_value" gorm:"not null"`
	Unit        string    `json:"unit" gorm:"size:16"`
	Remark      string    `json:"remark" gorm:"size:255"`
	CreatedBy   uint64    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Dashboard struct {
	ID          uint64 `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"size:100;not null"`
	Code        string `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Description string `json:"description" gorm:"size:500"`
	Layout      string `json:"-" gorm:"type:longtext"`
	Theme       string `json:"theme" gorm:"size:32;default:light"`
	Status      string `json:"status" gorm:"size:16;default:published;index"`
	Category    string `json:"category" gorm:"size:32;default:经营分析"`
	CreatedBy   uint64 `json:"created_by"`
	// 可见范围：all 全部用户 / roles 指定角色 / users 指定人员 / orgs 指定组织 / custom 组合
	VisibilityMode string `json:"visibility_mode" gorm:"size:32;default:all"`
	VisibleRoles   string `json:"-" gorm:"type:text"`
	VisibleUsers   string `json:"-" gorm:"type:text"`
	VisibleOrgs    string `json:"-" gorm:"type:text"`
	// 数据范围：role_org 按登录人组织层级 / all 全量组织
	ScopeMode string `json:"scope_mode" gorm:"size:32;default:role_org"`
	// 看板是否允许组织下钻，以及下钻默认模式
	DrillEnabled bool       `json:"drill_enabled" gorm:"default:true"`
	DrillMode    string     `json:"drill_mode" gorm:"size:32;default:level"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Template struct {
	ID          uint64    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	Description string    `json:"description" gorm:"size:500"`
	Category    string    `json:"category" gorm:"size:32"`
	Cover       string    `json:"cover" gorm:"size:64"`
	Layout      string    `json:"-" gorm:"type:longtext"`
	Theme       string    `json:"theme" gorm:"size:32;default:light"`
	BuiltIn     bool      `json:"built_in" gorm:"default:false"`
	CreatedBy   uint64    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PushTemplate struct {
	ID              uint64    `json:"id" gorm:"primaryKey"`
	Name            string    `json:"name" gorm:"size:100;not null"`
	Channel         string    `json:"channel" gorm:"size:32;not null"`
	TitleTemplate   string    `json:"title_template" gorm:"size:255;not null"`
	ContentTemplate string    `json:"content_template" gorm:"type:text;not null"`
	StyleJSON       string    `json:"style_json" gorm:"type:text"`
	Enabled         bool      `json:"enabled" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ChannelConfig struct {
	ID         uint64    `json:"id" gorm:"primaryKey"`
	Channel    string    `json:"channel" gorm:"size:32;uniqueIndex;not null"`
	Name       string    `json:"name" gorm:"size:64"`
	ConfigJSON string    `json:"-" gorm:"type:text"`
	Enabled    bool      `json:"enabled" gorm:"default:false"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Subscription struct {
	ID          uint64     `json:"id" gorm:"primaryKey"`
	Name        string     `json:"name" gorm:"size:100;not null"`
	DashboardID uint64     `json:"dashboard_id" gorm:"index;not null"`
	Cron        string     `json:"cron" gorm:"size:64;not null"`
	Timezone    string     `json:"timezone" gorm:"size:64;default:Asia/Shanghai"`
	Channels    string     `json:"-" gorm:"size:255;default:wecom"`
	Recipients  string     `json:"-" gorm:"type:text"`
	TemplateID  *uint64    `json:"template_id"`
	Enabled     bool       `json:"enabled" gorm:"default:true"`
	LastRunAt   *time.Time `json:"last_run_at"`
	NextRunAt   *time.Time `json:"next_run_at"`
	LastStatus  string     `json:"last_status" gorm:"size:32"`
	CreatedBy   uint64     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PushRecord struct {
	ID             uint64    `json:"id" gorm:"primaryKey"`
	SubscriptionID *uint64   `json:"subscription_id" gorm:"index"`
	DashboardID    *uint64   `json:"dashboard_id" gorm:"index"`
	Channel        string    `json:"channel" gorm:"size:32;index"`
	Recipient      string    `json:"recipient" gorm:"size:128"`
	Status         string    `json:"status" gorm:"size:32;index"`
	Title          string    `json:"title" gorm:"size:255"`
	Content        string    `json:"content" gorm:"type:text"`
	Error          string    `json:"error" gorm:"type:text"`
	SentAt         time.Time `json:"sent_at" gorm:"index"`
}

type ShareLink struct {
	ID           uint64     `json:"id" gorm:"primaryKey"`
	TokenHash    string     `json:"-" gorm:"size:64;uniqueIndex;not null"`
	DashboardID  uint64     `json:"dashboard_id" gorm:"index;not null"`
	OrgIDs       string     `json:"-" gorm:"type:text"`
	CreatedBy    uint64     `json:"created_by"`
	ExpiresAt    time.Time  `json:"expires_at" gorm:"index"`
	MaxViews     int        `json:"max_views" gorm:"default:0"`
	Views        int        `json:"views" gorm:"default:0"`
	Revoked      bool       `json:"revoked" gorm:"default:false"`
	LastViewedAt *time.Time `json:"last_viewed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AuditLog struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	UserID    uint64    `json:"user_id" gorm:"index"`
	Username  string    `json:"username" gorm:"size:64"`
	Action    string    `json:"action" gorm:"size:64;index"`
	Resource  string    `json:"resource" gorm:"size:128"`
	Method    string    `json:"method" gorm:"size:16"`
	Path      string    `json:"path" gorm:"size:255;index"`
	Status    int       `json:"status" gorm:"index"`
	Duration  int64     `json:"duration_ms"`
	Detail    string    `json:"detail" gorm:"type:text"`
	IP        string    `json:"ip" gorm:"size:64"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

type SalesFact struct {
	ID          uint64    `json:"id" gorm:"primaryKey"`
	OrgID       uint64    `json:"org_id" gorm:"index;not null"`
	StoreID     uint64    `json:"store_id" gorm:"index"`
	RegionID    uint64    `json:"region_id" gorm:"index"`
	BigRegionID uint64    `json:"big_region_id" gorm:"index"`
	StatDate    string    `json:"stat_date" gorm:"size:10;index;not null"`
	Revenue     float64   `json:"revenue"`
	Orders      int       `json:"orders"`
	GrossProfit float64   `json:"gross_profit"`
	Customers   int       `json:"customers"`
	Channel     string    `json:"channel" gorm:"size:32;index"`
	CreatedAt   time.Time `json:"created_at"`
}

// Employee 门店员工，用于门店下钻后的员工销售明细。
type Employee struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	OrgID     uint64    `json:"org_id" gorm:"index;not null"`
	Code      string    `json:"code" gorm:"size:64;index"`
	Name      string    `json:"name" gorm:"size:64;not null"`
	Position  string    `json:"position" gorm:"size:32"`
	Phone     string    `json:"phone" gorm:"size:32"`
	Status    string    `json:"status" gorm:"size:16;default:active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EmployeeSale 员工销售事实，按天记录，用于门店员工排行与明细下钻。
type EmployeeSale struct {
	ID          uint64    `json:"id" gorm:"primaryKey"`
	EmployeeID  uint64    `json:"employee_id" gorm:"index;not null"`
	OrgID       uint64    `json:"org_id" gorm:"index;not null"`
	StatDate    string    `json:"stat_date" gorm:"size:10;index;not null"`
	Revenue     float64   `json:"revenue"`
	Orders      int       `json:"orders"`
	GrossProfit float64   `json:"gross_profit"`
	Customers   int       `json:"customers"`
	CreatedAt   time.Time `json:"created_at"`
}

func AllModels() []any {
	return []any{
		&Organization{}, &Role{}, &User{}, &Dataset{}, &Dashboard{}, &Template{},
		&PushTemplate{}, &ChannelConfig{}, &Subscription{}, &PushRecord{},
		&ShareLink{}, &AuditLog{}, &SalesFact{}, &Datasource{}, &Target{},
		&Employee{}, &EmployeeSale{},
	}
}
