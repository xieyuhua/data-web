package model

import "time"

type ScheduleTask struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:128;not null" json:"name"`
	DashboardID     uint       `gorm:"index;not null" json:"dashboardId"`
	CronExpr        string     `gorm:"size:64;not null" json:"cronExpr"`
	Timezone        string     `gorm:"size:64;default:Local" json:"timezone"`
	Enabled         bool       `gorm:"default:true;index" json:"enabled"`
	DeliveryChannel string     `gorm:"size:16;default:email" json:"deliveryChannel"` // email|wecom
	MessageFormat   string     `gorm:"size:16;default:card" json:"messageFormat"`     // card|text (企微)
	LinkExpireHours int        `gorm:"default:72" json:"linkExpireHours"`             // 分享链接有效期（小时）
	Format          string     `gorm:"size:16;default:xlsx" json:"format"`
	RecipientsJSON  string     `gorm:"type:text" json:"recipientsJson"`
	CreatedByID     uint       `json:"createdBy"`
	NextRunAt       *time.Time `json:"nextRunAt"`
	LastRunStatus   string     `gorm:"size:16" json:"lastRunStatus"`
	LastRunAt       *time.Time `json:"lastRunAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ScheduleRun struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	TaskID       uint       `gorm:"index;not null" json:"taskId"`
	TriggeredBy  string     `gorm:"size:16" json:"triggeredBy"` // cron|manual
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	Status       string     `gorm:"size:16;index" json:"status"` // running|success|failed
	ErrorMessage string     `gorm:"size:1024" json:"errorMessage"`
	FilePath     string     `gorm:"size:512" json:"filePath"`
	FileSize     int64      `json:"fileSize"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"userId"`
	Action       string    `gorm:"size:64;index" json:"action"`
	ResourceType string    `gorm:"size:32" json:"resourceType"`
	ResourceID   uint      `json:"resourceId"`
	IP           string    `gorm:"size:64" json:"ip"`
	DetailJSON   string    `gorm:"type:text" json:"detailJson"`
	CreatedAt    time.Time `json:"createdAt"`
}
