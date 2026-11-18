package model

import (
	"time"

	"gorm.io/gorm"
)

// BigRegion is the top-level sales territory (e.g. 华东大区).
type BigRegion struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Code      string         `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name      string         `gorm:"size:64;not null" json:"name"`
	Pinyin    string         `gorm:"size:128;index" json:"pinyin"`   // full pinyin, e.g. huadong
	Initials  string         `gorm:"size:32;index" json:"initials"`  // initials, e.g. hd
	SortOrder int            `gorm:"default:0" json:"sortOrder"`
	Remarks   string         `gorm:"size:255" json:"remarks"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Region is the second-level territory; each belongs to a BigRegion.
type Region struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name        string         `gorm:"size:64;not null" json:"name"`
	BigRegionID uint           `gorm:"index;not null" json:"bigRegionId"`
	Pinyin      string         `gorm:"size:128;index" json:"pinyin"`
	Initials    string         `gorm:"size:32;index" json:"initials"`
	SortOrder   int            `gorm:"default:0" json:"sortOrder"`
	Remarks     string         `gorm:"size:255" json:"remarks"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// Store is an outlet. Region/big-region codes are denormalized alongside the
// foreign keys so imports and exports can round-trip by human-readable codes.
type Store struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Code          string         `gorm:"size:32;uniqueIndex;not null" json:"code"` // 门店编码
	Name          string         `gorm:"size:128;not null" json:"name"`
	BigRegionCode string         `gorm:"size:32;index" json:"bigRegionCode"`
	RegionCode    string         `gorm:"size:32;index" json:"regionCode"`
	BigRegionID   uint           `gorm:"index" json:"bigRegionId"`
	RegionID      uint           `gorm:"index" json:"regionId"`
	Address       string         `gorm:"size:255" json:"address"`
	Contact       string         `gorm:"size:64" json:"contact"`
	Phone         string         `gorm:"size:32" json:"phone"`
	Status        string         `gorm:"size:16;default:active;index" json:"status"` // active|closed
	OpenedAt      *time.Time     `json:"openedAt"`
	Remarks       string         `gorm:"size:255" json:"remarks"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
