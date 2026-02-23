package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Parser parses SKILL.md files
type Parser struct {
	skillsPath string
}

// NewParser creates a new skill parser
func NewParser(skillsPath string) *Parser {
	return &Parser{
		skillsPath: skillsPath,
	}
}

// Parse parses a SKILL.md file
func (p *Parser) Parse(skillPath string) (*ParsedSkill, error) {
	skillMDPath := filepath.Join(skillPath, "SKILL.md")

	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	metadata, body, err := p.parseFrontMatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse front matter: %w", err)
	}

	// First, parse into raw map to extract references
	var rawMeta map[string]interface{}
	if err := yaml.Unmarshal([]byte(metadata), &rawMeta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Extract references before parsing into SkillMetadata
	var references []string
	if refs, ok := rawMeta["parameters"].([]interface{}); ok {
		// Try to extract references from parameters
		for _, ref := range refs {
			if refMap, ok := ref.(map[string]interface{}); ok {
				if r, ok := refMap["references"].([]interface{}); ok {
					for _, r := range r {
						if rs, ok := r.(string); ok {
							references = append(references, rs)
						}
					}
				}
			}
		}
	} else if params, ok := rawMeta["parameters"].(map[string]interface{}); ok {
		if refs, ok := params["references"].([]interface{}); ok {
			for _, ref := range refs {
				if rs, ok := ref.(string); ok {
					references = append(references, rs)
				}
			}
		}
	}

	// Parse metadata
	var meta SkillMetadata
	if err := yaml.Unmarshal([]byte(metadata), &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Set ID from name if not provided
	if meta.ID == "" {
		meta.ID = strings.ToLower(strings.ReplaceAll(meta.Name, " ", "-"))
	}

	// Parse references
	for _, ref := range references {
		refPath := filepath.Join(skillPath, ref)
		refContent, err := os.ReadFile(refPath)
		if err != nil {
			log.Warn().Err(err).Str("path", refPath).Msg("failed to read reference file")
			continue
		}
		body += "\n\n" + string(refContent)
	}

	return &ParsedSkill{
		Metadata: meta,
		Content:  body,
		Path:     skillPath,
	}, nil
}

// parseFrontMatter parses YAML front matter from markdown
func (p *Parser) parseFrontMatter(content string) (string, string, error) {
	// Check for front matter delimiter
	if !strings.HasPrefix(content, "---") {
		return "", content, nil
	}

	// Find the end delimiter
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid front matter format")
	}

	metadata := strings.TrimSpace(parts[1])
	body := strings.TrimSpace(parts[2])

	return metadata, body, nil
}

// Discover scans the skills directory and returns all skills
func (p *Parser) Discover() ([]*ParsedSkill, error) {
	var skills []*ParsedSkill

	err := filepath.Walk(p.skillsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this is a SKILL.md file
		if info.Name() == "SKILL.md" {
			skillDir := filepath.Dir(path)
			skill, err := p.Parse(skillDir)
			if err != nil {
				log.Warn().Err(err).Str("path", skillDir).Msg("failed to parse skill")
				return nil
			}
			skills = append(skills, skill)
			log.Info().
				Str("id", skill.Metadata.ID).
				Str("name", skill.Metadata.Name).
				Msg("discovered skill")
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk skills directory: %w", err)
	}

	return skills, nil
}

// ParsedSkill represents a parsed skill with metadata and content
type ParsedSkill struct {
	Metadata SkillMetadata
	Content  string
	Path     string
}

// ToSkill converts a ParsedSkill to a Skill instance
func (ps *ParsedSkill) ToSkill() Skill {
	return &SimpleSkill{
		BaseSkill: NewBaseSkill(ps.Metadata),
		executeFunc: func(ctx *ExecutionContext, input string) (string, error) {
			// Default implementation: return the skill description
			return fmt.Sprintf("Skill: %s\n\n%s", ps.Metadata.Name, ps.Metadata.Description), nil
		},
	}
}

// Loader loads skills into the registry
type Loader struct {
	parser   *Parser
	registry Registry
}

// NewLoader creates a new skill loader
func NewLoader(skillsPath string, registry Registry) *Loader {
	return &Loader{
		parser:   NewParser(skillsPath),
		registry: registry,
	}
}

// LoadAll loads all skills from the skills directory
func (l *Loader) LoadAll() ([]Skill, error) {
	parsed, err := l.parser.Discover()
	if err != nil {
		return nil, err
	}

	var skills []Skill
	for _, ps := range parsed {
		skill := ps.ToSkill()
		if err := l.registry.Register(skill); err != nil {
			log.Warn().Err(err).
				Str("id", ps.Metadata.ID).
				Msg("failed to register skill")
			continue
		}
		skills = append(skills, skill)
	}

	log.Info().Int("count", len(skills)).Msg("loaded skills")
	return skills, nil
}

// Load loads a specific skill by ID
func (l *Loader) Load(skillID string) (Skill, error) {
	parsed, err := l.parser.Discover()
	if err != nil {
		return nil, err
	}

	for _, ps := range parsed {
		if ps.Metadata.ID == skillID {
			skill := ps.ToSkill()
			if err := l.registry.Register(skill); err != nil {
				return nil, err
			}
			return skill, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", skillID)
}

// Reload reloads all skills
func (l *Loader) Reload() ([]Skill, error) {
	return l.LoadAll()
}

// ParseSkillContent parses a skill from markdown content string
func ParseSkillContent(content []byte) (*SkillMetadata, error) {
	metadata, _, err := parseFrontMatterFromContent(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse front matter: %w", err)
	}

	// Parse metadata
	var meta SkillMetadata
	if err := yaml.Unmarshal([]byte(metadata), &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Set ID from name if not provided
	if meta.ID == "" {
		meta.ID = strings.ToLower(strings.ReplaceAll(meta.Name, " ", "-"))
	}

	return &meta, nil
}

// parseFrontMatterFromContent parses YAML front matter from markdown content
func parseFrontMatterFromContent(content string) (string, string, error) {
	// Check for front matter delimiter
	if !strings.HasPrefix(content, "---") {
		return "", content, nil
	}

	// Find the end delimiter
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid front matter format")
	}

	metadata := strings.TrimSpace(parts[1])
	body := strings.TrimSpace(parts[2])

	return metadata, body, nil
}
