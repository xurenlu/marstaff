package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MessageRole represents the role of a message sender
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// ToolCall represents a tool/function call
type ToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function ToolCallFunction       `json:"function"`
}

// ToolCallFunction represents the function call details
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCalls is a custom type for storing tool calls as JSON
type ToolCalls []ToolCall

// Scan implements sql.Scanner for ToolCalls
func (tc *ToolCalls) Scan(value interface{}) error {
	if value == nil {
		*tc = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, tc)
}

// Value implements driver.Valuer for ToolCalls
func (tc ToolCalls) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

// Message represents a message in a conversation
type Message struct {
	ID         string      `gorm:"type:varchar(36);primaryKey" json:"id"`
	SessionID  string      `gorm:"type:varchar(36);not null;index:idx_session_id" json:"session_id"`
	Role       MessageRole `gorm:"type:varchar(20);not null" json:"role"`
	Content    string      `gorm:"type:text;not null" json:"content"`
	ToolCalls  ToolCalls   `gorm:"type:json" json:"tool_calls,omitempty"`
	ToolCallID string      `gorm:"type:varchar(100)" json:"tool_call_id,omitempty"`
	Metadata   string      `gorm:"type:json;default:NULL" json:"metadata,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`

	// Usage statistics
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`

	// Relationships
	Session *Session `gorm:"foreignKey:SessionID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	// MySQL JSON column rejects empty string; use "{}" (SetColumn in hooks is unreliable per go-gorm/gorm#4990)
	if m.Metadata == "" {
		m.Metadata = "{}"
	}
	return nil
}

// BeforeSave normalizes metadata before create/update
func (m *Message) BeforeSave(tx *gorm.DB) error {
	if m.Metadata == "" {
		m.Metadata = "{}"
	}
	return nil
}
