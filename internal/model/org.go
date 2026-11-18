package model

import "time"

// Department mirrors a node of the 企业微信 organization tree.
type Department struct {
	ID           uint      `gorm:"primaryKey" json:"id"` // local id
	WeComID      int       `gorm:"uniqueIndex" json:"wecomId"`
	Name         string    `gorm:"size:128;not null" json:"name"`
	ParentWeComID int      `gorm:"index" json:"parentWecomId"`
	SortOrder    int       `json:"sortOrder"`
	SyncedAt     time.Time `json:"syncedAt"`
}

// DashboardDeptGrant grants a dashboard to a department; employees in that
// department (including sub-departments) can view it.
type DashboardDeptGrant struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DashboardID  uint      `gorm:"uniqueIndex:idx_dept_grant;not null" json:"dashboardId"`
	WeComDeptID  int       `gorm:"uniqueIndex:idx_dept_grant;not null" json:"wecomDeptId"`
	Permission   string    `gorm:"size:16;default:view" json:"permission"`
	CreatedAt    time.Time `json:"createdAt"`
}

// SyncState records the last successful contacts sync.
type SyncState struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:64;uniqueIndex;not null" json:"key"`
	LastRunAt time.Time `json:"lastRunAt"`
	Status    string    `gorm:"size:16" json:"status"`
	Message   string    `gorm:"size:512" json:"message"`
	DeptCount int       `json:"deptCount"`
	UserCount int       `json:"userCount"`
}
