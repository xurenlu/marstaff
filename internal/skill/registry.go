package skill

import (
	"fmt"
	"strings"
	"sync"
)

// Registry manages skill registration and lookup
type Registry interface {
	// Register registers a skill
	Register(skill Skill) error

	// Unregister unregisters a skill
	Unregister(id string) error

	// Get retrieves a skill by ID
	Get(id string) (Skill, error)

	// List returns all registered skills
	List() []Skill

	// ListEnabled returns all enabled skills
	ListEnabled() []Skill

	// FindByCategory returns skills in a category
	FindByCategory(category string) []Skill

	// Search searches for skills by name or description
	Search(query string) []Skill

	// GetTools returns all tools from enabled skills
	GetTools() []Tool

	// GetTool returns a tool by name
	GetTool(name string) (Tool, error)
}

// registry implements Registry
type registry struct {
	skills map[string]Skill
	mu     sync.RWMutex
}

// NewRegistry creates a new skill registry
func NewRegistry() Registry {
	return &registry{
		skills: make(map[string]Skill),
	}
}

func (r *registry) Register(skill Skill) error {
	if skill == nil {
		return fmt.Errorf("skill cannot be nil")
	}

	metadata := skill.Metadata()
	if metadata.ID == "" {
		return fmt.Errorf("skill ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	if _, exists := r.skills[metadata.ID]; exists {
		return fmt.Errorf("skill with ID %s already registered", metadata.ID)
	}

	r.skills[metadata.ID] = skill
	return nil
}

func (r *registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[id]; !exists {
		return fmt.Errorf("skill with ID %s not found", id)
	}

	delete(r.skills, id)
	return nil
}

func (r *registry) Get(id string) (Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[id]
	if !exists {
		return nil, fmt.Errorf("skill with ID %s not found", id)
	}

	return skill, nil
}

func (r *registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}

	return skills
}

func (r *registry) ListEnabled() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []Skill
	for _, skill := range r.skills {
		if skill.IsEnabled() {
			skills = append(skills, skill)
		}
	}

	return skills
}

func (r *registry) FindByCategory(category string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []Skill
	for _, skill := range r.skills {
		if skill.IsEnabled() && skill.Metadata().Category == category {
			skills = append(skills, skill)
		}
	}

	return skills
}

func (r *registry) Search(query string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(query)
	var skills []Skill

	for _, skill := range r.skills {
		if !skill.IsEnabled() {
			continue
		}

		metadata := skill.Metadata()
		if strings.Contains(strings.ToLower(metadata.Name), query) ||
			strings.Contains(strings.ToLower(metadata.Description), query) {
			skills = append(skills, skill)
			continue
		}

		// Check tags
		for _, tag := range metadata.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				skills = append(skills, skill)
				break
			}
		}
	}

	return skills
}

func (r *registry) GetTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, skill := range r.skills {
		if skill.IsEnabled() {
			tools = append(tools, skill.Tools()...)
		}
	}

	return tools
}

func (r *registry) GetTool(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, skill := range r.skills {
		if !skill.IsEnabled() {
			continue
		}

		for _, tool := range skill.Tools() {
			if tool.Name == name {
				return tool, nil
			}
		}
	}

	return Tool{}, fmt.Errorf("tool with name %s not found", name)
}
