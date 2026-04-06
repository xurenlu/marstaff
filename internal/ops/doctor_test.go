package ops

import (
	"testing"

	"github.com/rocky/marstaff/internal/config"
)

func TestFormatIssuesEmpty(t *testing.T) {
	s := FormatIssues(nil)
	if s == "" {
		t.Fatal("expected non-empty")
	}
}

func TestRunDoctorNilConfig(t *testing.T) {
	issues, hasErr := RunDoctor(nil, "")
	if !hasErr || len(issues) == 0 {
		t.Fatalf("expected error issues, got %v hasErr=%v", issues, hasErr)
	}
}

func TestAbsSkillsPath(t *testing.T) {
	p := AbsSkillsPath(&config.Config{Skills: config.SkillsConfig{Path: "./skills"}})
	if p == "" {
		t.Fatal("empty path")
	}
}
