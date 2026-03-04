package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// SkillRepository handles skill data operations
type SkillRepository struct {
	db *gorm.DB
}

// NewSkillRepository creates a new skill repository
func NewSkillRepository(db *gorm.DB) *SkillRepository {
	return &SkillRepository{db: db}
}

// List retrieves all skills
func (r *SkillRepository) List(ctx context.Context) ([]*model.Skill, error) {
	var skills []*model.Skill
	err := r.db.WithContext(ctx).Find(&skills).Error
	return skills, err
}

// GetByID retrieves a skill by ID
func (r *SkillRepository) GetByID(ctx context.Context, id string) (*model.Skill, error) {
	var skill model.Skill
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

// GetEnabled retrieves all enabled skills
func (r *SkillRepository) GetEnabled(ctx context.Context) ([]*model.Skill, error) {
	var skills []*model.Skill
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&skills).Error
	return skills, err
}

// Create creates a new skill
func (r *SkillRepository) Create(ctx context.Context, skill *model.Skill) error {
	return r.db.WithContext(ctx).Create(skill).Error
}

// Update updates a skill
func (r *SkillRepository) Update(ctx context.Context, skill *model.Skill) error {
	return r.db.WithContext(ctx).Save(skill).Error
}

// Delete soft deletes a skill
func (r *SkillRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Skill{}, "id = ?", id).Error
}

// SetEnabled sets the enabled status of a skill
func (r *SkillRepository) SetEnabled(ctx context.Context, id string, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.Skill{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// RuleRepository handles rule data operations
type RuleRepository struct {
	db *gorm.DB
}

// NewRuleRepository creates a new rule repository
func NewRuleRepository(db *gorm.DB) *RuleRepository {
	return &RuleRepository{db: db}
}

// List retrieves all rules for a user
func (r *RuleRepository) List(ctx context.Context, userID string) ([]*model.Rule, error) {
	var rules []*model.Rule
	err := r.db.WithContext(ctx).Where("user_id = ? OR user_id = ''", userID).Order("is_active DESC, created_at DESC").Find(&rules).Error
	return rules, err
}

// GetByID retrieves a rule by ID
func (r *RuleRepository) GetByID(ctx context.Context, id string) (*model.Rule, error) {
	var rule model.Rule
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetActive retrieves the active rule for a user
func (r *RuleRepository) GetActive(ctx context.Context, userID string) (*model.Rule, error) {
	var rule model.Rule
	err := r.db.WithContext(ctx).Where("(user_id = ? OR user_id = '') AND is_active = ?", userID, true).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// Create creates a new rule
func (r *RuleRepository) Create(ctx context.Context, rule *model.Rule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// Update updates a rule
func (r *RuleRepository) Update(ctx context.Context, rule *model.Rule) error {
	return r.db.WithContext(ctx).Save(rule).Error
}

// Delete soft deletes a rule
func (r *RuleRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Rule{}, "id = ?", id).Error
}

// SetActive sets a rule as active and deactivates others for the user
func (r *RuleRepository) SetActive(ctx context.Context, userID, ruleID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Deactivate all rules for this user
		if err := tx.Model(&model.Rule{}).Where("user_id = ?", userID).Update("is_active", false).Error; err != nil {
			return err
		}
		// Activate the selected rule
		if err := tx.Model(&model.Rule{}).Where("id = ?", ruleID).Update("is_active", true).Error; err != nil {
			return err
		}
		return nil
	})
}

// MCPServerRepository handles MCP server data operations
type MCPServerRepository struct {
	db *gorm.DB
}

// NewMCPServerRepository creates a new MCP server repository
func NewMCPServerRepository(db *gorm.DB) *MCPServerRepository {
	return &MCPServerRepository{db: db}
}

// List retrieves all MCP servers for a user
func (r *MCPServerRepository) List(ctx context.Context, userID string) ([]*model.MCPServer, error) {
	var servers []*model.MCPServer
	err := r.db.WithContext(ctx).Where("user_id = ? OR user_id = ''", userID).Preload("Tools").Find(&servers).Error
	return servers, err
}

// GetByID retrieves an MCP server by ID
func (r *MCPServerRepository) GetByID(ctx context.Context, id string) (*model.MCPServer, error) {
	var server model.MCPServer
	err := r.db.WithContext(ctx).Preload("Tools").Where("id = ?", id).First(&server).Error
	if err != nil {
		return nil, err
	}
	return &server, nil
}

// GetEnabled retrieves all enabled MCP servers
func (r *MCPServerRepository) GetEnabled(ctx context.Context) ([]*model.MCPServer, error) {
	var servers []*model.MCPServer
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Preload("Tools").Find(&servers).Error
	return servers, err
}

// Create creates a new MCP server
func (r *MCPServerRepository) Create(ctx context.Context, server *model.MCPServer) error {
	return r.db.WithContext(ctx).Create(server).Error
}

// Update updates an MCP server
func (r *MCPServerRepository) Update(ctx context.Context, server *model.MCPServer) error {
	return r.db.WithContext(ctx).Save(server).Error
}

// Delete soft deletes an MCP server
func (r *MCPServerRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.MCPServer{}, "id = ?", id).Error
}

// SetEnabled sets the enabled status of an MCP server
func (r *MCPServerRepository) SetEnabled(ctx context.Context, id string, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// SyncTools syncs tools from an MCP server
func (r *MCPServerRepository) SyncTools(ctx context.Context, serverID string, tools []*model.MCPTool) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing tools for this server
		if err := tx.Where("server_id = ?", serverID).Delete(&model.MCPTool{}).Error; err != nil {
			return err
		}
		// Insert new tools
		for _, tool := range tools {
			tool.ServerID = serverID
			if err := tx.Create(tool).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
