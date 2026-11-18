package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ChartBar   = "bar"
	ChartLine  = "line"
	ChartPie  = "pie"
	ChartTable = "table"
	ChartKPI   = "kpi"
)

type Chart struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Description string         `gorm:"size:512" json:"description"`
	DatasetID   uint           `gorm:"index;not null" json:"datasetId"`
	Type        string         `gorm:"size:16;not null" json:"type"`
	ConfigJSON  string         `gorm:"type:text" json:"configJson"`
	Version     int            `gorm:"default:1" json:"version"`
	CreatedByID uint           `json:"createdBy"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// ChartConfig is stored in Chart.ConfigJSON.
type ChartConfig struct {
	ChartType     string            `json:"chartType"`
	DataMode      string            `json:"dataMode"` // aggregate|raw
	Dimensions    []FieldRef        `json:"dimensions"`
	Metrics       []MetricRef       `json:"metrics"`
	Sort          []SortSpec        `json:"sort"`
	Limit         int               `json:"limit"`
	ParamDefaults map[string]string `json:"paramDefaults"`
	Format        FormatSpec        `json:"format"`
	ECharts       map[string]any    `json:"echarts"`
}

type FieldRef struct {
	Field string `json:"field"`
	Alias string `json:"alias"`
	// Grain optionally truncates a date/datetime dimension: day|week|month|quarter|year.
	Grain string `json:"grain,omitempty"`
}

type MetricRef struct {
	Field string `json:"field"`
	Agg   string `json:"agg"`
	Alias string `json:"alias"`
	// Expr turns the metric into a calculated metric: an arithmetic expression
	// over other metric aliases, evaluated after aggregation, e.g.
	//   "sales_sum / NULLIF(target_sum, 0) * 100"
	Expr string `json:"expr,omitempty"`
	// Format controls display: plain|percent|currency|number.
	Format string `json:"format,omitempty"`
}

type SortSpec struct {
	Alias string `json:"alias"`
	Desc  bool   `json:"desc"`
}

type FormatSpec struct {
	Decimals     int    `json:"decimals"`
	NumberFormat string `json:"numberFormat"`
}

const (
	ModeAggregate = "aggregate"
	ModeRaw       = "raw"
)
