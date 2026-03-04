package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project represents a programming project that sessions can be associated with
type Project struct {
	ID                string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID            string         `gorm:"type:varchar(36);not null;index" json:"user_id"`
	Name              string         `gorm:"type:varchar(255);not null" json:"name"`
	Description       string         `gorm:"type:text" json:"description,omitempty"`
	WorkDir           string         `gorm:"type:varchar(1024);not null" json:"work_dir"`
	Template          string         `gorm:"type:varchar(100)" json:"template,omitempty"` // react, go, python, nodejs, custom
	TechStack         string         `gorm:"type:json" json:"tech_stack,omitempty"`       // JSON array of tech tags
	Metadata          string         `gorm:"type:json" json:"metadata,omitempty"`
	MaxConcurrentBranches int        `gorm:"type:int;default:3" json:"max_concurrent_branches"` // Maximum active branches at once
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User     *User      `gorm:"foreignKey:UserID" json:"-"`
	Sessions []*Session `gorm:"foreignKey:ProjectID" json:"sessions,omitempty"`
}

// BeforeCreate creates a UUID before inserting
func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return p.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (p *Project) BeforeSave(tx *gorm.DB) error {
	return p.normalizeJSONColumns()
}

func (p *Project) normalizeJSONColumns() error {
	// MySQL JSON column rejects empty string; use "{}"/"[]" (SetColumn in hooks is unreliable per go-gorm/gorm#4990)
	if p.Metadata == "" {
		p.Metadata = "{}"
	}
	if p.TechStack == "" {
		p.TechStack = "[]"
	}
	return nil
}

// ProjectTemplate represents a project template definition
type ProjectTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TechStack   []string `json:"tech_stack"`
}

// ProjectListOptions defines filtering options for listing projects
type ProjectListOptions struct {
	UserID    string `json:"user_id"`
	Template  string `json:"template,omitempty"`
	Search    string `json:"search,omitempty"` // Search by name or description
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// CreateProjectRequest defines the request to create a project
type CreateProjectRequest struct {
	UserID      string   `json:"user_id"`
	Name        string   `json:"name" binding:"required,min=1,max=255"`
	Description string   `json:"description,omitempty"`
	WorkDir     string   `json:"work_dir" binding:"required"`
	Template    string   `json:"template,omitempty"`
	TechStack   []string `json:"tech_stack,omitempty"`
}

// UpdateProjectRequest defines the request to update a project
type UpdateProjectRequest struct {
	Name        string   `json:"name,omitempty" binding:"max=255"`
	Description string   `json:"description,omitempty"`
	TechStack   []string `json:"tech_stack,omitempty"`
}
