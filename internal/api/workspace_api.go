package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/tools/security"
)

// WorkspaceAPI handles workspace creation for programming mode
type WorkspaceAPI struct {
	basePath string
}

// NewWorkspaceAPI creates a new workspace API
func NewWorkspaceAPI(basePath string) *WorkspaceAPI {
	return &WorkspaceAPI{basePath: basePath}
}

// CreateWorkspaceRequest is the request body for creating a workspace
type CreateWorkspaceRequest struct {
	Name string `json:"name" binding:"required"`
}

// CreateWorkspaceResponse is the response for workspace creation
type CreateWorkspaceResponse struct {
	Path string `json:"path"`
}

// CreateWorkspace creates a new project directory under the configured base path
func (api *WorkspaceAPI) CreateWorkspace(c *gin.Context) {
	var req CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "name is required"})
		return
	}

	// Sanitize name: only alphanumeric, dash, underscore
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(req.Name) {
		c.JSON(400, gin.H{"error": "name must contain only letters, numbers, dashes and underscores"})
		return
	}

	absBase, err := filepath.Abs(api.basePath)
	if err != nil {
		log.Error().Err(err).Str("base_path", api.basePath).Msg("failed to resolve base path")
		c.JSON(500, gin.H{"error": "invalid workspace configuration"})
		return
	}

	// Ensure base path exists
	if err := os.MkdirAll(absBase, 0755); err != nil {
		log.Error().Err(err).Str("base_path", absBase).Msg("failed to create base path")
		c.JSON(500, gin.H{"error": "failed to create workspace directory"})
		return
	}

	// Validate base path is in allowed working directories
	secCfg := security.GetConfig()
	if secCfg != nil {
		inAllowed := false
		for _, wd := range secCfg.WorkingDirectories {
			absWd, _ := filepath.Abs(wd)
			rel, err := filepath.Rel(absWd, absBase)
			if err == nil && !strings.HasPrefix(rel, "..") {
				inAllowed = true
				break
			}
		}
		if !inAllowed {
			log.Warn().Str("base_path", absBase).Msg("workspace base path not in allowed directories")
			c.JSON(403, gin.H{"error": "workspace base path is not in allowed directories"})
			return
		}
	}

	projectPath := filepath.Join(absBase, req.Name)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		log.Error().Err(err).Str("path", projectPath).Msg("failed to create project directory")
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to create project: %v", err)})
		return
	}

	log.Info().Str("path", projectPath).Str("name", req.Name).Msg("workspace created")
	c.JSON(201, CreateWorkspaceResponse{Path: projectPath})
}
