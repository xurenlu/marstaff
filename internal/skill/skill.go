package skill

import (
	"context"
	"fmt"
)

// SkillMetadata contains metadata about a skill
type SkillMetadata struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Author      string                 `json:"author"`
	Category    string                 `json:"category"`
	Tags        []string               `json:"tags"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Tool represents a tool/function that can be called
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Handler     ToolHandler            `json:"-"`
}

// ToolHandler is a function that handles a tool call
type ToolHandler func(ctx context.Context, params map[string]interface{}) (string, error)

// ExecutionContext provides context for skill execution
type ExecutionContext struct {
	SessionID string            `json:"session_id"`
	UserID    string            `json:"user_id"`
	Variables map[string]string `json:"variables"`
	Context   context.Context   `json:"-"`
}

// Skill represents a skill/capability
type Skill interface {
	// Metadata returns the skill's metadata
	Metadata() SkillMetadata

	// Execute runs the skill with the given input
	Execute(ctx *ExecutionContext, input string) (string, error)

	// Tools returns the tools provided by this skill
	Tools() []Tool

	// Validate validates the parameters
	Validate(params map[string]interface{}) error

	// IsEnabled returns whether the skill is enabled
	IsEnabled() bool

	// SetEnabled sets the enabled state
	SetEnabled(enabled bool)
}

// BaseSkill provides common functionality for skills
type BaseSkill struct {
	metadata SkillMetadata
	tools    []Tool
	enabled  bool
}

// NewBaseSkill creates a new base skill
func NewBaseSkill(metadata SkillMetadata) *BaseSkill {
	return &BaseSkill{
		metadata: metadata,
		enabled:  true,
	}
}

// Metadata returns the skill's metadata
func (s *BaseSkill) Metadata() SkillMetadata {
	return s.metadata
}

// Tools returns the tools provided by this skill
func (s *BaseSkill) Tools() []Tool {
	return s.tools
}

// AddTool adds a tool to the skill
func (s *BaseSkill) AddTool(tool Tool) {
	s.tools = append(s.tools, tool)
}

// IsEnabled returns whether the skill is enabled
func (s *BaseSkill) IsEnabled() bool {
	return s.enabled
}

// SetEnabled sets the enabled state
func (s *BaseSkill) SetEnabled(enabled bool) {
	s.enabled = enabled
}

// Validate validates the parameters (default implementation accepts all)
func (s *BaseSkill) Validate(params map[string]interface{}) error {
	return nil
}

// Execute executes the skill (default implementation returns error)
func (s *BaseSkill) Execute(ctx *ExecutionContext, input string) (string, error) {
	return "", fmt.Errorf("skill execution not implemented")
}

// SimpleSkill is a skill with a simple execute function
type SimpleSkill struct {
	*BaseSkill
	executeFunc func(ctx *ExecutionContext, input string) (string, error)
}

// NewSimpleSkill creates a new simple skill
func NewSimpleSkill(metadata SkillMetadata, executeFunc func(ctx *ExecutionContext, input string) (string, error)) *SimpleSkill {
	return &SimpleSkill{
		BaseSkill:   NewBaseSkill(metadata),
		executeFunc: executeFunc,
	}
}

// Execute runs the skill
func (s *SimpleSkill) Execute(ctx *ExecutionContext, input string) (string, error) {
	return s.executeFunc(ctx, input)
}
