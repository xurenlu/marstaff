package adapter

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

// WebSocketAdapter implements Adapter for WebSocket clients
type WebSocketAdapter struct {
	*BaseAdapter
	hub           HubInterface
	messageMap    sync.Map // sessionID -> userID mapping
}

// HubInterface defines the interface for interacting with the WebSocket hub
type HubInterface interface {
	Broadcast(msg *HubMessage)
	SendToUser(userID string, msg *HubMessage)
	SendToSession(sessionID string, msg *HubMessage)
}

// HubMessage represents a message sent through the hub
type HubMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	UserID    string      `json:"user_id,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// NewWebSocketAdapter creates a new WebSocket adapter
func NewWebSocketAdapter(hub HubInterface) *WebSocketAdapter {
	return &WebSocketAdapter{
		BaseAdapter: NewBaseAdapter(PlatformWebsocket),
		hub:         hub,
	}
}

func (a *WebSocketAdapter) Start(ctx context.Context) error {
	log.Info().Msg("websocket adapter ready (hub manages connections)")
	return nil
}

func (a *WebSocketAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("websocket adapter stopped")
	return nil
}

func (a *WebSocketAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	msg := &HubMessage{
		Type:      "chat",
		UserID:    userID,
		SessionID: sessionID,
		Data: map[string]interface{}{
			"content": content,
		},
	}

	// Send to session if specified
	if sessionID != "" {
		a.hub.SendToSession(sessionID, msg)
	} else {
		// Otherwise send to user
		a.hub.SendToUser(userID, msg)
	}

	// Update mapping
	if sessionID != "" && userID != "" {
		a.messageMap.Store(sessionID, userID)
	}

	return nil
}

func (a *WebSocketAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	msg := &HubMessage{
		Type:   "typing",
		UserID: userID,
		Data: map[string]interface{}{
			"typing": true,
		},
	}

	a.hub.SendToUser(userID, msg)
	return nil
}

func (a *WebSocketAdapter) HealthCheck(ctx context.Context) error {
	if a.hub == nil {
		return fmt.Errorf("hub not initialized")
	}
	return nil
}

// RegisterSession associates a session with a user
func (a *WebSocketAdapter) RegisterSession(sessionID, userID string) {
	a.messageMap.Store(sessionID, userID)
}

// UnregisterSession removes a session association
func (a *WebSocketAdapter) UnregisterSession(sessionID string) {
	a.messageMap.Delete(sessionID)
}

// GetUserForSession returns the user ID for a session
func (a *WebSocketAdapter) GetUserForSession(sessionID string) (string, bool) {
	if userID, ok := a.messageMap.Load(sessionID); ok {
		return userID.(string), true
	}
	return "", false
}
