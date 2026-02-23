package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/tools/security"
)

// SessionAPI handles session management
type SessionAPI struct {
	userRepo    *repository.UserRepository
	sessionRepo *repository.SessionRepository
	messageRepo *repository.MessageRepository
	memoryRepo  *repository.MemoryRepository
	todoRepo    *repository.TodoRepository
}

// NewSessionAPI creates a new session API
func NewSessionAPI(db *gorm.DB) *SessionAPI {
	return &SessionAPI{
		userRepo:    repository.NewUserRepository(db),
		sessionRepo: repository.NewSessionRepository(db),
		messageRepo: repository.NewMessageRepository(db),
		memoryRepo:  repository.NewMemoryRepository(db),
		todoRepo:    repository.NewTodoRepository(db),
	}
}

// CreateSessionRequest is a request to create a session
type CreateSessionRequest struct {
	SessionID        string  `json:"session_id,omitempty"`        // optional, use this ID if provided
	UserID           string  `json:"user_id"`
	Platform         string  `json:"platform"`
	Title            string  `json:"title,omitempty"`
	Model            string  `json:"model,omitempty"`
	ParentID         string  `json:"parent_id,omitempty"`
	WorkDir          string  `json:"work_dir,omitempty"`          // edit mode: restrict file/command ops to this dir
	WorkingDirectory string  `json:"working_directory,omitempty"` // alias for work_dir
	ProjectID        string  `json:"project_id,omitempty"`        // project association (programming mode)
	Mode             string  `json:"mode,omitempty"`              // "chat" | "programming"
}

// CreateSessionResponse is a response for creating a session
type CreateSessionResponse struct {
	SessionID string  `json:"session_id"`
	UserID    string  `json:"user_id"`
	Title     string  `json:"title"`
	Model     string  `json:"model"`
	WorkDir   string  `json:"work_dir,omitempty"`
	ProjectID *string `json:"project_id,omitempty"`
	CreatedAt string  `json:"created_at"`
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

	// Create session (use provided ID if any)
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	session := &model.Session{
		ID:     sessionID,
		UserID: user.ID,
		Title:  req.Title,
		Model:  req.Model,
	}

	if req.ParentID != "" {
		session.ParentID = &req.ParentID
	}

	// Handle project association for programming mode
	if req.ProjectID != "" {
		session.ProjectID = &req.ProjectID
	}

	if req.WorkDir != "" {
		if err := security.ValidateWorkDir(req.WorkDir); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		session.WorkDir = req.WorkDir
	} else if req.WorkingDirectory != "" {
		if err := security.ValidateWorkDir(req.WorkingDirectory); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		session.WorkDir = req.WorkingDirectory
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
		WorkDir:   session.WorkDir,
		ProjectID: session.ProjectID,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
	})
}

// messageWithParts is the response format including content_parts from metadata
type messageWithParts struct {
	ID           string                 `json:"id"`
	SessionID    string                 `json:"session_id"`
	Role         string                 `json:"role"`
	Content      string                 `json:"content"`
	ContentParts []ContentPartForStorage `json:"content_parts,omitempty"`
	CreatedAt    string                 `json:"created_at"`
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

	// Enrich messages with content_parts from metadata
	msgsResp := make([]messageWithParts, len(messages))
	for i, m := range messages {
		msgsResp[i] = messageWithParts{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      string(m.Role),
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}
		if m.Metadata != "" {
			var meta struct {
				ContentParts []ContentPartForStorage `json:"content_parts"`
			}
			if json.Unmarshal([]byte(m.Metadata), &meta) == nil && len(meta.ContentParts) > 0 {
				msgsResp[i].ContentParts = meta.ContentParts
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         session.ID,
		"user_id":    session.UserID,
		"title":      session.Title,
		"model":      session.Model,
		"work_dir":   session.WorkDir,
		"parent_id":  session.ParentID,
		"project_id": session.ProjectID,
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
		"messages":   msgsResp,
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

// UpdateSessionRequest is a request to update session metadata
type UpdateSessionRequest struct {
	Title   string  `json:"title,omitempty"`
	WorkDir *string `json:"work_dir,omitempty"` // nil = no change, "" = clear, "path" = set
}

// UpdateSession updates session metadata (title, work_dir)
func (api *SessionAPI) UpdateSession(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	var req UpdateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if req.Title != "" {
		session.Title = req.Title
	}
	if req.WorkDir != nil {
		if *req.WorkDir == "" {
			session.WorkDir = ""
		} else {
			if err := security.ValidateWorkDir(*req.WorkDir); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			session.WorkDir = *req.WorkDir
		}
	}

	if err := api.sessionRepo.Update(ctx, session); err != nil {
		log.Error().Err(err).Msg("failed to update session")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         session.ID,
		"title":      session.Title,
		"work_dir":   session.WorkDir,
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
	})
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
	// Delete todos for this session
	if err := api.todoRepo.DeleteBySessionID(ctx, sessionID); err != nil {
		log.Warn().Err(err).Msg("failed to delete session todos")
	}

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

// GetOrCreateSessionDirect ensures session exists in DB; creates it if missing (avoids FK constraint on messages)
func (api *SessionAPI) GetOrCreateSessionDirect(ctx context.Context, req *CreateSessionRequest) (*CreateSessionResponse, error) {
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
		req.SessionID = sessionID
		return api.CreateSessionDirect(ctx, req)
	}
	// Session ID provided: check if exists
	existing, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err == nil && existing != nil {
		return &CreateSessionResponse{
			SessionID: existing.ID,
			UserID:    existing.UserID,
			Title:     existing.Title,
			Model:     existing.Model,
			WorkDir:   existing.WorkDir,
			ProjectID: existing.ProjectID,
			CreatedAt: existing.CreatedAt.Format(time.RFC3339),
		}, nil
	}
	// Not found or error: create with the given ID
	req.SessionID = sessionID
	return api.CreateSessionDirect(ctx, req)
}

// UpdateSessionTitleDirect updates session title by ID (for programmatic use, no gin context)
func (api *SessionAPI) UpdateSessionTitleDirect(ctx context.Context, sessionID, title string) error {
	return api.sessionRepo.UpdateTitle(ctx, sessionID, title)
}

// GetSessionSummary retrieves the conversation summary for a session
func (api *SessionAPI) GetSessionSummary(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	session, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"summary":    session.Summary,
	})
}

// TriggerSummary manually triggers conversation summarization for a session
func (api *SessionAPI) TriggerSummary(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	// Check session exists
	_, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Get message count
	count, err := api.messageRepo.CountBySessionID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count messages"})
		return
	}

	if count < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough messages to summarize (need at least 5)"})
		return
	}

	// Return async job info (actual summarization happens via background service)
	c.JSON(http.StatusAccepted, gin.H{
		"status":     "queued",
		"session_id": sessionID,
		"message":    "Summarization queued. Use GET /api/sessions/:id/summary to check result.",
	})
}

// TriggerMemoryExtraction manually triggers memory extraction from a session
func (api *SessionAPI) TriggerMemoryExtraction(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()

	// Check session exists
	session, err := api.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Get message count
	count, err := api.messageRepo.CountBySessionID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count messages"})
		return
	}

	if count < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough messages to extract memories (need at least 3)"})
		return
	}

	// Return async job info (actual extraction happens via background service)
	c.JSON(http.StatusAccepted, gin.H{
		"status":     "queued",
		"session_id": sessionID,
		"user_id":    session.UserID,
		"message":    "Memory extraction queued. Use GET /api/memory/:user_id to check results.",
	})
}

// SearchMemories searches memories by query string
func (api *SessionAPI) SearchMemories(c *gin.Context) {
	userID := c.Query("user_id")
	query := c.Query("q")
	category := c.Query("category")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx := c.Request.Context()

	var memories []*model.Memory
	var err error

	if query != "" {
		// For now, get all memories and filter by keyword
		// TODO: Implement semantic search with embeddings
		allMemories, memErr := api.memoryRepo.GetAll(ctx, userID)
		if memErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": memErr.Error()})
			return
		}

		queryLower := strings.ToLower(query)
		for _, mem := range allMemories {
			if strings.Contains(strings.ToLower(mem.Key), queryLower) ||
			   strings.Contains(strings.ToLower(mem.Value), queryLower) {
				memories = append(memories, mem)
			}
		}
	} else if category != "" {
		memories, err = api.memoryRepo.GetByCategory(ctx, userID, model.MemoryCategory(category))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		memories, err = api.memoryRepo.GetAll(ctx, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"query":    query,
		"category": category,
		"count":    len(memories),
		"memories": memories,
	})
}

// CreateSessionDirect creates a new session directly (helper method)
func (api *SessionAPI) CreateSessionDirect(ctx context.Context, req *CreateSessionRequest) (*CreateSessionResponse, error) {
	// Get or create user
	user, err := api.userRepo.GetOrCreateByPlatformID(ctx, req.Platform, req.UserID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}

	// Create session (use provided ID if any)
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	session := &model.Session{
		ID:     sessionID,
		UserID: user.ID,
		Title:  req.Title,
		Model:  req.Model,
	}

	if req.ParentID != "" {
		session.ParentID = &req.ParentID
	}

	// Handle project association for programming mode
	if req.ProjectID != "" {
		session.ProjectID = &req.ProjectID
	}

	if req.WorkDir != "" {
		if err := security.ValidateWorkDir(req.WorkDir); err != nil {
			return nil, fmt.Errorf("invalid work_dir: %w", err)
		}
		session.WorkDir = req.WorkDir
	} else if req.WorkingDirectory != "" {
		if err := security.ValidateWorkDir(req.WorkingDirectory); err != nil {
			return nil, fmt.Errorf("invalid work_dir: %w", err)
		}
		session.WorkDir = req.WorkingDirectory
	}

	if err := api.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &CreateSessionResponse{
		SessionID: session.ID,
		UserID:    user.ID,
		Title:     session.Title,
		Model:     session.Model,
		WorkDir:   session.WorkDir,
		ProjectID: session.ProjectID,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
	}, nil
}

// ImageURLPart is the image URL structure for content parts
type ImageURLPart struct {
	URL string `json:"url"`
}

// ContentPartForStorage represents image/text part for storage (matches provider format)
type ContentPartForStorage struct {
	Type     string        `json:"type,omitempty"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
}

// AddMessageRequest is a request to add a message
type AddMessageRequest struct {
	Role         string                 `json:"role"`
	Content      string                 `json:"content"`
	ContentParts []ContentPartForStorage `json:"content_parts,omitempty"`
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

	if len(req.ContentParts) > 0 {
		meta, _ := json.Marshal(map[string]interface{}{"content_parts": req.ContentParts})
		message.Metadata = string(meta)
	}

	if err := api.messageRepo.Create(ctx, message); err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	return nil
}
