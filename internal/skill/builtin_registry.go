package skill

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed builtin_registry.json
var builtinRegistryFS embed.FS

// BuiltinRegistry implements SkillRegistryClient using embedded JSON
type BuiltinRegistry struct {
	skills []SkillMeta
}

// NewBuiltinRegistry creates a registry from embedded builtin_registry.json
func NewBuiltinRegistry() (*BuiltinRegistry, error) {
	data, err := builtinRegistryFS.ReadFile("builtin_registry.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read builtin registry: %w", err)
	}
	var result struct {
		Skills []SkillMeta `json:"skills"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse builtin registry: %w", err)
	}
	return &BuiltinRegistry{skills: result.Skills}, nil
}

// Search searches builtin skills by keyword
func (r *BuiltinRegistry) Search(ctx context.Context, query string) ([]SkillMeta, error) {
	query = strings.ToLower(query)
	var filtered []SkillMeta
	for _, s := range r.skills {
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Description), query) ||
			strings.Contains(strings.ToLower(s.ID), query) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// GetByID retrieves a skill by ID from the builtin registry
func (r *BuiltinRegistry) GetByID(ctx context.Context, id string) (*SkillMeta, error) {
	for _, s := range r.skills {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("skill %s not found in registry", id)
}
