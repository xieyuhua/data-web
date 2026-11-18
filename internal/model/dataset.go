package model

import (
	"time"

	"gorm.io/gorm"
)

type Dataset struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Name           string         `gorm:"size:128;index;not null" json:"name"`
	Description    string         `gorm:"size:512" json:"description"`
	DataSourceID   uint           `gorm:"index;not null" json:"dataSourceId"`
	SQLText        string         `gorm:"type:text;not null" json:"sqlText"`
	Version        int            `gorm:"default:1" json:"version"`
	DefaultLimit   int            `gorm:"default:1000" json:"defaultLimit"`
	ColumnMetaJSON string         `gorm:"type:text" json:"columnMetaJson"`
	CreatedByID    uint           `json:"createdBy"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Params         []DatasetParam `gorm:"foreignKey:DatasetID;constraint:OnDelete:CASCADE" json:"params"`
}

type DatasetParam struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	DatasetID    uint   `gorm:"index;not null" json:"datasetId"`
	Name         string `gorm:"size:64;not null" json:"name"`
	Label        string `gorm:"size:128" json:"label"`
	Type         string `gorm:"size:16;default:string" json:"type"`
	Required     bool   `gorm:"default:false" json:"required"`
	DefaultValue string `gorm:"size:255" json:"defaultValue"`
}

type ColumnMeta struct {
	Name        string `json:"name"`
	DBType      string `json:"dbType"`
	LogicalType string `json:"logicalType"` // string|number|bool|date|datetime
	Nullable    bool   `json:"nullable"`
	// Format is an optional display hint carried from the chart metric config
	// (plain|percent|currency|number) so clients can render ratios correctly.
	Format string `json:"format,omitempty"`
}
