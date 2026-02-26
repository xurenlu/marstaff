package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Pipeline represents a workflow for complex multi-step tasks
type Pipeline struct {
	ID          uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      string         `gorm:"type:varchar(36);not null;index:idx_user_id" json:"user_id"`
	SessionID   *string        `gorm:"type:varchar(36);index:idx_session_id" json:"session_id,omitempty"`
	Name        string         `gorm:"type:varchar(255);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Status      PipelineStatus `gorm:"type:varchar(50);default:'pending';index:idx_status" json:"status"`
	Definition  PipelineDef    `gorm:"type:json;not null" json:"definition"`
	Result      json.RawMessage `gorm:"type:json" json:"result,omitempty"`
	Error       string         `gorm:"type:text" json:"error,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   *time.Time     `gorm:"index" json:"-"`

	// Relationships
	Steps []*PipelineStep `gorm:"foreignKey:PipelineID" json:"steps,omitempty"`
}

// PipelineStatus represents the status of a pipeline
type PipelineStatus string

const (
	PipelineStatusPending   PipelineStatus = "pending"
	PipelineStatusRunning   PipelineStatus = "running"
	PipelineStatusCompleted PipelineStatus = "completed"
	PipelineStatusFailed    PipelineStatus = "failed"
	PipelineStatusCancelled PipelineStatus = "cancelled"
)

// PipelineDef defines the structure of a pipeline
type PipelineDef struct {
	Steps       []PipelineStepDef `json:"steps"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	OnSuccess   []PipelineStepDef `json:"on_success,omitempty"`
	OnFailure   []PipelineStepDef `json:"on_failure,omitempty"`
	MaxDuration int               `json:"max_duration_seconds,omitempty"` // Max execution time
}

// PipelineStepDef defines a single step in the pipeline
type PipelineStepDef struct {
	Key          string                 `json:"key"`                    // Unique identifier
	Type         string                 `json:"type"`                   // task, parallel, conditional, delay, wait
	Name         string                 `json:"name,omitempty"`         // Human-readable name
	Order        int                    `json:"order"`                  // Execution order
	Dependencies []string               `json:"dependencies,omitempty"` // Step keys to wait for
	Config       map[string]interface{} `json:"config,omitempty"`       // Step-specific config
	Conditions   []StepCondition        `json:"conditions,omitempty"`   // For conditional steps
}

// StepCondition defines a condition for conditional execution
type StepCondition struct {
	Variable string      `json:"variable"` // Variable name to check
	Operator string      `json:"operator"` // eq, ne, gt, lt, contains, exists
	Value    interface{} `json:"value"`    // Value to compare against
}

// Scan implements sql.Scanner for PipelineDef
func (p *PipelineDef) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, p)
}

// Value implements driver.Valuer for PipelineDef
func (p PipelineDef) Value() (driver.Value, error) {
	if len(p.Steps) == 0 {
		return nil, nil
	}
	return json.Marshal(p)
}

// PipelineStep represents an individual step execution
type PipelineStep struct {
	ID           uint            `gorm:"primaryKey;autoIncrement" json:"id"`
	PipelineID   uint            `gorm:"not null;index:idx_pipeline_id" json:"pipeline_id"`
	StepKey      string          `gorm:"type:varchar(100);not null" json:"step_key"`
	StepType     string          `gorm:"type:varchar(50);not null" json:"step_type"`
	StepOrder    int             `gorm:"not null" json:"step_order"`
	Name         string          `json:"name,omitempty"`
	Config       json.RawMessage `gorm:"type:json" json:"config,omitempty"`
	Dependencies json.RawMessage `gorm:"type:json" json:"dependencies,omitempty"`
	Status       PipelineStatus  `gorm:"type:varchar(50);default:'pending';index" json:"status"`
	Result       json.RawMessage `gorm:"type:json" json:"result,omitempty"`
	Error        string          `gorm:"type:text" json:"error,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`

	// Relationships
	Pipeline *Pipeline `gorm:"foreignKey:PipelineID" json:"-"`
}
