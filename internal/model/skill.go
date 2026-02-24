package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Skill represents a skill/capability in the system
type Skill struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Category    string         `gorm:"type:varchar(50);index:idx_category" json:"category"`
	Version     string         `gorm:"type:varchar(20)" json:"version"`
	Author      string         `gorm:"type:varchar(100)" json:"author"`
	Content     string         `gorm:"type:text;not null" json:"content"`
	Metadata    string         `gorm:"type:json" json:"metadata,omitempty"`
	Enabled     bool           `gorm:"default:true;index:idx_enabled" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Tools []*Tool `gorm:"foreignKey:SkillID" json:"tools,omitempty"`
}

// BeforeCreate creates a UUID before inserting
func (s *Skill) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return s.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (s *Skill) BeforeSave(tx *gorm.DB) error {
	return s.normalizeJSONColumns()
}

func (s *Skill) normalizeJSONColumns() error {
	if s.Metadata == "" {
		s.Metadata = "{}"
	}
	return nil
}

// Tool represents a tool/function that can be called
type Tool struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Parameters  string         `gorm:"type:json" json:"parameters"`
	HandlerName string         `gorm:"type:varchar(100);not null" json:"handler_name"`
	SkillID     *string        `gorm:"type:varchar(36);index:idx_skill_id" json:"skill_id,omitempty"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`

	// Relationships
	Skill *Skill `gorm:"foreignKey:SkillID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (t *Tool) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return t.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (t *Tool) BeforeSave(tx *gorm.DB) error {
	return t.normalizeJSONColumns()
}

func (t *Tool) normalizeJSONColumns() error {
	// Parameters is JSON schema; empty string is invalid
	if t.Parameters == "" {
		t.Parameters = "{}"
	}
	return nil
}
