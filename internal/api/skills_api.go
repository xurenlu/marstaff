package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/skill"
)

// SkillsAPI handles skills, rules, and MCP management
type SkillsAPI struct {
	skillRepo     *repository.SkillRepository
	ruleRepo      *repository.RuleRepository
	mcpRepo       *repository.MCPServerRepository
	skillRegistry skill.Registry
	skillLoader   *skill.Loader
	skillsDir     string
}

// NewSkillsAPI creates a new skills API
func NewSkillsAPI(db *gorm.DB, skillsDir string, skillRegistry skill.Registry) *SkillsAPI {
	return &SkillsAPI{
		skillRepo:     repository.NewSkillRepository(db),
		ruleRepo:      repository.NewRuleRepository(db),
		mcpRepo:       repository.NewMCPServerRepository(db),
		skillRegistry: skillRegistry,
		skillLoader:   skill.NewLoader(skillsDir, skillRegistry),
		skillsDir:     skillsDir,
	}
}

// RegisterRoutes registers all routes
func (api *SkillsAPI) RegisterRoutes(router *gin.RouterGroup) {
	// Skills routes
	router.GET("/skills", api.ListSkills)
	router.GET("/skills/:id", api.GetSkill)
	router.PUT("/skills/:id/enable", api.EnableSkill)
	router.PUT("/skills/:id/disable", api.DisableSkill)
	router.POST("/skills/install", api.InstallSkill)
	router.POST("/skills/uninstall/:id", api.UninstallSkill)

	// Rules routes
	router.GET("/rules", api.ListRules)
	router.GET("/rules/:id", api.GetRule)
	router.POST("/rules", api.CreateRule)
	router.PUT("/rules/:id", api.UpdateRule)
	router.DELETE("/rules/:id", api.DeleteRule)
	router.PUT("/rules/:id/activate", api.ActivateRule)
	router.PUT("/rules/:id/deactivate", api.DeactivateRule)

	// MCP routes
	router.GET("/mcp/servers", api.ListMCPServers)
	router.GET("/mcp/servers/:id", api.GetMCPServer)
	router.POST("/mcp/servers", api.CreateMCPServer)
	router.PUT("/mcp/servers/:id", api.UpdateMCPServer)
	router.DELETE("/mcp/servers/:id", api.DeleteMCPServer)
	router.PUT("/mcp/servers/:id/enable", api.EnableMCPServer)
	router.PUT("/mcp/servers/:id/disable", api.DisableMCPServer)
	router.POST("/mcp/servers/:id/sync", api.SyncMCPServer)
}

// ============== Skills ==============

// ListSkillsResponse is the response for listing skills
type ListSkillsResponse struct {
	Skills []*model.Skill `json:"skills"`
}

// ListSkills returns all skills
func (api *SkillsAPI) ListSkills(c *gin.Context) {
	ctx := c.Request.Context()
	skills, err := api.skillRepo.List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListSkillsResponse{Skills: skills})
}

// GetSkill returns a single skill
func (api *SkillsAPI) GetSkill(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	skill, err := api.skillRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}
	c.JSON(http.StatusOK, skill)
}

// EnableSkillRequest is the request to enable a skill
type EnableSkillRequest struct {
	Enabled bool `json:"enabled"`
}

// EnableSkill enables a skill
func (api *SkillsAPI) EnableSkill(c *gin.Context) {
	id := c.Param("id")
	var req EnableSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	if err := api.skillRepo.SetEnabled(ctx, id, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Reload skills
	// TODO: Implement skill reload via Loader
	// api.skillRegistry.LoadAll()
	c.JSON(http.StatusOK, gin.H{"status": "enabled"})
}

// DisableSkill disables a skill
func (api *SkillsAPI) DisableSkill(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if err := api.skillRepo.SetEnabled(ctx, id, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Reload skills
	// TODO: Implement skill reload via Loader
	// api.skillRegistry.LoadAll()
	c.JSON(http.StatusOK, gin.H{"status": "disabled"})
}

// InstallSkillRequest is the request to install a skill
type InstallSkillRequest struct {
	URL      string `json:"url"`       // URL to install from
	Content  string `json:"content"`   // Direct SKILL.md content
	Name     string `json:"name"`      // Name for the skill
	Overwrite bool   `json:"overwrite"` // Overwrite if exists
}

// InstallSkill installs a new skill from URL or content
func (api *SkillsAPI) InstallSkill(c *gin.Context) {
	var req InstallSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var content string

	if req.URL != "" {
		// Download from URL
		// For security, only allow GitHub URLs or localhost
		if !strings.HasPrefix(req.URL, "https://github.com/") && !strings.HasPrefix(req.URL, "http://localhost") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only GitHub URLs are supported"})
			return
		}
		// TODO: Implement URL fetching
		c.JSON(http.StatusNotImplemented, gin.H{"error": "URL installation not yet implemented"})
		return
	} else if req.Content != "" {
		content = req.Content
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either URL or content is required"})
		return
	}

	// Parse the skill
	skillData, err := skill.ParseSkillContent([]byte(content))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse skill: " + err.Error()})
		return
	}

	// Check if skill already exists
	existing, _ := api.skillRepo.GetByID(ctx, skillData.ID)
	if existing != nil && !req.Overwrite {
		c.JSON(http.StatusConflict, gin.H{"error": "Skill already exists", "id": existing.ID})
		return
	}

	// Convert to model.Skill
	newSkill := &model.Skill{
		ID:          skillData.ID,
		Name:        skillData.Name,
		Description: skillData.Description,
		Category:    skillData.Category,
		Version:     skillData.Version,
		Enabled:     true,
	}

	if err := api.skillRepo.Create(ctx, newSkill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reload skills via loader
	if api.skillLoader != nil {
		if _, err := api.skillLoader.Reload(); err != nil {
			log.Warn().Err(err).Msg("failed to reload skills after installation")
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "installed", "id": skillData.ID})
}

// UninstallSkill removes a skill
func (api *SkillsAPI) UninstallSkill(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	// Check if builtin
	skill, err := api.skillRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}
	if skill.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	if err := api.skillRepo.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reload skills
	// TODO: Implement skill reload via Loader
	// api.skillRegistry.LoadAll()

	c.JSON(http.StatusOK, gin.H{"status": "uninstalled"})
}

// ============== Rules ==============

// ListRulesResponse is the response for listing rules
type ListRulesResponse struct {
	Rules []*model.Rule `json:"rules"`
}

// ListRules returns all rules
func (api *SkillsAPI) ListRules(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}
	ctx := c.Request.Context()
	rules, err := api.ruleRepo.List(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListRulesResponse{Rules: rules})
}

// GetRule returns a single rule
func (api *SkillsAPI) GetRule(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	rule, err := api.ruleRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// CreateRuleRequest is the request to create a rule
type CreateRuleRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Content     string `json:"content" binding:"required"`
	Category    string `json:"category"`
	Tags        string `json:"tags"`
}

// CreateRule creates a new rule
func (api *SkillsAPI) CreateRule(c *gin.Context) {
	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}

	rule := &model.Rule{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Category:    req.Category,
		Tags:        req.Tags,
		UserID:      userID,
		Enabled:     true,
		IsActive:    false,
		IsBuiltin:   false,
	}

	ctx := c.Request.Context()
	if err := api.ruleRepo.Create(ctx, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// UpdateRuleRequest is the request to update a rule
type UpdateRuleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Category    string `json:"category"`
	Tags        string `json:"tags"`
	Enabled     *bool  `json:"enabled"`
}

// UpdateRule updates a rule
func (api *SkillsAPI) UpdateRule(c *gin.Context) {
	id := c.Param("id")
	var req UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	rule, err := api.ruleRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	if rule.IsBuiltin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify builtin rule"})
		return
	}

	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Description != "" {
		rule.Description = req.Description
	}
	if req.Content != "" {
		rule.Content = req.Content
	}
	if req.Category != "" {
		rule.Category = req.Category
	}
	if req.Tags != "" {
		rule.Tags = req.Tags
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}

	if err := api.ruleRepo.Update(ctx, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DeleteRule deletes a rule
func (api *SkillsAPI) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	rule, err := api.ruleRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	if rule.IsBuiltin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete builtin rule"})
		return
	}

	if err := api.ruleRepo.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ActivateRule sets a rule as active
func (api *SkillsAPI) ActivateRule(c *gin.Context) {
	id := c.Param("id")
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}

	ctx := c.Request.Context()
	if err := api.ruleRepo.SetActive(ctx, userID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "activated"})
}

// DeactivateRule deactivates a rule
func (api *SkillsAPI) DeactivateRule(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	rule, err := api.ruleRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	rule.IsActive = false
	if err := api.ruleRepo.Update(ctx, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deactivated"})
}

// ============== MCP Servers ==============

// ListMCPServersResponse is the response for listing MCP servers
type ListMCPServersResponse struct {
	Servers []*model.MCPServer `json:"servers"`
}

// ListMCPServers returns all MCP servers
func (api *SkillsAPI) ListMCPServers(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}
	ctx := c.Request.Context()
	servers, err := api.mcpRepo.List(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListMCPServersResponse{Servers: servers})
}

// GetMCPServer returns a single MCP server
func (api *SkillsAPI) GetMCPServer(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	server, err := api.mcpRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}
	c.JSON(http.StatusOK, server)
}

// CreateMCPServerRequest is the request to create an MCP server
type CreateMCPServerRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Endpoint    string                 `json:"endpoint" binding:"required"`
	Config      map[string]interface{} `json:"config"`
}

// CreateMCPServer creates a new MCP server
func (api *SkillsAPI) CreateMCPServer(c *gin.Context) {
	var req CreateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}

	configJSON, _ := json.Marshal(req.Config)
	server := &model.MCPServer{
		Name:        req.Name,
		Description: req.Description,
		Endpoint:    req.Endpoint,
		Config:      string(configJSON),
		UserID:      userID,
		Enabled:     true,
	}

	ctx := c.Request.Context()
	if err := api.mcpRepo.Create(ctx, server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Try to sync tools from the server
	api.syncMCPTools(ctx, server)

	c.JSON(http.StatusOK, server)
}

// UpdateMCPServerRequest is the request to update an MCP server
type UpdateMCPServerRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Endpoint    string                 `json:"endpoint"`
	Config      map[string]interface{} `json:"config"`
	Enabled     *bool                  `json:"enabled"`
}

// UpdateMCPServer updates an MCP server
func (api *SkillsAPI) UpdateMCPServer(c *gin.Context) {
	id := c.Param("id")
	var req UpdateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	server, err := api.mcpRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	if req.Name != "" {
		server.Name = req.Name
	}
	if req.Description != "" {
		server.Description = req.Description
	}
	if req.Endpoint != "" {
		server.Endpoint = req.Endpoint
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		server.Config = string(configJSON)
	}
	if req.Enabled != nil {
		server.Enabled = *req.Enabled
	}

	if err := api.mcpRepo.Update(ctx, server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Re-sync tools if enabled
	if server.Enabled {
		api.syncMCPTools(ctx, server)
	}

	c.JSON(http.StatusOK, server)
}

// DeleteMCPServer deletes an MCP server
func (api *SkillsAPI) DeleteMCPServer(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if err := api.mcpRepo.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// EnableMCPServer enables an MCP server
func (api *SkillsAPI) EnableMCPServer(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if err := api.mcpRepo.SetEnabled(ctx, id, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "enabled"})
}

// DisableMCPServer disables an MCP server
func (api *SkillsAPI) DisableMCPServer(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if err := api.mcpRepo.SetEnabled(ctx, id, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "disabled"})
}

// SyncMCPServer syncs tools from an MCP server
func (api *SkillsAPI) SyncMCPServer(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	server, err := api.mcpRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	if err := api.syncMCPTools(ctx, server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "synced"})
}

// syncMCPTools syncs tools from an MCP server
func (api *SkillsAPI) syncMCPTools(ctx context.Context, server *model.MCPServer) error {
	// TODO: Implement actual MCP protocol to fetch tools
	// For now, return success
	log.Info().Str("server", server.ID).Msg("MCP sync not yet implemented")
	return nil
}

// ============== File System Skills ==============

// ListLocalSkills lists skills from the local filesystem
func (api *SkillsAPI) ListLocalSkills(c *gin.Context) {
	skillsDir := api.skillsDir
	if skillsDir == "" {
		skillsDir = "./skills"
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type LocalSkill struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Version     string `json:"version"`
		Installed   bool   `json:"installed"`
	}

	var localSkills []LocalSkill
	ctx := c.Request.Context()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMDPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillMDPath)
		if err != nil {
			continue
		}

		skillData, err := skill.ParseSkillContent(content)
		if err != nil {
			continue
		}

		// Check if installed
		existing, _ := api.skillRepo.GetByID(ctx, skillData.ID)
		installed := existing != nil

		localSkills = append(localSkills, LocalSkill{
			ID:          skillData.ID,
			Name:        skillData.Name,
			Description: skillData.Description,
			Category:    skillData.Category,
			Version:     skillData.Version,
			Installed:   installed,
		})
	}

	c.JSON(http.StatusOK, gin.H{"skills": localSkills})
}
