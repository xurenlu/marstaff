package skill

import (
	"context"
	"fmt"
)

// CompositeRegistry combines builtin and remote registries
type CompositeRegistry struct {
	builtin SkillRegistryClient
	remote  SkillRegistryClient
}

// NewCompositeRegistry creates a registry that searches both builtin and remote
func NewCompositeRegistry(builtin, remote SkillRegistryClient) *CompositeRegistry {
	return &CompositeRegistry{builtin: builtin, remote: remote}
}

// Search searches both registries and merges results (deduplicated by ID)
func (r *CompositeRegistry) Search(ctx context.Context, query string) ([]SkillMeta, error) {
	var all []SkillMeta
	seen := make(map[string]bool)

	if r.builtin != nil {
		skills, err := r.builtin.Search(ctx, query)
		if err == nil {
			for _, s := range skills {
				if !seen[s.ID] {
					seen[s.ID] = true
					all = append(all, s)
				}
			}
		}
	}

	if r.remote != nil {
		skills, err := r.remote.Search(ctx, query)
		if err == nil {
			for _, s := range skills {
				if !seen[s.ID] {
					seen[s.ID] = true
					all = append(all, s)
				}
			}
		}
	}

	return all, nil
}

// GetByID looks up in builtin first, then remote
func (r *CompositeRegistry) GetByID(ctx context.Context, id string) (*SkillMeta, error) {
	if r.builtin != nil {
		if s, err := r.builtin.GetByID(ctx, id); err == nil && s != nil {
			return s, nil
		}
	}
	if r.remote != nil {
		return r.remote.GetByID(ctx, id)
	}
	return nil, fmt.Errorf("skill %s not found in registry", id)
}
