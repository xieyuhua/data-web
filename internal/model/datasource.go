package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	DSMySQL    = "mysql"
	DSPostgres = "postgres"
	DSSQLite   = "sqlite"
	DSOracle   = "oracle"
)

type DataSource struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Name            string         `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Type            string         `gorm:"size:32;not null" json:"type"`
	Host            string         `gorm:"size:255" json:"host"`
	Port            int            `json:"port"`
	Database        string         `gorm:"size:255" json:"database"`
	Username        string         `gorm:"size:128" json:"username"`
	PasswordEnc     string         `gorm:"size:512" json:"-"`
	DSNExtra        string         `gorm:"size:512" json:"dsnExtra"`
	OptionsJSON     string         `gorm:"type:text" json:"optionsJson"`
	CreatedByID     uint           `json:"createdBy"`
	LastTestedAt    *time.Time     `json:"lastTestedAt"`
	LastTestStatus  string         `gorm:"size:16" json:"lastTestStatus"`
	LastTestMessage string         `gorm:"size:512" json:"lastTestMessage"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func (d *DataSource) HasPassword() bool { return d.PasswordEnc != "" }

type DataSourceOptions struct {
	MaxOpen     int `json:"maxOpen,omitempty"`
	MaxIdle     int `json:"maxIdle,omitempty"`
	QueryTimeoutSec int `json:"queryTimeoutSec,omitempty"`
}
