package ops

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"

	"github.com/rocky/marstaff/internal/config"
)

// Issue is a single doctor finding.
type Issue struct {
	Level   string // "error" or "warn"
	Code    string
	Message string
}

// RunDoctor checks config, database, and local paths. Returns non-nil issues and whether any error-level issue exists.
func RunDoctor(cfg *config.Config, configPath string) ([]Issue, bool) {
	var issues []Issue
	hasErr := false

	add := func(level, code, msg string) {
		issues = append(issues, Issue{Level: level, Code: code, Message: msg})
		if level == "error" {
			hasErr = true
		}
	}

	if configPath != "" {
		if st, err := os.Stat(configPath); err != nil {
			add("error", "config_file", fmt.Sprintf("cannot read config: %v", err))
		} else if st.IsDir() {
			add("error", "config_file", "config path is a directory")
		}
	}

	if cfg == nil {
		add("error", "config", "config is nil")
		return issues, true
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		add("error", "database_open", err.Error())
	} else {
		defer db.Close()
		if err := db.Ping(); err != nil {
			add("error", "database_ping", err.Error())
		}
	}

	skillsPath := cfg.Skills.Path
	if skillsPath == "" {
		skillsPath = "./skills"
	}
	if st, err := os.Stat(skillsPath); err != nil {
		add("warn", "skills_path", fmt.Sprintf("skills path not accessible: %v", err))
	} else if !st.IsDir() {
		add("warn", "skills_path", "skills path is not a directory")
	}

	wp := cfg.Workspace.BasePath
	if wp != "" {
		if st, err := os.Stat(wp); err != nil {
			add("warn", "workspace", fmt.Sprintf("workspace path: %v", err))
		} else if !st.IsDir() {
			add("warn", "workspace", "workspace base_path is not a directory")
		}
	}

	if cfg.GatewayNode.Token != "" && len(cfg.GatewayNode.Token) < 12 {
		add("warn", "gateway_node_token", "gateway_node.token is short; use a long random secret for production")
	}

	if cfg.Provider.Default == "" {
		add("warn", "provider_default", "provider.default is empty")
	}

	return issues, hasErr
}

// FormatIssues prints human-readable lines for terminal output.
func FormatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return "All checks passed.\n"
	}
	var b string
	for _, i := range issues {
		b += fmt.Sprintf("[%s] %s: %s\n", i.Level, i.Code, i.Message)
	}
	return b
}

// AbsSkillsPath returns absolute path for skills (for messages).
func AbsSkillsPath(cfg *config.Config) string {
	p := cfg.Skills.Path
	if p == "" {
		p = "./skills"
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
