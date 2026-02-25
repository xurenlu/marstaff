package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParser_Discover(t *testing.T) {
	skillsDir := "./skills"
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		// Try from project root
		skillsDir = filepath.Join("..", "..", "skills")
		if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
			t.Skip("skills directory not found, skipping")
		}
	}

	p := NewParser(skillsDir)
	skills, err := p.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(skills) == 0 {
		t.Error("expected at least one skill, got 0")
	}
	for _, s := range skills {
		if s.Metadata.ID == "" {
			t.Errorf("skill has empty ID: %+v", s.Metadata)
		}
		if s.Metadata.Parameters != nil {
			// Parameters should be map[string]interface{} with nested param defs
			for k, v := range s.Metadata.Parameters {
				if v == nil {
					t.Errorf("skill %s: param %s has nil value", s.Metadata.ID, k)
				}
			}
		}
	}
}
