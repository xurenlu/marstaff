package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/provider"
)

// Memory handles conversation history and persistent storage
type Memory struct {
	// TODO: Add database connection
	sessions map[string][]provider.Message
	mu       sync.RWMutex
}

// NewMemory creates a new memory instance
func NewMemory(db interface{}) *Memory {
	return &Memory{
		sessions: make(map[string][]provider.Message),
	}
}

// SaveMessages saves messages for a session
func (m *Memory) SaveMessages(ctx context.Context, sessionID string, messages ...provider.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[sessionID]; !exists {
		m.sessions[sessionID] = []provider.Message{}
	}

	m.sessions[sessionID] = append(m.sessions[sessionID], messages...)

	log.Debug().
		Str("session_id", sessionID).
		Int("count", len(messages)).
		Msg("saved messages to memory")

	// TODO: Persist to database

	return nil
}

// GetHistory retrieves message history for a session
func (m *Memory) GetHistory(ctx context.Context, sessionID string, limit int) ([]provider.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages, exists := m.sessions[sessionID]
	if !exists {
		return []provider.Message{}, nil
	}

	if limit > 0 && len(messages) > limit {
		// Return the last N messages
		return messages[len(messages)-limit:], nil
	}

	return messages, nil
}

// Clear clears all messages for a session
func (m *Memory) Clear(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)

	log.Debug().Str("session_id", sessionID).Msg("cleared session memory")

	// TODO: Clear from database

	return nil
}

// GetSessions returns all session IDs
func (m *Memory) GetSessions(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		sessions = append(sessions, sessionID)
	}

	return sessions, nil
}

// GetSessionStats returns statistics for a session
func (m *Memory) GetSessionStats(ctx context.Context, sessionID string) (*SessionStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	stats := &SessionStats{
		MessageCount: len(messages),
	}

	// Count by role
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			stats.UserMessages++
		case provider.RoleAssistant:
			stats.AssistantMessages++
		case provider.RoleSystem:
			stats.SystemMessages++
		case provider.RoleTool:
			stats.ToolMessages++
		}
	}

	return stats, nil
}

// SessionStats holds session statistics
type SessionStats struct {
	MessageCount      int
	UserMessages      int
	AssistantMessages int
	SystemMessages    int
	ToolMessages      int
}
