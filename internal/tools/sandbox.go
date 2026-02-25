package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SandboxExecutor runs commands and file ops in a Docker container
type SandboxExecutor struct {
	Image string // e.g. "alpine:latest" or "marstaff-sandbox"
}

// NewSandboxExecutor creates a new sandbox executor
func NewSandboxExecutor(image string) *SandboxExecutor {
	if image == "" {
		image = "alpine:latest"
	}
	return &SandboxExecutor{Image: image}
}

// RunCommand runs a command in the sandbox
func (s *SandboxExecutor) RunCommand(ctx context.Context, workDir, command string) (string, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("invalid work_dir: %w", err)
	}
	if _, err := os.Stat(absWorkDir); os.IsNotExist(err) {
		return "", fmt.Errorf("work_dir does not exist: %s", absWorkDir)
	}

	// docker run --rm -v workdir:/workspace image sh -c "command"
	args := []string{"run", "--rm", "-v", absWorkDir + ":/workspace", "-w", "/workspace", s.Image, "sh", "-c", command}
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("sandbox command failed: %w", err)
	}
	return string(out), nil
}

// ReadFile reads a file from the sandbox (uses cat in container)
func (s *SandboxExecutor) ReadFile(ctx context.Context, workDir, path string) (string, error) {
	// Ensure path is within workDir (no .. escape)
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("invalid work_dir: %w", err)
	}
	absPath, err := filepath.Abs(filepath.Join(absWorkDir, path))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absPath, absWorkDir) {
		return "", fmt.Errorf("path escapes work_dir")
	}
	relPath := strings.TrimPrefix(absPath, absWorkDir)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))

	return s.RunCommand(ctx, absWorkDir, "cat '"+relPath+"'")
}

// WriteFile writes content to a file in the sandbox (uses base64 to preserve content)
func (s *SandboxExecutor) WriteFile(ctx context.Context, workDir, path, content string) error {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("invalid work_dir: %w", err)
	}
	absPath, err := filepath.Abs(filepath.Join(absWorkDir, path))
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absPath, absWorkDir) {
		return fmt.Errorf("path escapes work_dir")
	}
	relPath := strings.TrimPrefix(absPath, absWorkDir)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))

	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	_, err = s.RunCommand(ctx, absWorkDir, "mkdir -p $(dirname '"+relPath+"') && echo '"+b64+"' | base64 -d > '"+relPath+"'")
	return err
}
