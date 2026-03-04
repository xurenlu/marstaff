package envvars

import (
	"context"
	"os"
	"strings"

	"github.com/rocky/marstaff/internal/repository"
)

// Provider returns merged env vars for command execution
type Provider interface {
	GetMergedEnv(ctx context.Context) ([]string, error)
}

// Loader provides merged environment variables for command execution
type Loader struct {
	repo *repository.EnvVarRepository
}

// NewLoader creates a new env vars loader
func NewLoader(repo *repository.EnvVarRepository) *Loader {
	return &Loader{repo: repo}
}

// GetMergedEnv returns environment slice for cmd.Env.
// DB-configured vars override os.Environ(). Nil repo returns os.Environ() as-is.
func (l *Loader) GetMergedEnv(ctx context.Context) ([]string, error) {
	base := os.Environ()
	if l == nil || l.repo == nil {
		return base, nil
	}

	dbVars, err := l.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(dbVars) == 0 {
		return base, nil
	}

	// Track which keys we've seen from base
	seen := make(map[string]bool)
	out := make([]string, 0, len(base)+len(dbVars))

	// First pass: base vars, with DB overrides
	for _, s := range base {
		if idx := strings.Index(s, "="); idx > 0 {
			k := s[:idx]
			seen[k] = true
			if v, ok := dbVars[k]; ok {
				out = append(out, k+"="+v)
			} else {
				out = append(out, s)
			}
		}
	}

	// Second pass: DB-only vars (not in base)
	for k, v := range dbVars {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out, nil
}
