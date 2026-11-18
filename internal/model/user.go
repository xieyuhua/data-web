package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	SourceLocal = "local"
	SourceWeCom = "wecom"
)

type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	DisplayName  string         `gorm:"size:128" json:"displayName"`
	Email        string         `gorm:"size:255" json:"email"`
	Role         string         `gorm:"size:16;default:user;index" json:"role"`
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	LastLoginAt  *time.Time     `json:"lastLoginAt"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	// Organization binding (企业微信)
	Source        string `gorm:"size:16;default:local;index" json:"source"`
	WeComUserID   string `gorm:"size:128;index" json:"wecomUserId"`
	WeComDeptIDs  string `gorm:"size:512" json:"wecomDeptIds"` // JSON array of wecom department ids
	Position      string `gorm:"size:128" json:"position"`
	Mobile        string `gorm:"size:32" json:"mobile"`
}

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }
