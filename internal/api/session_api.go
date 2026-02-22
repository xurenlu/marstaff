package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
)

// SessionAPI handles session management
type SessionAPI struct {
	userRepo    *repository.UserRepository
	sessionRepo *repository.SessionRepository
	messageRepo *repository.MessageRepository
	memoryRepo  *repository.MemoryRepository
}

// NewSessionAPI creates a new session API
func NewSessionAPI(db *gorm.DB) *SessionAPI {
	return &SessionAPI{
		userRepo:    repository.NewUserRepository(db),
		sessionRepo: repository.NewSessionRepository(db),
		messageRepo: repository.NewMessageRepository(db),
		memoryRepo:  repository.NewMemoryRepository(db),
	}
}

// CreateSessionRequest is a request to create a session
type CreateSessionRequest struct {
	UserID    string `json:"user_id"`
	Platform  string `json:"platform"`
	Title     string `json:"title,omitempty"`
	Model     string `json:"model,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

// CreateSessionResponse is a response for creating a session
type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
}

// CreateSession creates a new session
func (api *SessionAPI) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Get or create user
	user, err := api.userRepo.GetOrCreateByPlatformID(ctx, req.Platform, req.UserID, req.UserID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get or create user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	// Create session
	session := &model.Session{
		ID:     uuid.New().String(),
		UserID: user.ID,
		Title:  req.Title,
		Model:  req.Model,
	}

	if req.ParentID != "" {
		session.ParentID = &req.ParentID
	}

	if err := api.sessionRepo.Create(ctx, session); err != nil {
		log.Error().Err(err).Msg("failed to create session")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, CreateSessionResponse{
		SessionID: session.ID,
		UserID:    user.ID,
		Title:     session.Title,
		Model:     session.Model,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
	})
}

// GetSession retrieves a session
func (api *SessionAPI) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	session, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Load messages
	messages, err := api.messageRepo.GetBySessionID(ctx, sessionID, 0)
	if err != nil {
		log.Error().Err(err).Msg("failed to load messages")
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         session.ID,
		"user_id":    session.UserID,
		"title":      session.Title,
		"model":      session.Model,
		"parent_id":  session.ParentID,
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
		"messages":   messages,
	})
}

// ListSessions lists sessions for a user
func (api *SessionAPI) ListSessions(c *gin.Context) {
	userID := c.Query("user_id")
	platform := c.Query("platform")
	limit := 50

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	ctx := c.Request.Context()

	// Get user by platform ID
	var user *model.User
	if platform != "" && userID != "" {
		user, _ = api.userRepo.GetByPlatformID(ctx, platform, userID)
	}

	if user == nil {
		c.JSON(http.StatusOK, gin.H{"sessions": []*model.Session{}})
		return
	}

	sessions, err := api.sessionRepo.GetByUserID(ctx, user.ID, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to list sessions")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// DeleteSession deletes a session
func (api *SessionAPI) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	// Delete messages first
	if err := api.messageRepo.DeleteBySessionID(ctx, sessionID); err != nil {
		log.Error().Err(err).Msg("failed to delete messages")
	}

	// Delete session
	if err := api.sessionRepo.Delete(ctx, sessionID); err != nil {
		log.Error().Err(err).Msg("failed to delete session")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// AddMessage adds a message to a session
func (api *SessionAPI) AddMessage(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	if req.Role != string(model.RoleUser) && req.Role != string(model.RoleAssistant) &&
		req.Role != string(model.RoleSystem) && req.Role != string(model.RoleTool) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	message := &model.Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      model.MessageRole(req.Role),
		Content:   req.Content,
	}

	if err := api.messageRepo.Create(ctx, message); err != nil {
		log.Error().Err(err).Msg("failed to create message")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

// GetMessages retrieves messages for a session
func (api *SessionAPI) GetMessages(c *gin.Context) {
	sessionID := c.Param("id")
	limit := 100

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	ctx := c.Request.Context()

	messages, err := api.messageRepo.GetBySessionID(ctx, sessionID, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to get messages")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// GetHistory retrieves chat history for the agent
func (api *SessionAPI) GetHistory(ctx context.Context, sessionID string, limit int) ([]provider.Message, error) {
	if limit == 0 {
		limit = 100
	}

	messages, err := api.messageRepo.GetBySessionID(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]provider.Message, len(messages))
	for i, msg := range messages {
		result[i] = provider.Message{
			Role:    provider.MessageRole(msg.Role),
			Content: msg.Content,
		}
	}

	return result, nil
}

// SaveMessages saves messages to the database
func (api *SessionAPI) SaveMessages(ctx context.Context, sessionID string, messages []provider.Message) error {
	var dbMessages []*model.Message
	for _, msg := range messages {
		dbMessages = append(dbMessages, &model.Message{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Role:      model.MessageRole(msg.Role),
			Content:   msg.Content,
		})
	}

	if len(dbMessages) > 0 {
		return api.messageRepo.CreateBatch(ctx, dbMessages)
	}

	return nil
}

// SetMemory sets a memory value for a user
func (api *SessionAPI) SetMemory(c *gin.Context) {
	userID := c.Param("user_id")
	ctx := c.Request.Context()

	var req struct {
		Key     string                 `json:"key"`
		Value   string                 `json:"value"`
		Category string                 `json:"category,omitempty"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from platform ID
	user, err := api.userRepo.GetByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	memory := &model.Memory{
		ID:     uuid.New().String(),
		UserID: user.ID,
		Key:    req.Key,
		Value:  req.Value,
	}

	if req.Category != "" {
		memory.Category = model.MemoryCategory(req.Category)
	}

	if err := api.memoryRepo.Set(ctx, memory); err != nil {
		log.Error().Err(err).Msg("failed to set memory")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set memory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetMemory retrieves memory for a user
func (api *SessionAPI) GetMemory(c *gin.Context) {
	userID := c.Param("user_id")
	category := c.Query("category")
	ctx := c.Request.Context()

	// Get user
	user, err := api.userRepo.GetByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var memories []*model.Memory
	if category != "" {
		memories, err = api.memoryRepo.GetByCategory(ctx, user.ID, model.MemoryCategory(category))
	} else {
		memories, err = api.memoryRepo.GetAll(ctx, user.ID)
	}

	if err != nil {
		log.Error().Err(err).Msg("failed to get memory")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get memory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"memories": memories})
}

// CreateSessionDirect creates a new session directly (helper method)
func (api *SessionAPI) CreateSessionDirect(ctx context.Context, req *CreateSessionRequest) (*CreateSessionResponse, error) {
	// Get or create user
	user, err := api.userRepo.GetOrCreateByPlatformID(ctx, req.Platform, req.UserID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}

	// Create session
	session := &model.Session{
		ID:     uuid.New().String(),
		UserID: user.ID,
		Title:  req.Title,
		Model:  req.Model,
	}

	if req.ParentID != "" {
		session.ParentID = &req.ParentID
	}

	if err := api.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &CreateSessionResponse{
		SessionID: session.ID,
		UserID:    user.ID,
		Title:     session.Title,
		Model:     session.Model,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
	}, nil
}

// AddMessageRequest is a request to add a message
type AddMessageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AddMessageToSession adds a message to a session (helper method)
func (api *SessionAPI) AddMessageToSession(ctx context.Context, sessionID string, req *AddMessageRequest) error {
	// Validate role
	if req.Role != string(model.RoleUser) && req.Role != string(model.RoleAssistant) &&
		req.Role != string(model.RoleSystem) && req.Role != string(model.RoleTool) {
		return fmt.Errorf("invalid role: %s", req.Role)
	}

	message := &model.Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      model.MessageRole(req.Role),
		Content:   req.Content,
	}

	if err := api.messageRepo.Create(ctx, message); err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	return nil
}
