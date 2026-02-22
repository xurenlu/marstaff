package adapter

import (
	"context"
	"time"
)

// Platform represents the IM platform
type Platform string

const (
	PlatformWebsocket Platform = "websocket"
	PlatformTelegram  Platform = "telegram"
	PlatformMatrix    Platform = "matrix"
	PlatformDiscord   Platform = "discord"
	PlatformSlack     Platform = "slack"
)

// Message represents a message from any platform
type Message struct {
	ID        string            `json:"id"`
	Platform  Platform          `json:"platform"`
	UserID    string            `json:"user_id"`
	SessionID string            `json:"session_id,omitempty"`
	Content   string            `json:"content"`
	Type      string            `json:"type"` // text, image, file, command
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`

	// Reply-to information
	ReplyToID  string `json:"reply_to_id,omitempty"`
	ReplyToMsg *Message `json:"reply_to_msg,omitempty"`
}

// MessageHandler is a function that handles incoming messages
type MessageHandler func(ctx context.Context, msg *Message) error

// Adapter is the interface for IM platform adapters
type Adapter interface {
	// Platform returns the platform name
	Platform() Platform

	// Start starts the adapter
	Start(ctx context.Context) error

	// Stop stops the adapter
	Stop(ctx context.Context) error

	// SendMessage sends a message to a user/session
	SendMessage(ctx context.Context, userID, sessionID, content string) error

	// SendTypingIndicator sends a typing indicator
	SendTypingIndicator(ctx context.Context, userID string) error

	// SetMessageHandler sets the message handler
	SetMessageHandler(handler MessageHandler)

	// HealthCheck checks if the adapter is healthy
	HealthCheck(ctx context.Context) error

	// IsEnabled returns whether the adapter is enabled
	IsEnabled() bool

	// SetEnabled sets the enabled state
	SetEnabled(enabled bool)
}

// BaseAdapter provides common functionality for adapters
type BaseAdapter struct {
	platform       Platform
	enabled        bool
	messageHandler MessageHandler
}

// NewBaseAdapter creates a new base adapter
func NewBaseAdapter(platform Platform) *BaseAdapter {
	return &BaseAdapter{
		platform: platform,
		enabled:  true,
	}
}

// Platform returns the platform name
func (a *BaseAdapter) Platform() Platform {
	return a.platform
}

// SetMessageHandler sets the message handler
func (a *BaseAdapter) SetMessageHandler(handler MessageHandler) {
	a.messageHandler = handler
}

// IsEnabled returns whether the adapter is enabled
func (a *BaseAdapter) IsEnabled() bool {
	return a.enabled
}

// SetEnabled sets the enabled state
func (a *BaseAdapter) SetEnabled(enabled bool) {
	a.enabled = enabled
}

// HandleMessage handles an incoming message
func (a *BaseAdapter) HandleMessage(ctx context.Context, msg *Message) error {
	if a.messageHandler != nil {
		return a.messageHandler(ctx, msg)
	}
	return nil
}
