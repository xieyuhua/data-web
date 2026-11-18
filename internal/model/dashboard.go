package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	WidgetChart = "chart"
	WidgetText  = "text"
)

type Dashboard struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Description string         `gorm:"size:512" json:"description"`
	Version     int            `gorm:"default:1" json:"version"`
	OwnerID     uint           `gorm:"index" json:"ownerId"`
	CreatedByID uint           `json:"createdBy"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Widgets []DashboardWidget `gorm:"foreignKey:DashboardID;constraint:OnDelete:CASCADE" json:"widgets,omitempty"`
	Filters []DashboardFilter `gorm:"foreignKey:DashboardID;constraint:OnDelete:CASCADE" json:"filters,omitempty"`
}

type DashboardWidget struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	DashboardID uint   `gorm:"index;not null" json:"dashboardId"`
	WidgetType  string `gorm:"size:16;default:chart" json:"widgetType"`
	ChartID     *uint  `gorm:"index" json:"chartId"`
	Title       string `gorm:"size:255" json:"title"`
	X           int    `gorm:"default:0" json:"x"`
	Y           int    `gorm:"default:0" json:"y"`
	W           int    `gorm:"default:6" json:"w"`
	H           int    `gorm:"default:6" json:"h"`
	OptionsJSON string `gorm:"type:text" json:"optionsJson"`
	// Bindings don't have their own table: they live in OptionsJSON? No - see FilterBinding.
	Bindings []FilterBinding `gorm:"foreignKey:WidgetID;constraint:OnDelete:CASCADE" json:"bindings,omitempty"`
}

type DashboardFilter struct {
	ID               uint   `gorm:"primaryKey" json:"id"`
	DashboardID      uint   `gorm:"index;not null" json:"dashboardId"`
	Name             string `gorm:"size:64;not null" json:"name"`
	Label            string `gorm:"size:128" json:"label"`
	FilterType       string `gorm:"size:16;default:text" json:"filterType"` // select|date|daterange|number|text
	DatasetID        *uint  `gorm:"index" json:"datasetId"`
	ValueColumn      string `gorm:"size:128" json:"valueColumn"`
	LabelColumn      string `gorm:"size:128" json:"labelColumn"`
	DefaultValueJSON string `gorm:"type:text" json:"defaultValueJson"`
	Multiple         bool   `gorm:"default:false" json:"multiple"`
	X                int    `gorm:"default:0" json:"x"`
	Y                int    `gorm:"default:0" json:"y"`
	W                int    `gorm:"default:3" json:"w"`
	H                int    `gorm:"default:1" json:"h"`
}

// FilterBinding links a dashboard filter value to a widget dataset parameter.
type FilterBinding struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	FilterID   uint   `gorm:"uniqueIndex:idx_binding;not null" json:"filterId"`
	WidgetID   uint   `gorm:"uniqueIndex:idx_binding;not null" json:"widgetId"`
	ParamName  string `gorm:"size:64;uniqueIndex:idx_binding;not null" json:"paramName"`
	ValueIndex int    `gorm:"uniqueIndex:idx_binding;default:0" json:"valueIndex"` // 0 scalar/from, 1 range-to
}

type DashboardGrant struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	DashboardID uint      `gorm:"uniqueIndex:idx_grant;not null" json:"dashboardId"`
	UserID      uint      `gorm:"uniqueIndex:idx_grant;not null" json:"userId"`
	Permission  string    `gorm:"size:16;default:view" json:"permission"` // view|edit
	CreatedAt   time.Time `json:"createdAt"`
}

type ShareLink struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	DashboardID uint       `gorm:"index;not null" json:"dashboardId"`
	TokenHash   string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Enabled     bool       `gorm:"default:true" json:"enabled"`
	CreatedByID uint       `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
}
