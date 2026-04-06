package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/gateway"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
)

// SessionsExecutor registers session collaboration tools (sessions_list, sessions_history, sessions_send)
type SessionsExecutor struct {
	engine       *agent.Engine
	executor     *agent.Executor
	sessionRepo  *repository.SessionRepository
	messageRepo  *repository.MessageRepository
	hub          *gateway.Hub
}

// NewSessionsExecutor creates a new sessions tool executor
func NewSessionsExecutor(
	engine *agent.Engine,
	exec *agent.Executor,
	sessionRepo *repository.SessionRepository,
	messageRepo *repository.MessageRepository,
	hub *gateway.Hub,
) *SessionsExecutor {
	return &SessionsExecutor{
		engine:      engine,
		executor:    exec,
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
		hub:         hub,
	}
}

// RegisterBuiltInTools registers sessions_* tools
func (e *SessionsExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("sessions_list",
		"List active sessions for the current user. Returns session id, title, model, and updated_at. Use this to discover other agents/sessions you can message.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of sessions to return (default: 20)",
				},
			},
		},
		e.toolSessionsList,
	)

	e.engine.RegisterTool("sessions_history",
		"Get recent message history for a specific session. Use to understand what another session/agent has been discussing.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The session ID to fetch history for",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of messages to return (default: 20)",
				},
			},
			"required": []string{"session_id"},
		},
		e.toolSessionsHistory,
	)

	e.engine.RegisterTool("sessions_send",
		"Send a message to another session. The target session's agent will process the message. Set wait_for_reply true to block until the assistant reply (same process as OpenClaw-style cross-session ping). The originating session_id is stored in message metadata for the target.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The target session ID to send the message to",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "The message content to send",
				},
				"wait_for_reply": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, wait synchronously for the target assistant reply (default false = fire-and-forget)",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "number",
					"description": "When wait_for_reply is true, max seconds to wait (default 120, max 600)",
				},
			},
			"required": []string{"session_id", "message"},
		},
		e.toolSessionsSend,
	)

	e.engine.RegisterTool("sessions_spawn",
		"Create a new session and send an initial message. Returns the new session ID.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Optional title for the new session",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "The initial message to send to the new session",
				},
			},
			"required": []string{"message"},
		},
		e.toolSessionsSpawn,
	)

	log.Info().Msg("sessions tools registered (sessions_list, sessions_history, sessions_send, sessions_spawn)")
}

func (e *SessionsExecutor) getUserID(ctx context.Context) (string, error) {
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		return uid, nil
	}
	return "", fmt.Errorf("user_id not found in context")

}

func (e *SessionsExecutor) toolSessionsList(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}

	limit := 20
	if l, ok := params["limit"].(float64); ok && l > 0 && l <= 100 {
		limit = int(l)
	}

	sessions, err := e.sessionRepo.GetByUserID(ctx, userID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		return "No sessions found.", nil
	}

	type sessionInfo struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Model     string `json:"model"`
		UpdatedAt string `json:"updated_at"`
	}

	var items []sessionInfo
	for _, s := range sessions {
		items = append(items, sessionInfo{
			ID:        s.ID,
			Title:     s.Title,
			Model:     s.Model,
			UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
		})
	}

	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b), nil
}

func (e *SessionsExecutor) toolSessionsHistory(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}

	sessionID, ok := params["session_id"].(string)
	if !ok || sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}

	// Verify session belongs to user
	session, err := e.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %w", err)
	}
	if session.UserID != userID {
		return "", fmt.Errorf("access denied: session %s does not belong to current user", sessionID)
	}

	limit := 20
	if l, ok := params["limit"].(float64); ok && l > 0 && l <= 100 {
		limit = int(l)
	}

	messages, err := e.messageRepo.GetLastNBySessionID(ctx, sessionID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to get history: %w", err)
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	type msgInfo struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}

	var items []msgInfo
	for _, m := range messages {
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		items = append(items, msgInfo{
			Role:      string(m.Role),
			Content:   content,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		})
	}

	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b), nil
}

func (e *SessionsExecutor) toolSessionsSend(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}

	sessionID, ok := params["session_id"].(string)
	if !ok || sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	message, ok := params["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("message is required")
	}

	waitReply := false
	if w, ok := params["wait_for_reply"].(bool); ok {
		waitReply = w
	}
	timeoutSec := 120.0
	if t, ok := params["timeout_seconds"].(float64); ok && t > 0 {
		timeoutSec = t
		if timeoutSec > 600 {
			timeoutSec = 600
		}
	}

	sourceSessionID, _ := ctx.Value(contextkeys.SessionID).(string)

	// Verify session belongs to user
	session, err := e.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %w", err)
	}
	if session.UserID != userID {
		return "", fmt.Errorf("access denied: session %s does not belong to current user", sessionID)
	}

	meta := map[string]interface{}{}
	if sourceSessionID != "" && sourceSessionID != sessionID {
		meta["cross_session_from"] = sourceSessionID
	}
	metaJSON := "{}"
	if len(meta) > 0 {
		b, err := json.Marshal(meta)
		if err != nil {
			return "", err
		}
		metaJSON = string(b)
	}

	// Add user message to target session
	userMsg := &model.Message{
		SessionID: sessionID,
		Role:      model.RoleUser,
		Content:   message,
		Metadata:  metaJSON,
	}
	if err := e.messageRepo.Create(ctx, userMsg); err != nil {
		return "", fmt.Errorf("failed to add message: %w", err)
	}

	runAgent := func(c context.Context) (*agent.ChatResponse, error) {
		modelName := session.Model
		if modelName == "default" || modelName == "" {
			modelName = ""
		}
		chatReq := &agent.ChatRequest{
			SessionID: sessionID,
			UserID:    userID,
			Messages:  []provider.Message{{Role: provider.RoleUser, Content: message}},
			Model:     modelName,
		}
		return e.executor.ExecuteWithTools(c, chatReq)
	}

	if waitReply {
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec*float64(time.Second)))
		defer cancel()
		runCtx = context.WithValue(runCtx, contextkeys.UserID, userID)
		runCtx = context.WithValue(runCtx, contextkeys.SessionID, sessionID)
		runCtx = context.WithValue(runCtx, "current_time", time.Now().Format("2006-01-02 15:04:05"))

		resp, err := runAgent(runCtx)
		if err != nil {
			return "", fmt.Errorf("sessions_send (wait): %w", err)
		}
		out := fmt.Sprintf("Reply from session %s:\n%s", sessionID, resp.Content)
		if resp.Content != "" {
			e.hub.BroadcastToSession(sessionID, string(gateway.MessageTypeContent), resp.Content)
		}
		return out, nil
	}

	// Run agent for target session (async to avoid blocking)
	go func() {
		ctx := context.WithValue(context.Background(), contextkeys.UserID, userID)
		ctx = context.WithValue(ctx, contextkeys.SessionID, sessionID)
		ctx = context.WithValue(ctx, "current_time", time.Now().Format("2006-01-02 15:04:05"))

		modelName := session.Model
		if modelName == "default" || modelName == "" {
			modelName = ""
		}
		chatReq := &agent.ChatRequest{
			SessionID: sessionID,
			UserID:    userID,
			Messages:  []provider.Message{{Role: provider.RoleUser, Content: message}},
			Model:     modelName,
		}

		resp, err := e.executor.ExecuteWithTools(ctx, chatReq)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("sessions_send: agent execution failed")
			e.hub.BroadcastToSession(sessionID, string(gateway.MessageTypeError), map[string]interface{}{
				"error": fmt.Sprintf("Agent error: %v", err),
			})
			return
		}

		if resp.Content != "" {
			e.hub.BroadcastToSession(sessionID, string(gateway.MessageTypeContent), resp.Content)
		}
	}()

	return fmt.Sprintf("Message sent to session %s. The target agent will process it and respond.", sessionID), nil
}

func (e *SessionsExecutor) toolSessionsSpawn(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}

	message, ok := params["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("message is required")
	}

	title := "New session"
	if t, ok := params["title"].(string); ok && t != "" {
		title = t
	}

	// Create new session
	session := &model.Session{
		UserID: userID,
		Title:  title,
		Model:  "default",
	}
	if err := e.sessionRepo.Create(ctx, session); err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Add user message
	userMsg := &model.Message{
		SessionID: session.ID,
		Role:      model.RoleUser,
		Content:   message,
	}
	if err := e.messageRepo.Create(ctx, userMsg); err != nil {
		return "", fmt.Errorf("failed to add message: %w", err)
	}

	// Run agent for new session (async)
	go func() {
		ctx := context.WithValue(context.Background(), contextkeys.UserID, userID)
		ctx = context.WithValue(ctx, contextkeys.SessionID, session.ID)
		ctx = context.WithValue(ctx, "current_time", time.Now().Format("2006-01-02 15:04:05"))

		modelName := session.Model
		if modelName == "default" || modelName == "" {
			modelName = ""
		}
		chatReq := &agent.ChatRequest{
			SessionID: session.ID,
			UserID:    userID,
			Messages:  []provider.Message{{Role: provider.RoleUser, Content: message}},
			Model:     modelName,
		}

		_, err := e.executor.ExecuteWithTools(ctx, chatReq)
		if err != nil {
			log.Error().Err(err).Str("session_id", session.ID).Msg("sessions_spawn: agent execution failed")
		}
	}()

	return fmt.Sprintf("Created session %s: %s. Message sent.", session.ID, title), nil
}
