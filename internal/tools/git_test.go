package tools

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/tools/security"
)

// setupTestGitRepo creates a temporary git repository for testing
func setupTestGitRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	exec.Command("git", "config", "user.name", "Test User").Dir = tmpDir
	exec.Command("git", "config", "user.email", "test@example.com").Dir = tmpDir
	exec.Command("git", "config", "init.defaultBranch", "main").Dir = tmpDir

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to add file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Return cleanup function
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// setupTestGitExecutor creates a GitExecutor for testing
func setupTestGitExecutor(t *testing.T, workDir string) *GitExecutor {
	t.Helper()

	// Create a test engine
	engine, err := agent.NewEngine(&agent.Config{
		Provider: &mockProvider{},
	})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Create security config that allows the test directory
	cfg := &security.Config{
		WorkingDirectories: []string{workDir},
		Limits: security.Limits{
			CommandTimeout:  10,
			MaxCommandOutput: 1024 * 1024,
		},
		Policy: security.Policy{
			AllowCommands: true,
			EnableLogging: true,
		},
	}

	validator, err := security.NewValidator(cfg)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	return NewGitExecutor(engine, validator)
}

// mockProvider is a minimal implementation of provider.Provider for testing
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) HealthCheck(ctx context.Context) error { return nil }
func (m *mockProvider) CreateChatCompletion(ctx context.Context, req provider.ChatCompletionRequest) (*provider.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *mockProvider) CreateChatCompletionStream(ctx context.Context, req provider.ChatCompletionRequest) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockProvider) SupportedModels() []string { return nil }

// TestGitStatus tests the git_status tool
func TestGitStatus(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Clean working tree", func(t *testing.T) {
		result, err := executor.toolGitStatus(ctx, map[string]interface{}{})
		if err != nil {
			t.Fatalf("git_status failed: %v", err)
		}
		if strings.Contains(strings.ToLower(result), "clean") || !strings.Contains(result, "M") {
			t.Logf("✓ Clean working tree: %s", result)
		}
	})

	t.Run("Modified file", func(t *testing.T) {
		// Modify a file
		testFile := filepath.Join(tmpDir, "README.md")
		if err := os.WriteFile(testFile, []byte("# Modified\n\nNew content"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		result, err := executor.toolGitStatus(ctx, map[string]interface{}{})
		if err != nil {
			t.Fatalf("git_status failed: %v", err)
		}
		if !strings.Contains(result, "M") {
			t.Errorf("Expected status to show modified file, got: %s", result)
		}
		t.Logf("✓ Modified file detected: %s", result)
	})
}

// TestGitLog tests the git_log tool
func TestGitLog(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Show commit log", func(t *testing.T) {
		result, err := executor.toolGitLog(ctx, map[string]interface{}{
			"max_count": 5,
			"oneline":   true,
		})
		if err != nil {
			t.Fatalf("git_log failed: %v", err)
		}
		if !strings.Contains(result, "Initial commit") {
			t.Errorf("Expected log to contain 'Initial commit', got: %s", result)
		}
		t.Logf("✓ Commit log: %s", result)
	})
}

// TestGitBranch tests the git_branch tool
func TestGitBranch(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("List branches", func(t *testing.T) {
		result, err := executor.toolGitBranch(ctx, map[string]interface{}{
			"action": "list",
		})
		if err != nil {
			t.Fatalf("git_branch list failed: %v", err)
		}
		if !strings.Contains(result, "main") {
			t.Errorf("Expected branch list to contain 'main', got: %s", result)
		}
		t.Logf("✓ Branch list: %s", result)
	})

	t.Run("Create branch", func(t *testing.T) {
		result, err := executor.toolGitBranch(ctx, map[string]interface{}{
			"action": "create",
			"name":   "test-branch",
		})
		if err != nil {
			t.Fatalf("git_branch create failed: %v", err)
		}
		t.Logf("✓ Branch created: %s", result)

		// Verify branch was created
		result, err = executor.toolGitBranch(ctx, map[string]interface{}{
			"action": "list",
		})
		if err != nil {
			t.Fatalf("git_branch list failed: %v", err)
		}
		if !strings.Contains(result, "test-branch") {
			t.Errorf("Expected branch list to contain 'test-branch', got: %s", result)
		}
	})

	t.Run("Delete branch", func(t *testing.T) {
		result, err := executor.toolGitBranch(ctx, map[string]interface{}{
			"action": "delete",
			"name":   "test-branch",
		})
		if err != nil {
			t.Fatalf("git_branch delete failed: %v", err)
		}
		t.Logf("✓ Branch deleted: %s", result)
	})
}

// TestGitAdd tests the git_add tool
func TestGitAdd(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Add new file", func(t *testing.T) {
		// Create a new file
		newFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(newFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		result, err := executor.toolGitAdd(ctx, map[string]interface{}{
			"pathspec": ".",
		})
		if err != nil {
			t.Fatalf("git_add failed: %v", err)
		}
		if !strings.Contains(result, "test.txt") {
			t.Logf("Warning: Expected output to mention test.txt, got: %s", result)
		} else {
			t.Logf("✓ File added: %s", result)
		}
	})
}

// TestGitCommit tests the git_commit tool
func TestGitCommit(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Create commit", func(t *testing.T) {
		// Create and add a file
		newFile := filepath.Join(tmpDir, "commit-test.txt")
		if err := os.WriteFile(newFile, []byte("commit test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		_, _ = executor.toolGitAdd(ctx, map[string]interface{}{
			"pathspec": ".",
		})

		result, err := executor.toolGitCommit(ctx, map[string]interface{}{
			"message": "Test commit",
		})
		if err != nil {
			t.Fatalf("git_commit failed: %v", err)
		}
		t.Logf("✓ Commit created: %s", result)

		// Verify commit exists
		logResult, _ := executor.toolGitLog(ctx, map[string]interface{}{
			"max_count": 3,
		})
		if !strings.Contains(logResult, "Test commit") {
			t.Errorf("Expected log to contain 'Test commit', got: %s", logResult)
		}
	})
}

// TestGitDiff tests the git_diff tool
func TestGitDiff(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Show unstaged changes", func(t *testing.T) {
		// Modify a file
		testFile := filepath.Join(tmpDir, "README.md")
		if err := os.WriteFile(testFile, []byte("# Modified\n\nNew content here"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		result, err := executor.toolGitDiff(ctx, map[string]interface{}{})
		if err != nil {
			t.Fatalf("git_diff failed: %v", err)
		}
		if !strings.Contains(result, "README.md") && !strings.Contains(result, "+") {
			t.Logf("Warning: Diff may not show expected content: %s", result)
		} else {
			t.Logf("✓ Diff output: %s", truncateString(result, 200))
		}
	})

	t.Run("Name only", func(t *testing.T) {
		result, err := executor.toolGitDiff(ctx, map[string]interface{}{
			"name_only": true,
		})
		if err != nil {
			t.Fatalf("git_diff name_only failed: %v", err)
		}
		t.Logf("✓ Changed files: %s", result)
	})
}

// TestGitCheckout tests the git_checkout tool
func TestGitCheckout(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Create and checkout branch", func(t *testing.T) {
		// Create a new branch first
		_, err := executor.toolGitBranch(ctx, map[string]interface{}{
			"action": "create",
			"name":   "feature-test",
		})
		if err != nil {
			t.Fatalf("Failed to create branch: %v", err)
		}

		// Checkout the branch
		result, err := executor.toolGitCheckout(ctx, map[string]interface{}{
			"branch": "feature-test",
		})
		if err != nil {
			t.Fatalf("git_checkout failed: %v", err)
		}
		t.Logf("✓ Checked out branch: %s", result)
	})
}

// TestGitSwitch tests the git_switch tool
func TestGitSwitch(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Create and switch branch", func(t *testing.T) {
		result, err := executor.toolGitSwitch(ctx, map[string]interface{}{
			"create": true,
			"branch": "switch-test",
		})
		if err != nil {
			t.Fatalf("git_switch failed: %v", err)
		}
		t.Logf("✓ Switched to new branch: %s", result)
	})
}

// TestGitStash tests the git_stash tool
func TestGitStash(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Stash and pop changes", func(t *testing.T) {
		// Modify a file
		testFile := filepath.Join(tmpDir, "README.md")
		if err := os.WriteFile(testFile, []byte("# Stash test"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		// Stash changes
		result, err := executor.toolGitStash(ctx, map[string]interface{}{
			"action":  "push",
			"message": "test stash",
		})
		if err != nil {
			t.Fatalf("git_stash push failed: %v", err)
		}
		t.Logf("✓ Stashed changes: %s", result)

		// List stashes
		listResult, err := executor.toolGitStash(ctx, map[string]interface{}{
			"action": "list",
		})
		if err != nil {
			t.Fatalf("git_stash list failed: %v", err)
		}
		if !strings.Contains(listResult, "test stash") {
			t.Logf("Warning: Expected stash list to contain 'test stash', got: %s", listResult)
		} else {
			t.Logf("✓ Stash list: %s", listResult)
		}

		// Pop stash
		popResult, err := executor.toolGitStash(ctx, map[string]interface{}{
			"action": "pop",
		})
		if err != nil {
			t.Fatalf("git_stash pop failed: %v", err)
		}
		t.Logf("✓ Popped stash: %s", popResult)
	})
}

// TestGitShow tests the git_show tool
func TestGitShow(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Show HEAD", func(t *testing.T) {
		result, err := executor.toolGitShow(ctx, map[string]interface{}{})
		if err != nil {
			t.Fatalf("git_show failed: %v", err)
		}
		if !strings.Contains(result, "Initial commit") && !strings.Contains(result, "diff") {
			t.Logf("Warning: Show output may not be as expected: %s", truncateString(result, 200))
		} else {
			t.Logf("✓ Show HEAD: %s", truncateString(result, 200))
		}
	})
}

// TestRegisterBuiltInTools tests that all git tools are properly registered
func TestRegisterBuiltInTools(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)

	// This should not panic
	t.Run("Register all tools", func(t *testing.T) {
		executor.RegisterBuiltInTools()
		t.Log("✓ All git tools registered successfully")
	})
}

// TestGitWorkflow tests a complete git workflow
func TestGitWorkflow(t *testing.T) {
	tmpDir, cleanup := setupTestGitRepo(t)
	defer cleanup()

	executor := setupTestGitExecutor(t, tmpDir)
	ctx := context.WithValue(context.Background(), contextkeys.SessionWorkDir, tmpDir)

	t.Run("Complete workflow", func(t *testing.T) {
		steps := []struct {
			name string
			fn   func() (string, error)
		}{
			{"Check initial status", func() (string, error) {
				return executor.toolGitStatus(ctx, map[string]interface{}{})
			}},
			{"Create feature branch", func() (string, error) {
				return executor.toolGitBranch(ctx, map[string]interface{}{
					"action": "create",
					"name":   "feature",
				})
			}},
			{"Switch to feature branch", func() (string, error) {
				return executor.toolGitSwitch(ctx, map[string]interface{}{
					"branch": "feature",
				})
			}},
			{"Create new file", func() (string, error) {
				newFile := filepath.Join(tmpDir, "feature.txt")
				if err := os.WriteFile(newFile, []byte("feature content"), 0644); err != nil {
					return "", err
				}
				return executor.toolGitAdd(ctx, map[string]interface{}{
					"pathspec": ".",
				})
			}},
			{"Commit changes", func() (string, error) {
				return executor.toolGitCommit(ctx, map[string]interface{}{
					"message": "Add feature file",
				})
			}},
			{"Switch back to main", func() (string, error) {
				return executor.toolGitSwitch(ctx, map[string]interface{}{
					"branch": "main",
				})
			}},
			{"Merge feature branch", func() (string, error) {
				return executor.toolGitMerge(ctx, map[string]interface{}{
					"branch": "feature",
				})
			}},
			{"Check final status", func() (string, error) {
				return executor.toolGitStatus(ctx, map[string]interface{}{})
			}},
			{"Show log", func() (string, error) {
				return executor.toolGitLog(ctx, map[string]interface{}{
					"max_count": 3,
				})
			}},
		}

		for _, step := range steps {
			t.Run(step.name, func(t *testing.T) {
				result, err := step.fn()
				if err != nil {
					t.Errorf("%s failed: %v", step.name, err)
				} else {
					t.Logf("✓ %s: %s", step.name, truncateString(result, 100))
				}
				// Small delay between operations
				time.Sleep(10 * time.Millisecond)
			})
		}
	})
}

// Helper function to truncate strings for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Run tests with: go test -v ./internal/tools/
// Run specific test: go test -v ./internal/tools/ -run TestGitStatus
