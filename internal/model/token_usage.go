package model

import (
	"time"
)

// TokenUsage represents token usage tracking for AI model calls
type TokenUsage struct {
	ID               uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID        *string        `gorm:"type:varchar(36);index:idx_session_id" json:"session_id,omitempty"`
	Provider         string         `gorm:"type:varchar(50);not null;index:idx_provider_model,priority:1;index" json:"provider"` // zai, qwen, gemini, deepseek, openai, etc.
	Model            string         `gorm:"type:varchar(100);not null;index:idx_provider_model,priority:2;index" json:"model"`     // glm-4-flash, qwen-plus, gemini-pro, etc.
	CallType         string         `gorm:"type:varchar(50);default:'chat';index" json:"call_type"`                                 // chat, stream, vision, thinking
	PromptTokens     uint           `gorm:"type:int unsigned;default:0" json:"prompt_tokens"`
	CompletionTokens uint           `gorm:"type:int unsigned;default:0" json:"completion_tokens"`
	TotalTokens      uint           `gorm:"type:int unsigned;default:0" json:"total_tokens"`
	EstimatedCost    float64        `gorm:"type:decimal(10,6);default:0" json:"estimated_cost"`
	Metadata         string         `gorm:"type:json;default:'{}'" json:"metadata,omitempty"` // latency, error info, etc.
	CreatedAt        time.Time      `gorm:"index:idx_created_at;index:idx_created_date" json:"created_at"`
	DeletedAt        *time.Time     `gorm:"index" json:"-"`

	// Relationships
	Session *Session `gorm:"foreignKey:SessionID" json:"-"`
}
