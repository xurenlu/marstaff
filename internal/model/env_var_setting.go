package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnvVarSetting stores environment variables configured in settings UI.
// These are injected into all agent-executed commands (run_command, oneoff tasks, etc.).
type EnvVarSetting struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Key       string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"key"`
	Value     string         `gorm:"type:text" json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName overrides table name
func (EnvVarSetting) TableName() string {
	return "env_var_settings"
}

// BeforeCreate creates UUID before inserting
func (e *EnvVarSetting) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}
