package model

import "time"

// Setting stores a runtime configuration override as a dotted key + string value.
// Values are merged into the file config at startup, so overrides survive restarts.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}
