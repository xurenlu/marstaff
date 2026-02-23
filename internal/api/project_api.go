package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/tools/security"
)

// ProjectAPI handles project management
type ProjectAPI struct {
	userRepo    *repository.UserRepository
	projectRepo *repository.ProjectRepository
	sessionRepo *repository.SessionRepository
}

// NewProjectAPI creates a new project API
func NewProjectAPI(db *gorm.DB) *ProjectAPI {
	return &ProjectAPI{
		userRepo:    repository.NewUserRepository(db),
		projectRepo: repository.NewProjectRepository(db),
		sessionRepo: repository.NewSessionRepository(db),
	}
}

// CreateProject creates a new project
func (api *ProjectAPI) CreateProject(c *gin.Context) {
	var req model.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Get or create user (for single-user mode with "default" user)
	user, err := api.userRepo.GetOrCreateByPlatformID(ctx, "web", req.UserID, req.UserID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get or create user")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Validate project name (alphanumeric, underscore, hyphen, dot)
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-. ]+$`, req.Name)
	if !matched {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project name can only contain letters, numbers, spaces, underscores, hyphens, and dots"})
		return
	}

	// Check for duplicate project name
	existing, _ := api.projectRepo.GetByName(ctx, req.UserID, req.Name)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "project with this name already exists"})
		return
	}

	// Validate work directory
	if err := security.ValidateWorkDir(req.WorkDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prepare tech stack - use empty JSON array instead of empty string for MySQL compatibility
	var techStackJSON string
	if len(req.TechStack) > 0 {
		data, err := json.Marshal(req.TechStack)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tech_stack format"})
			return
		}
		techStackJSON = string(data)
	} else {
		// Use empty JSON array for empty tech_stack to avoid MySQL JSON validation error
		techStackJSON = "[]"
	}

	// Use empty JSON object for metadata to avoid MySQL JSON validation error
	project := &model.Project{
		UserID:      user.ID,
		Name:        req.Name,
		Description: req.Description,
		WorkDir:     req.WorkDir,
		Template:    req.Template,
		TechStack:   techStackJSON,
		Metadata:    "{}", // Empty JSON object for MySQL compatibility
	}

	if err := api.projectRepo.Create(ctx, project); err != nil {
		log.Error().Err(err).Msg("failed to create project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	c.JSON(http.StatusCreated, project)
}

// GetProject retrieves a project by ID
func (api *ProjectAPI) GetProject(c *gin.Context) {
	projectID := c.Param("id")
	ctx := c.Request.Context()

	project, err := api.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Parse tech stack
	var techStack []string
	if project.TechStack != "" {
		json.Unmarshal([]byte(project.TechStack), &techStack)
	}

	response := gin.H{
		"id":          project.ID,
		"user_id":     project.UserID,
		"name":        project.Name,
		"description": project.Description,
		"work_dir":    project.WorkDir,
		"template":    project.Template,
		"tech_stack":  techStack,
		"created_at":  project.CreatedAt.Format(time.RFC3339),
		"updated_at":  project.UpdatedAt.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// ListProjects lists projects for a user
func (api *ProjectAPI) ListProjects(c *gin.Context) {
	userID := c.Query("user_id")
	template := c.Query("template")
	search := c.Query("search")
	limit := 50
	offset := 0

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx := c.Request.Context()

	// Get or create user (for single-user mode with "default" user)
	user, err := api.userRepo.GetOrCreateByPlatformID(ctx, "web", userID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	opts := &model.ProjectListOptions{
		UserID:  user.ID,
		Template: template,
		Search:  search,
		Limit:   limit,
		Offset:  offset,
	}

	projects, err := api.projectRepo.List(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("failed to list projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	// Parse tech stack for each project
	type projectResponse struct {
		ID          string   `json:"id"`
		UserID      string   `json:"user_id"`
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		WorkDir     string   `json:"work_dir"`
		Template    string   `json:"template,omitempty"`
		TechStack   []string `json:"tech_stack,omitempty"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
	}

	response := make([]projectResponse, len(projects))
	for i, p := range projects {
		var techStack []string
		if p.TechStack != "" {
			json.Unmarshal([]byte(p.TechStack), &techStack)
		}
		response[i] = projectResponse{
			ID:          p.ID,
			UserID:      p.UserID,
			Name:        p.Name,
			Description: p.Description,
			WorkDir:     p.WorkDir,
			Template:    p.Template,
			TechStack:   techStack,
			CreatedAt:   p.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, gin.H{"projects": response})
}

// UpdateProject updates a project
func (api *ProjectAPI) UpdateProject(c *gin.Context) {
	projectID := c.Param("id")
	ctx := c.Request.Context()

	var req model.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	project, err := api.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Update fields
	if req.Name != "" {
		// Validate new name
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-. ]+$`, req.Name)
		if !matched {
			c.JSON(http.StatusBadRequest, gin.H{"error": "project name can only contain letters, numbers, spaces, underscores, hyphens, and dots"})
			return
		}

		// Check for duplicate name (if name changed)
		if req.Name != project.Name {
			existing, _ := api.projectRepo.GetByName(ctx, project.UserID, req.Name)
			if existing != nil && existing.ID != projectID {
				c.JSON(http.StatusConflict, gin.H{"error": "project with this name already exists"})
				return
			}
		}
		project.Name = req.Name
	}

	if req.Description != "" {
		project.Description = req.Description
	}

	if len(req.TechStack) > 0 {
		data, err := json.Marshal(req.TechStack)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tech_stack format"})
			return
		}
		project.TechStack = string(data)
	}

	if err := api.projectRepo.Update(ctx, project); err != nil {
		log.Error().Err(err).Msg("failed to update project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}

	// Parse tech stack for response
	var techStack []string
	if project.TechStack != "" {
		json.Unmarshal([]byte(project.TechStack), &techStack)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          project.ID,
		"user_id":     project.UserID,
		"name":        project.Name,
		"description": project.Description,
		"work_dir":    project.WorkDir,
		"template":    project.Template,
		"tech_stack":  techStack,
		"updated_at":  project.UpdatedAt.Format(time.RFC3339),
	})
}

// DeleteProject deletes a project
func (api *ProjectAPI) DeleteProject(c *gin.Context) {
	projectID := c.Param("id")
	ctx := c.Request.Context()

	// Check if project exists
	project, err := api.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// TODO: Option to also delete associated sessions or set project_id to NULL

	if err := api.projectRepo.Delete(ctx, projectID); err != nil {
		log.Error().Err(err).Msg("failed to delete project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "deleted",
		"deleted_name": project.Name,
	})
}

// ListTemplates lists available project templates
func (api *ProjectAPI) ListTemplates(c *gin.Context) {
	// For now, return static templates
	// In the future, this could load from configs/project_templates.yaml
	templates := []model.ProjectTemplate{
		{
			ID:          "react",
			Name:        "React Project",
			Description: "Modern React application with Vite",
			TechStack:   []string{"react", "typescript", "vite"},
		},
		{
			ID:          "go-api",
			Name:        "Go API Service",
			Description: "RESTful API with Gin framework",
			TechStack:   []string{"go", "gin", "gorm"},
		},
		{
			ID:          "python",
			Name:        "Python Project",
			Description: "Python application with FastAPI",
			TechStack:   []string{"python", "fastapi", "uvicorn"},
		},
		{
			ID:          "nodejs",
			Name:        "Node.js Project",
			Description: "Node.js backend with Express",
			TechStack:   []string{"nodejs", "express", "npm"},
		},
		{
			ID:          "custom",
			Name:        "Custom Project",
			Description: "Empty project without template",
			TechStack:   []string{},
		},
	}

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// GetProjectSessions retrieves sessions for a specific project
func (api *ProjectAPI) GetProjectSessions(c *gin.Context) {
	projectID := c.Param("id")
	limit := 50

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	ctx := c.Request.Context()

	// Verify project exists
	_, err := api.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Get sessions for this project
	var sessions []*model.Session
	err = api.sessionRepo.GetDB().WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("updated_at DESC").
		Limit(limit).
		Find(&sessions).Error

	if err != nil {
		log.Error().Err(err).Msg("failed to get project sessions")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}
