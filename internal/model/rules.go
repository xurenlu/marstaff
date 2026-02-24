package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Rule represents a system prompt rule or instruction
type Rule struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Content     string         `gorm:"type:text;not null" json:"content"`       // The actual system prompt/rule content
	Enabled     bool           `gorm:"default:true;index:idx_enabled" json:"enabled"`
	IsActive    bool           `gorm:"default:false" json:"is_active"` // Whether this is the active rule for the user
	IsBuiltin   bool           `gorm:"default:false" json:"is_builtin"` // Built-in rules cannot be deleted
	UserID      string         `gorm:"type:varchar(36);index" json:"user_id"` // Empty means global rule
	Category    string         `gorm:"type:varchar(50)" json:"category,omitempty"` // e.g., "coding", "writing", "general"
	Tags        string         `gorm:"type:varchar(255)" json:"tags,omitempty"` // Comma-separated tags
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (r *Rule) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}

// MCPServer represents an MCP (Model Context Protocol) server configuration
type MCPServer struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Endpoint    string         `gorm:"type:varchar(500);not null" json:"endpoint"` // MCP server endpoint URL
	Enabled     bool           `gorm:"default:true;index:idx_enabled" json:"enabled"`
	UserID      string         `gorm:"type:varchar(36);index" json:"user_id"` // Empty means global server
	Config      string         `gorm:"type:json" json:"config,omitempty"` // Additional configuration (headers, auth, etc.)
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (m *MCPServer) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return m.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (m *MCPServer) BeforeSave(tx *gorm.DB) error {
	return m.normalizeJSONColumns()
}

func (m *MCPServer) normalizeJSONColumns() error {
	if m.Config == "" {
		m.Config = "{}"
	}
	return nil
}

// MCPTool represents a tool available from an MCP server
type MCPTool struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ServerID    string         `gorm:"type:varchar(36);index:idx_server_id;not null" json:"server_id"`
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	InputSchema string         `gorm:"type:json" json:"input_schema,omitempty"` // JSON schema for tool input
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`

	// Relationships
	Server *MCPServer `gorm:"foreignKey:ServerID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (t *MCPTool) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return t.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (t *MCPTool) BeforeSave(tx *gorm.DB) error {
	return t.normalizeJSONColumns()
}

func (t *MCPTool) normalizeJSONColumns() error {
	if t.InputSchema == "" {
		t.InputSchema = "{}"
	}
	return nil
}
