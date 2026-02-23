package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/tools/security"
)

// GitExecutor handles git workflow tools with security validation
type GitExecutor struct {
	engine    *agent.Engine
	validator *security.Validator
}

// NewGitExecutor creates a new git tool executor
func NewGitExecutor(engine *agent.Engine, validator *security.Validator) *GitExecutor {
	return &GitExecutor{
		engine:    engine,
		validator: validator,
	}
}

// RegisterBuiltInTools registers all git workflow tools
func (e *GitExecutor) RegisterBuiltInTools() {
	// Basic operations
	e.registerGitStatus()
	e.registerGitDiff()
	e.registerGitLog()
	e.registerGitShow()

	// Branch operations
	e.registerGitBranch()
	e.registerGitCheckout()
	e.registerGitSwitch()

	// Commit operations
	e.registerGitAdd()
	e.registerGitCommit()
	e.registerGitAmend()

	// Remote operations
	e.registerGitRemote()
	e.registerGitFetch()
	e.registerGitPull()
	e.registerGitPush()

	// Advanced operations
	e.registerGitMerge()
	e.registerGitRebase()
	e.registerGitStash()
	e.registerGitReset()
	e.registerGitRevert()
	e.registerGitCherryPick()

	// Additional operations
	e.registerGitTag()
	e.registerGitReflog()
	e.registerGitBlame()
	e.registerGitClean()
	e.registerGitClone()
}

// getWorkDir returns the working directory for git operations
func (e *GitExecutor) getWorkDir(ctx context.Context) (string, error) {
	// Prefer session work_dir (edit mode)
	if wd := ctx.Value(contextkeys.SessionWorkDir); wd != nil {
		if s, ok := wd.(string); ok && s != "" {
			abs, err := filepath.Abs(s)
			if err != nil {
				return "", fmt.Errorf("invalid work_dir: %w", err)
			}
			return abs, nil
		}
	}

	// Fallback to first configured directory
	workingDirs := e.validator.GetConfig().WorkingDirectories
	if len(workingDirs) == 0 {
		return "", fmt.Errorf("no working directories configured")
	}
	return workingDirs[0], nil
}

// runGitCommand executes a git command in the working directory
func (e *GitExecutor) runGitCommand(ctx context.Context, args ...string) (string, error) {
	workDir, err := e.getWorkDir(ctx)
	if err != nil {
		return "", err
	}

	// Create context with timeout from config
	timeout := time.Duration(e.validator.GetConfig().Limits.CommandTimeout) * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build git command
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = workDir

	// Run command and capture output
	output, err := cmd.CombinedOutput()

	// Check output size limit
	maxOutput := e.validator.GetConfig().Limits.MaxCommandOutput
	if int64(len(output)) > maxOutput {
		output = output[:maxOutput]
		output = append(output, []byte("\n\n[Output truncated due to size limit]")...)
	}

	result := string(output)

	// Check if command timed out
	if cmdCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git command timed out")
	}

	// Return error if command failed
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), fmt.Errorf("%s", result))
	}

	log.Info().
		Str("work_dir", workDir).
		Str("args", strings.Join(args, " ")).
		Int("output_size", len(result)).
		Msg("git command executed successfully")

	return result, nil
}

// parseGitStatus parses git status --porcelain output
func parseGitStatus(output string) map[string][]string {
	result := map[string][]string{
		"modified":  {},
		"added":     {},
		"deleted":   {},
		"untracked": {},
		"renamed":   {},
		"copied":    {},
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[3:])

		switch {
		case strings.HasPrefix(status, "M"):
			result["modified"] = append(result["modified"], path)
		case strings.HasPrefix(status, "A"):
			result["added"] = append(result["added"], path)
		case strings.HasPrefix(status, "D"):
			result["deleted"] = append(result["deleted"], path)
		case strings.HasPrefix(status, "??"):
			result["untracked"] = append(result["untracked"], path)
		case strings.HasPrefix(status, "R"):
			result["renamed"] = append(result["renamed"], path)
		case strings.HasPrefix(status, "C"):
			result["copied"] = append(result["copied"], path)
		}
	}

	return result
}

// ============== Basic Operations ==============

func (e *GitExecutor) registerGitStatus() {
	e.engine.RegisterTool("git_status",
		"Shows the working tree status",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"porcelain": map[string]interface{}{
					"type":        "boolean",
					"description": "Output in machine-readable format (default: false)",
				},
			},
		}, e.toolGitStatus)
}

func (e *GitExecutor) toolGitStatus(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"status", "-sb"}
	if getBool(params, "porcelain", false) {
		args = []string{"status", "--porcelain"}
	}

	output, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	// Parse and format output for better readability
	if !getBool(params, "porcelain", false) {
		return output, nil
	}

	// Parse porcelain output
	status := parseGitStatus(output)
	var b strings.Builder
	b.WriteString("Git Status:\n")

	for category, files := range status {
		if len(files) > 0 {
			b.WriteString(fmt.Sprintf("\n%s:\n", category))
			for _, f := range files {
				b.WriteString(fmt.Sprintf("  - %s\n", f))
			}
		}
	}

	if b.Len() == len("Git Status:\n") {
		return "Working tree clean", nil
	}

	return b.String(), nil
}

func (e *GitExecutor) registerGitDiff() {
	e.engine.RegisterTool("git_diff",
		"Shows changes between commits, commit and working tree, etc",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cached": map[string]interface{}{
					"type":        "boolean",
					"description": "Show staged changes instead of unstaged (default: false)",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Limit diff to specific file or directory",
				},
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Show diff between commits (e.g., 'HEAD~1', 'abc123..def456')",
				},
				"name_only": map[string]interface{}{
					"type":        "boolean",
					"description": "Show only changed file names (default: false)",
				},
			},
		}, e.toolGitDiff)
}

func (e *GitExecutor) toolGitDiff(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"diff"}

	if getBool(params, "cached", false) {
		args = append(args, "--cached")
	}

	if getBool(params, "name_only", false) {
		args = append(args, "--name-only")
	}

	if commit, _ := getString(params, "commit", false); commit != "" {
		args = append(args, commit)
	}

	if path, _ := getString(params, "path", false); path != "" {
		args = append(args, "--", path)
	}

	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitLog() {
	e.engine.RegisterTool("git_log",
		"Shows the commit logs",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"max_count": map[string]interface{}{
					"type":        "integer",
					"description": "Limit number of commits to show (default: 10)",
				},
				"oneline": map[string]interface{}{
					"type":        "boolean",
					"description": "Show one commit per line (default: true)",
				},
				"graph": map[string]interface{}{
					"type":        "boolean",
					"description": "Show ASCII graph of branch history (default: false)",
				},
				"author": map[string]interface{}{
					"type":        "string",
					"description": "Filter by author",
				},
				"since": map[string]interface{}{
					"type":        "string",
					"description": "Show commits since (e.g., '2 weeks ago', '2024-01-01')",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Limit log to specific file or directory",
				},
			},
		}, e.toolGitLog)
}

func (e *GitExecutor) toolGitLog(ctx context.Context, params map[string]interface{}) (string, error) {
	maxCount, _ := getInt(params, "max_count", false, 10)
	oneline := getBool(params, "oneline", true)
	graph := getBool(params, "graph", false)

	args := []string{"log"}
	if oneline {
		args = append(args, "--oneline")
	}
	if graph {
		args = append(args, "--graph", "--decorate")
	}
	args = append(args, fmt.Sprintf("-%d", maxCount))

	if author, _ := getString(params, "author", false); author != "" {
		args = append(args, fmt.Sprintf("--author=%s", author))
	}

	if since, _ := getString(params, "since", false); since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}

	if path, _ := getString(params, "path", false); path != "" {
		args = append(args, "--", path)
	}

	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitShow() {
	e.engine.RegisterTool("git_show",
		"Shows various types of objects (commits, tags, trees, blobs)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Revision to show (default: HEAD)",
				},
				"name_only": map[string]interface{}{
					"type":        "boolean",
					"description": "Show only file names (default: false)",
				},
			},
		}, e.toolGitShow)
}

func (e *GitExecutor) toolGitShow(ctx context.Context, params map[string]interface{}) (string, error) {
	revision, _ := getString(params, "revision", false)
	if revision == "" {
		revision = "HEAD"
	}

	args := []string{"show"}
	if getBool(params, "name_only", false) {
		args = append(args, "--name-only")
	}
	args = append(args, revision)

	return e.runGitCommand(ctx, args...)
}

// ============== Branch Operations ==============

func (e *GitExecutor) registerGitBranch() {
	e.engine.RegisterTool("git_branch",
		"Lists, creates, or deletes branches",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action to perform: list (default), create, delete, rename",
					"enum":        []string{"list", "create", "delete", "rename"},
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Branch name (for create/delete/rename)",
				},
				"new_name": map[string]interface{}{
					"type":        "string",
					"description": "New branch name (for rename)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Force delete or rename (default: false)",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "List both local and remote branches (default: false)",
				},
			},
		}, e.toolGitBranch)
}

func (e *GitExecutor) toolGitBranch(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		args := []string{"branch", "--list", "--format=%(refname:short)%09%(HEAD)%09%(authorname)%09%(committerdate:short)"}
		if getBool(params, "all", false) {
			args = append(args, "--all")
		}
		output, err := e.runGitCommand(ctx, args...)
		if err != nil {
			return "", err
		}
		return formatBranchList(output), nil

	case "create":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "branch", name)

	case "delete":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		args := []string{"branch", "-d"}
		if getBool(params, "force", false) {
			args = []string{"branch", "-D"}
		}
		args = append(args, name)
		return e.runGitCommand(ctx, args...)

	case "rename":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		newName, err := getString(params, "new_name", true)
		if err != nil {
			return "", err
		}
		args := []string{"branch", "-m"}
		if getBool(params, "force", false) {
			args = []string{"branch", "-M"}
		}
		args = append(args, name, newName)
		return e.runGitCommand(ctx, args...)

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func formatBranchList(output string) string {
	if output == "" {
		return "No branches found"
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "No branches found"
	}

	var b strings.Builder
	b.WriteString("Branches:\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			name := parts[0]
			head := parts[1]
			indicator := "  "
			if head == "*" {
				indicator = "* "
			}
			author := ""
			date := ""
			if len(parts) >= 4 {
				author = parts[2]
				date = parts[3]
			}
			if author != "" {
				b.WriteString(fmt.Sprintf("%s%s (%s, %s)\n", indicator, name, author, date))
			} else {
				b.WriteString(fmt.Sprintf("%s%s\n", indicator, name))
			}
		}
	}
	return b.String()
}

func (e *GitExecutor) registerGitCheckout() {
	e.engine.RegisterTool("git_checkout",
		"Switches branches or restores working tree files",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to checkout",
				},
				"create_new": map[string]interface{}{
					"type":        "boolean",
					"description": "Create and checkout new branch (default: false)",
				},
				"files": map[string]interface{}{
					"type":        "string",
					"description": "Restore specific file(s) from index",
				},
				"theirs": map[string]interface{}{
					"type":        "boolean",
					"description": "Use their version for merge conflicts (default: false)",
				},
				"ours": map[string]interface{}{
					"type":        "boolean",
					"description": "Use our version for merge conflicts (default: false)",
				},
			},
		}, e.toolGitCheckout)
}

func (e *GitExecutor) toolGitCheckout(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"checkout"}

	if files, _ := getString(params, "files", false); files != "" {
		// Restore files
		if getBool(params, "theirs", false) {
			args = append(args, "--theirs")
		}
		if getBool(params, "ours", false) {
			args = append(args, "--ours")
		}
		args = append(args, "--", files)
		return e.runGitCommand(ctx, args...)
	}

	branch, err := getString(params, "branch", true)
	if err != nil {
		return "", err
	}

	if getBool(params, "create_new", false) {
		args = append(args, "-b")
	}
	args = append(args, branch)

	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitSwitch() {
	e.engine.RegisterTool("git_switch",
		"Switches branches (modern alternative to checkout)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to switch to",
				},
				"create": map[string]interface{}{
					"type":        "boolean",
					"description": "Create and switch to new branch (default: false)",
				},
				"detach": map[string]interface{}{
					"type":        "boolean",
					"description": "Switch to a detached HEAD (default: false)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Proceed even if index has uncommitted changes (default: false)",
				},
			},
		}, e.toolGitSwitch)
}

func (e *GitExecutor) toolGitSwitch(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"switch"}

	if getBool(params, "create", false) {
		args = append(args, "-c")
	}
	if getBool(params, "detach", false) {
		args = append(args, "--detach")
	}
	if getBool(params, "force", false) {
		args = append(args, "--force")
	}

	branch, err := getString(params, "branch", true)
	if err != nil {
		return "", err
	}
	args = append(args, branch)

	return e.runGitCommand(ctx, args...)
}

// ============== Commit Operations ==============

func (e *GitExecutor) registerGitAdd() {
	e.engine.RegisterTool("git_add",
		"Adds file contents to the staging area",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pathspec": map[string]interface{}{
					"type":        "string",
					"description": "Files/directories to add (use '.' for all, or specific path)",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "Add all modified and deleted files (default: false)",
				},
				"update": map[string]interface{}{
					"type":        "boolean",
					"description": "Only update files already tracked (default: false)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Allow adding otherwise ignored files (default: false)",
				},
			},
		}, e.toolGitAdd)
}

func (e *GitExecutor) toolGitAdd(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"add"}

	if getBool(params, "all", false) {
		args = append(args, "-A")
	}
	if getBool(params, "update", false) {
		args = append(args, "-u")
	}
	if getBool(params, "force", false) {
		args = append(args, "-f")
	}

	pathspec, _ := getString(params, "pathspec", false)
	if pathspec != "" {
		args = append(args, "--", pathspec)
	} else if !getBool(params, "all", false) && !getBool(params, "update", false) {
		return "", fmt.Errorf("specify pathspec or use all=true")
	}

	output, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	// Check what was staged
	statusOutput, _ := e.runGitCommand(ctx, "status", "--porcelain")
	if statusOutput == "" {
		return output, nil
	}

	status := parseGitStatus(statusOutput)
	var staged []string
	staged = append(staged, status["added"]...)
	staged = append(staged, status["modified"]...)
	staged = append(staged, status["deleted"]...)

	if len(staged) > 0 {
		return fmt.Sprintf("Staged %d file(s):\n  - %s", len(staged), strings.Join(staged, "\n  - ")), nil
	}

	return output, nil
}

func (e *GitExecutor) registerGitCommit() {
	e.engine.RegisterTool("git_commit",
		"Records changes to the repository",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Commit message",
				},
				"allow_empty": map[string]interface{}{
					"type":        "boolean",
					"description": "Allow empty commit (default: false)",
				},
				"no_verify": map[string]interface{}{
					"type":        "boolean",
					"description": "Bypass pre-commit hooks (default: false)",
				},
				"amend": map[string]interface{}{
					"type":        "boolean",
					"description": "Amend previous commit (default: false)",
				},
			},
			"required": []string{"message"},
		}, e.toolGitCommit)
}

func (e *GitExecutor) toolGitCommit(ctx context.Context, params map[string]interface{}) (string, error) {
	message, err := getString(params, "message", true)
	if err != nil {
		return "", err
	}

	args := []string{"commit", "-m", message}

	if getBool(params, "allow_empty", false) {
		args = append(args, "--allow-empty")
	}
	if getBool(params, "no_verify", false) {
		args = append(args, "--no-verify")
	}
	if getBool(params, "amend", false) {
		args = append(args, "--amend")
	}

	output, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	// Get commit info
	var commitOutput string
	if getBool(params, "amend", false) {
		commitOutput, _ = e.runGitCommand(ctx, "log", "-1", "--oneline")
	} else {
		commitOutput, _ = e.runGitCommand(ctx, "log", "-1", "--oneline")
	}

	if commitOutput != "" {
		return fmt.Sprintf("Commit created:\n%s", commitOutput), nil
	}

	return output, nil
}

func (e *GitExecutor) registerGitAmend() {
	e.engine.RegisterTool("git_amend",
		"Amends the most recent commit",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "New commit message (optional)",
				},
				"no_edit": map[string]interface{}{
					"type":        "boolean",
					"description": "Keep existing message (default: true)",
				},
			},
		}, e.toolGitAmend)
}

func (e *GitExecutor) toolGitAmend(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"commit", "--amend"}

	if message, _ := getString(params, "message", false); message != "" {
		args = append(args, "-m", message)
	} else if getBool(params, "no_edit", true) {
		args = append(args, "--no-edit")
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	logOutput, _ := e.runGitCommand(ctx, "log", "-1", "--oneline")
	return fmt.Sprintf("Amended commit:\n%s", logOutput), nil
}

// ============== Remote Operations ==============

func (e *GitExecutor) registerGitFetch() {
	e.engine.RegisterTool("git_fetch",
		"Downloads objects and refs from another repository",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"remote": map[string]interface{}{
					"type":        "string",
					"description": "Remote name (default: origin)",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Specific branch to fetch",
				},
				"prune": map[string]interface{}{
					"type":        "boolean",
					"description": "Remove remote-tracking references that no longer exist (default: false)",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "Fetch from all remotes (default: false)",
				},
				"tags": map[string]interface{}{
					"type":        "boolean",
					"description": "Fetch all tags (default: false)",
				},
			},
		}, e.toolGitFetch)
}

func (e *GitExecutor) toolGitFetch(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"fetch"}

	if getBool(params, "all", false) {
		args = append(args, "--all")
	}
	if getBool(params, "prune", false) {
		args = append(args, "--prune")
	}
	if getBool(params, "tags", false) {
		args = append(args, "--tags")
	}

	remote, _ := getString(params, "remote", false)
	branch, _ := getString(params, "branch", false)

	if remote != "" && branch != "" {
		args = append(args, remote, branch)
	} else if remote != "" {
		args = append(args, remote)
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Fetch completed successfully", nil
}

func (e *GitExecutor) registerGitPull() {
	e.engine.RegisterTool("git_pull",
		"Fetches from and integrates with another repository or a local branch",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"remote": map[string]interface{}{
					"type":        "string",
					"description": "Remote name (default: origin)",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Remote branch to pull",
				},
				"rebase": map[string]interface{}{
					"type":        "boolean",
					"description": "Use rebase instead of merge (default: false)",
				},
				"no_commit": map[string]interface{}{
					"type":        "boolean",
					"description": "Don't create merge commit (default: false)",
				},
				"strategy": map[string]interface{}{
					"type":        "string",
					"description": "Merge strategy: resolve, recursive, octopus, ours, subtree",
				},
			},
		}, e.toolGitPull)
}

func (e *GitExecutor) toolGitPull(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"pull"}

	if getBool(params, "rebase", false) {
		args = append(args, "--rebase")
	}
	if getBool(params, "no_commit", false) {
		args = append(args, "--no-commit")
	}

	if strategy, _ := getString(params, "strategy", false); strategy != "" {
		args = append(args, fmt.Sprintf("-s=%s", strategy))
	}

	remote, _ := getString(params, "remote", false)
	branch, _ := getString(params, "branch", false)

	if remote != "" && branch != "" {
		args = append(args, remote, branch)
	} else if remote != "" {
		args = append(args, remote)
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Pull completed successfully", nil
}

func (e *GitExecutor) registerGitPush() {
	e.engine.RegisterTool("git_push",
		"Updates remote refs along with associated objects",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"remote": map[string]interface{}{
					"type":        "string",
					"description": "Remote name (default: origin)",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to push (default: current)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Force push (default: false)",
				},
				"force_with_lease": map[string]interface{}{
					"type":        "boolean",
					"description": "Safer force push (default: false)",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "Push all branches (default: false)",
				},
				"tags": map[string]interface{}{
					"type":        "boolean",
					"description": "Push all tags (default: false)",
				},
				"set_upstream": map[string]interface{}{
					"type":        "boolean",
					"description": "Set upstream tracking branch (default: false)",
				},
				"delete": map[string]interface{}{
					"type":        "string",
					"description": "Delete remote branch",
				},
			},
		}, e.toolGitPush)
}

func (e *GitExecutor) toolGitPush(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"push"}

	if getBool(params, "all", false) {
		args = append(args, "--all")
	}
	if getBool(params, "tags", false) {
		args = append(args, "--tags")
	}
	if getBool(params, "force", false) {
		args = append(args, "--force")
	}
	if getBool(params, "force_with_lease", false) {
		args = append(args, "--force-with-lease")
	}
	if getBool(params, "set_upstream", false) {
		args = append(args, "-u")
	}

	remote, _ := getString(params, "remote", false)
	if remote == "" {
		remote = "origin"
	}

	if deleteBranch, _ := getString(params, "delete", false); deleteBranch != "" {
		args = append(args, "--delete", deleteBranch)
		args = append(args, remote)
		return e.runGitCommand(ctx, args...)
	}

	branch, _ := getString(params, "branch", false)
	if branch != "" {
		args = append(args, remote, branch)
	} else {
		args = append(args, remote)
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Push completed successfully", nil
}

// ============== Advanced Operations ==============

func (e *GitExecutor) registerGitMerge() {
	e.engine.RegisterTool("git_merge",
		"Joins two or more development histories together",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch/commit to merge",
				},
				"no_commit": map[string]interface{}{
					"type":        "boolean",
					"description": "Don't create merge commit (default: false)",
				},
				"squash": map[string]interface{}{
					"type":        "boolean",
					"description": "Squash all commits into one (default: false)",
				},
				"strategy": map[string]interface{}{
					"type":        "string",
					"description": "Merge strategy: resolve, recursive, octopus, ours, subtree",
				},
				"no_ff": map[string]interface{}{
					"type":        "boolean",
					"description": "Always create merge commit (default: false)",
				},
				"ff_only": map[string]interface{}{
					"type":        "boolean",
					"description": "Only fast-forward (default: false)",
				},
			},
		}, e.toolGitMerge)
}

func (e *GitExecutor) toolGitMerge(ctx context.Context, params map[string]interface{}) (string, error) {
	branch, err := getString(params, "branch", true)
	if err != nil {
		return "", err
	}

	args := []string{"merge"}

	if getBool(params, "no_commit", false) {
		args = append(args, "--no-commit")
	}
	if getBool(params, "squash", false) {
		args = append(args, "--squash")
	}
	if getBool(params, "no_ff", false) {
		args = append(args, "--no-ff")
	}
	if getBool(params, "ff_only", false) {
		args = append(args, "--ff-only")
	}

	if strategy, _ := getString(params, "strategy", false); strategy != "" {
		args = append(args, fmt.Sprintf("-s=%s", strategy))
	}

	args = append(args, branch)

	_, err = e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Merge completed successfully", nil
}

func (e *GitExecutor) registerGitRebase() {
	e.engine.RegisterTool("git_rebase",
		"Reapplies commits on top of another base tip",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"upstream": map[string]interface{}{
					"type":        "string",
					"description": "Upstream branch to rebase onto",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to rebase (default: current)",
				},
				"interactive": map[string]interface{}{
					"type":        "boolean",
					"description": "Interactive rebase (not supported in non-interactive mode)",
				},
				"onto": map[string]interface{}{
					"type":        "string",
					"description": "New base for the rebased commits",
				},
				"continue": map[string]interface{}{
					"type":        "boolean",
					"description": "Continue after conflict resolution (default: false)",
				},
				"skip": map[string]interface{}{
					"type":        "boolean",
					"description": "Skip current patch (default: false)",
				},
				"abort": map[string]interface{}{
					"type":        "boolean",
					"description": "Abort rebase (default: false)",
				},
				"autostash": map[string]interface{}{
					"type":        "boolean",
					"description": "Auto-stash before rebase (default: false)",
				},
			},
		}, e.toolGitRebase)
}

func (e *GitExecutor) toolGitRebase(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"rebase"}

	if getBool(params, "interactive", false) {
		return "", fmt.Errorf("interactive rebase is not supported in non-interactive mode")
	}
	if getBool(params, "autostash", false) {
		args = append(args, "--autostash")
	}
	if getBool(params, "continue", false) {
		args = append(args, "--continue")
	}
	if getBool(params, "skip", false) {
		args = append(args, "--skip")
	}
	if getBool(params, "abort", false) {
		args = append(args, "--abort")
	}

	upstream, _ := getString(params, "upstream", false)
	onto, _ := getString(params, "onto", false)
	branch, _ := getString(params, "branch", false)

	if onto != "" {
		args = append(args, "--onto", onto)
	}

	if upstream != "" {
		args = append(args, upstream)
		if branch != "" {
			args = append(args, branch)
		}
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Rebase completed successfully", nil
}

func (e *GitExecutor) registerGitStash() {
	e.engine.RegisterTool("git_stash",
		"Stashes away changes in a dirty working directory",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: push (default), pop, apply, list, drop, clear, show",
					"enum":        []string{"push", "pop", "apply", "list", "drop", "clear", "show"},
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Stash message",
				},
				"index": map[string]interface{}{
					"type":        "integer",
					"description": "Stash index (for pop/apply/drop/show, default: 0)",
				},
				"include_untracked": map[string]interface{}{
					"type":        "boolean",
					"description": "Include untracked files (default: false)",
				},
				"keep_index": map[string]interface{}{
					"type":        "boolean",
					"description": "Keep index changes (default: false)",
				},
			},
		}, e.toolGitStash)
}

func (e *GitExecutor) toolGitStash(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "push"
	}

	switch action {
	case "list":
		return e.runGitCommand(ctx, "stash", "list")

	case "show":
		idx, _ := getInt(params, "index", false, 0)
		args := []string{"stash", "show", fmt.Sprintf("stash@{%d}", idx)}
		return e.runGitCommand(ctx, args...)

	case "push":
		args := []string{"stash", "push"}
		if getBool(params, "include_untracked", false) {
			args = append(args, "-u")
		}
		if getBool(params, "keep_index", false) {
			args = append(args, "--keep-index")
		}
		if message, _ := getString(params, "message", false); message != "" {
			args = append(args, "-m", message)
		}
		return e.runGitCommand(ctx, args...)

	case "pop":
		idx, _ := getInt(params, "index", false, 0)
		return e.runGitCommand(ctx, "stash", "pop", fmt.Sprintf("stash@{%d}", idx))

	case "apply":
		idx, _ := getInt(params, "index", false, 0)
		return e.runGitCommand(ctx, "stash", "apply", fmt.Sprintf("stash@{%d}", idx))

	case "drop":
		idx, _ := getInt(params, "index", false, 0)
		return e.runGitCommand(ctx, "stash", "drop", fmt.Sprintf("stash@{%d}", idx))

	case "clear":
		return e.runGitCommand(ctx, "stash", "clear")

	default:
		return "", fmt.Errorf("unknown stash action: %s", action)
	}
}

func (e *GitExecutor) registerGitReset() {
	e.engine.RegisterTool("git_reset",
		"Resets the current HEAD to the specified state",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Reset mode: soft, mixed, hard (default: mixed)",
					"enum":        []string{"soft", "mixed", "hard"},
				},
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Commit to reset to (default: HEAD)",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to reset (resets only specified paths)",
				},
			},
		}, e.toolGitReset)
}

func (e *GitExecutor) toolGitReset(ctx context.Context, params map[string]interface{}) (string, error) {
	mode, _ := getString(params, "mode", false)
	if mode == "" {
		mode = "mixed"
	}

	args := []string{"reset"}

	switch mode {
	case "soft":
		args = append(args, "--soft")
	case "mixed":
		// Default, no flag needed
	case "hard":
		args = append(args, "--hard")
	}

	commit, _ := getString(params, "commit", false)
	if commit != "" {
		args = append(args, commit)
	} else {
		args = append(args, "HEAD")
	}

	path, _ := getString(params, "path", false)
	if path != "" {
		args = append(args, "--", path)
	}

	_, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Reset to %s mode (%s)", mode, commit), nil
}

func (e *GitExecutor) registerGitRevert() {
	e.engine.RegisterTool("git_revert",
		"Reverts some existing commits",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Commit(s) to revert",
				},
				"no_commit": map[string]interface{}{
					"type":        "boolean",
					"description": "Don't auto-commit (default: false)",
				},
			},
		}, e.toolGitRevert)
}

func (e *GitExecutor) toolGitRevert(ctx context.Context, params map[string]interface{}) (string, error) {
	commit, err := getString(params, "commit", true)
	if err != nil {
		return "", err
	}

	args := []string{"revert"}
	if getBool(params, "no_commit", false) {
		args = append(args, "--no-commit")
	}
	args = append(args, commit)

	_, err = e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Revert completed successfully", nil
}

func (e *GitExecutor) registerGitCherryPick() {
	e.engine.RegisterTool("git_cherry_pick",
		"Applies the changes introduced by some existing commits",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Commit(s) to cherry-pick",
				},
				"no_commit": map[string]interface{}{
					"type":        "boolean",
					"description": "Don't auto-commit (default: false)",
				},
				"continue": map[string]interface{}{
					"type":        "boolean",
					"description": "Continue after conflict resolution (default: false)",
				},
				"abort": map[string]interface{}{
					"type":        "boolean",
					"description": "Abort cherry-pick (default: false)",
				},
			},
		}, e.toolGitCherryPick)
}

func (e *GitExecutor) toolGitCherryPick(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"cherry-pick"}

	if getBool(params, "continue", false) {
		args = append(args, "--continue")
		return e.runGitCommand(ctx, args...)
	}
	if getBool(params, "abort", false) {
		args = append(args, "--abort")
		return e.runGitCommand(ctx, args...)
	}

	commit, err := getString(params, "commit", true)
	if err != nil {
		return "", err
	}

	if getBool(params, "no_commit", false) {
		args = append(args, "-n")
	}
	args = append(args, commit)

	_, err = e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	return "Cherry-pick completed successfully", nil
}

// ============== Additional Git Operations ==============

func (e *GitExecutor) registerGitRemote() {
	e.engine.RegisterTool("git_remote",
		"Manages set of tracked repositories",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: list (default), add, remove, rename, set_url, show",
					"enum":        []string{"list", "add", "remove", "rename", "set_url", "show"},
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Remote name (for add/remove/rename/set_url/show)",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Remote URL (for add/set_url)",
				},
				"new_name": map[string]interface{}{
					"type":        "string",
					"description": "New remote name (for rename)",
				},
				"verbose": map[string]interface{}{
					"type":        "boolean",
					"description": "Show detailed information (default: false)",
				},
			},
		}, e.toolGitRemote)
}

func (e *GitExecutor) toolGitRemote(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		args := []string{"remote", "-v"}
		if getBool(params, "verbose", false) {
			// -v already shows verbose info
		}
		return e.runGitCommand(ctx, args...)

	case "show":
		name, _ := getString(params, "name", false)
		if name == "" {
			// Show all remotes if no name specified
			return e.runGitCommand(ctx, "remote", "-v")
		}
		return e.runGitCommand(ctx, "remote", "show", name)

	case "add":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		url, err := getString(params, "url", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "remote", "add", name, url)

	case "remove":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "remote", "remove", name)

	case "rename":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		newName, err := getString(params, "new_name", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "remote", "rename", name, newName)

	case "set_url":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		url, err := getString(params, "url", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "remote", "set-url", name, url)

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (e *GitExecutor) registerGitTag() {
	e.engine.RegisterTool("git_tag",
		"Creates, lists, deletes or verifies a tag object",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: list (default), create, delete",
					"enum":        []string{"list", "create", "delete"},
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Tag name (for create/delete)",
				},
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Commit to tag (for create, default: HEAD)",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Tag message (for create, creates annotated tag)",
				},
				"annotate": map[string]interface{}{
					"type":        "boolean",
					"description": "Create an annotated tag (default: false)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Force tag creation/replacement (default: false)",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Pattern to filter tags (for list)",
				},
				"contains": map[string]interface{}{
					"type":        "string",
					"description": "List tags with specific commit (for list)",
				},
			},
		}, e.toolGitTag)
}

func (e *GitExecutor) toolGitTag(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		args := []string{"tag"}
		if getBool(params, "annotate", false) {
			args = append(args, "-n") // Show annotation
		}
		if pattern, _ := getString(params, "pattern", false); pattern != "" {
			args = append(args, "--list", pattern)
		}
		if contains, _ := getString(params, "contains", false); contains != "" {
			args = append(args, "--contains", contains)
		}
		return e.runGitCommand(ctx, args...)

	case "create":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}

		args := []string{"tag"}
		if getBool(params, "force", false) {
			args = append(args, "--force")
		}
		if message, _ := getString(params, "message", false); message != "" {
			args = append(args, "-a", name, "-m", message)
		} else if getBool(params, "annotate", false) {
			args = append(args, "-a", name)
		} else {
			args = append(args, name)
		}

		if commit, _ := getString(params, "commit", false); commit != "" {
			args = append(args, commit)
		}

		return e.runGitCommand(ctx, args...)

	case "delete":
		name, err := getString(params, "name", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "tag", "-d", name)

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (e *GitExecutor) registerGitReflog() {
	e.engine.RegisterTool("git_reflog",
		"Manages reflog information (useful for recovering lost commits)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ref": map[string]interface{}{
					"type":        "string",
					"description": "Reference to show reflog for (default: HEAD)",
				},
				"max_count": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of entries to show (default: 20)",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "Show reflogs of all references (default: false)",
				},
				"expire": map[string]interface{}{
					"type":        "string",
					"description": "Prune older entries (e.g., '30 days', '2 weeks')",
				},
				"expire_unreachable": map[string]interface{}{
					"type":        "string",
					"description": "Prune unreachable older entries (e.g., 'now')",
				},
			},
		}, e.toolGitReflog)
}

func (e *GitExecutor) toolGitReflog(ctx context.Context, params map[string]interface{}) (string, error) {
	ref, _ := getString(params, "ref", false)
	if ref == "" {
		ref = "HEAD"
	}

	maxCount, _ := getInt(params, "max_count", false, 20)

	args := []string{"reflog"}
	if expire, _ := getString(params, "expire", false); expire != "" {
		args = append(args, "expire", "--expire="+expire)
		if expireUnreachable, _ := getString(params, "expire_unreachable", false); expireUnreachable != "" {
			args = append(args, "--expire-unreachable="+expireUnreachable)
		}
		return e.runGitCommand(ctx, args...)
	}

	args = append(args, "show")
	if getBool(params, "all", false) {
		args = append(args, "--all")
	}
	args = append(args, fmt.Sprintf("-%d", maxCount), ref)

	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitBlame() {
	e.engine.RegisterTool("git_blame",
		"Shows what revision and author last modified each line of a file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{
					"type":        "string",
					"description": "File to blame",
				},
				"line": map[string]interface{}{
					"type":        "integer",
					"description": "Annotate only the specified line",
				},
				"range": map[string]interface{}{
					"type":        "string",
					"description": "Annotate only the specified line range (e.g., '5,10')",
				},
				"show_email": map[string]interface{}{
					"type":        "boolean",
					"description": "Show author email instead of name (default: false)",
				},
				"date_format": map[string]interface{}{
					"type":        "string",
					"description": "Date format: relative, default, iso, local",
				},
				"ignore_whitespace": map[string]interface{}{
					"type":        "boolean",
					"description": "Ignore whitespace when comparing (default: false)",
				},
				"minimal": map[string]interface{}{
					"type":        "boolean",
					"description": "Show minimal blame info (default: false)",
				},
			},
		}, e.toolGitBlame)
}

func (e *GitExecutor) toolGitBlame(ctx context.Context, params map[string]interface{}) (string, error) {
	file, err := getString(params, "file", true)
	if err != nil {
		return "", err
	}

	args := []string{"blame"}

	if getBool(params, "show_email", false) {
		args = append(args, "-e")
	}

	if getBool(params, "minimal", false) {
		args = append(args, "-w")
	}

	if getBool(params, "ignore_whitespace", false) {
		args = append(args, "-w")
	}

	if dateFormat, _ := getString(params, "date_format", false); dateFormat != "" {
		args = append(args, fmt.Sprintf("--date=%s", dateFormat))
	}

	if line, _ := getInt(params, "line", false, 0); line > 0 {
		args = append(args, fmt.Sprintf("-L %d", line))
	}

	if lineRange, _ := getString(params, "range", false); lineRange != "" {
		args = append(args, fmt.Sprintf("-L %s", lineRange))
	}

	args = append(args, "--", file)

	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitClean() {
	e.engine.RegisterTool("git_clean",
		"Removes untracked files from the working tree",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Show what would be deleted without actually deleting (default: true)",
				},
				"force": map[string]interface{}{
					"type":        "boolean",
					"description": "Actually delete files (default: false)",
				},
				"directories": map[string]interface{}{
					"type":        "boolean",
					"description": "Remove untracked directories (default: false)",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Specific path(s) to clean",
				},
				"ignored": map[string]interface{}{
					"type":        "boolean",
					"description": "Remove ignored files as well (default: false)",
				},
				"exclude": map[string]interface{}{
					"type":        "string",
					"description": "Pattern to exclude from cleaning",
				},
				"quiet": map[string]interface{}{
					"type":        "boolean",
					"description": "Only report errors (default: false)",
				},
			},
		}, e.toolGitClean)
}

func (e *GitExecutor) toolGitClean(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"clean"}

	// Default to dry run for safety
	dryRun := getBool(params, "dry_run", true)
	force := getBool(params, "force", false)

	if dryRun && !force {
		args = append(args, "-n")
	} else if force {
		args = append(args, "-f")
	}

	if getBool(params, "directories", false) {
		args = append(args, "-d")
	}

	if getBool(params, "ignored", false) {
		args = append(args, "-X")
	}

	if exclude, _ := getString(params, "exclude", false); exclude != "" {
		args = append(args, "-e", exclude)
	}

	if getBool(params, "quiet", false) {
		args = append(args, "-q")
	}

	if path, _ := getString(params, "path", false); path != "" {
		args = append(args, "--", path)
	}

	result, err := e.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	if dryRun && !force {
		return "Dry run mode (no files deleted). Use force=true to actually delete:\n" + result, nil
	}

	return result, nil
}

func (e *GitExecutor) registerGitClone() {
	e.engine.RegisterTool("git_clone",
		"Clones a repository into a new directory",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Repository URL to clone",
				},
				"directory": map[string]interface{}{
					"type":        "string",
					"description": "Directory to clone into (default: uses repository name)",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Specific branch to checkout",
				},
				"depth": map[string]interface{}{
					"type":        "integer",
					"description": "Create a shallow clone with specified depth (1 = latest commit only)",
				},
				"single_branch": map[string]interface{}{
					"type":        "boolean",
					"description": "Clone only the history leading to the tip of a single branch (default: false)",
				},
				"recursive": map[string]interface{}{
					"type":        "boolean",
					"description": "Clone submodules (default: false)",
				},
				"quiet": map[string]interface{}{
					"type":        "boolean",
					"description": "Suppress progress output (default: false)",
				},
			},
		}, e.toolGitClone)
}

func (e *GitExecutor) toolGitClone(ctx context.Context, params map[string]interface{}) (string, error) {
	url, err := getString(params, "url", true)
	if err != nil {
		return "", err
	}

	args := []string{"clone"}

	if getBool(params, "quiet", false) {
		args = append(args, "--quiet")
	}

	if branch, _ := getString(params, "branch", false); branch != "" {
		args = append(args, "--branch", branch)
	}

	if depth, _ := getInt(params, "depth", false, 0); depth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", depth))
	}

	if getBool(params, "single_branch", false) {
		args = append(args, "--single-branch")
	}

	if getBool(params, "recursive", false) {
		args = append(args, "--recursive")
	}

	args = append(args, url)

	directory, _ := getString(params, "directory", false)
	if directory != "" {
		args = append(args, directory)
	}

	// Clone should use the parent working directory, not the repo directory itself
	workDir, err := e.getWorkDir(ctx)
	if err != nil {
		return "", err
	}

	// Clone into the working directory
	timeout := time.Duration(e.validator.GetConfig().Limits.CommandTimeout) * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	// Return success message with clone location
	cloneDir := directory
	if cloneDir == "" {
		// Extract repository name from URL
		parts := strings.Split(strings.TrimSuffix(url, ".git"), "/")
		cloneDir = parts[len(parts)-1]
	}

	return fmt.Sprintf("Repository cloned successfully to: %s", filepath.Join(workDir, cloneDir)), nil
}

// ============== Extra Git Operations ==============

func (e *GitExecutor) registerGitConfig() {
	e.engine.RegisterTool("git_config",
		"Get and set repository or global options",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: get (default), set, unset, list",
					"enum":        []string{"get", "set", "unset", "list"},
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Config key",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "Config value (for set)",
				},
				"scope": map[string]interface{}{
					"type":        "string",
					"description": "local (default), global, system",
					"enum":        []string{"local", "global", "system"},
				},
			},
		}, e.toolGitConfig)
}

func (e *GitExecutor) toolGitConfig(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "get"
	}
	scope, _ := getString(params, "scope", false)
	if scope == "" {
		scope = "local"
	}
	args := []string{"config"}
	if scope == "global" {
		args = append(args, "--global")
	} else if scope == "system" {
		args = append(args, "--system")
	}
	switch action {
	case "get":
		key, _ := getString(params, "key", false)
		if key == "" {
			args = append(args, "--list")
		} else {
			args = append(args, "--get", key)
		}
		return e.runGitCommand(ctx, args...)
	case "set":
		key, err := getString(params, "key", true)
		if err != nil {
			return "", err
		}
		value, err := getString(params, "value", true)
		if err != nil {
			return "", err
		}
		args = append(args, key, value)
		return e.runGitCommand(ctx, args...)
	case "unset":
		key, err := getString(params, "key", true)
		if err != nil {
			return "", err
		}
		args = append(args, "--unset", key)
		return e.runGitCommand(ctx, args...)
	case "list":
		args = append(args, "--list")
		return e.runGitCommand(ctx, args...)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (e *GitExecutor) registerGitApply() {
	e.engine.RegisterTool("git_apply",
		"Applies a patch to files",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"patch": map[string]interface{}{
					"type":        "string",
					"description": "Patch file path",
				},
				"check": map[string]interface{}{
					"type":        "boolean",
					"description": "Check mode (default: false)",
				},
				"stat": map[string]interface{}{
					"type":        "boolean",
					"description": "Show diffstat (default: false)",
				},
			},
		}, e.toolGitApply)
}

func (e *GitExecutor) toolGitApply(ctx context.Context, params map[string]interface{}) (string, error) {
	patch, err := getString(params, "patch", true)
	if err != nil {
		return "", err
	}
	args := []string{"apply"}
	if getBool(params, "check", false) {
		args = append(args, "--check")
	}
	if getBool(params, "stat", false) {
		args = append(args, "--stat")
	}
	workDir, _ := e.getWorkDir(ctx)
	patchPath := filepath.Join(workDir, patch)
	args = append(args, "--", patchPath)
	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitFormatPatch() {
	e.engine.RegisterTool("git_format_patch",
		"Generates patch files",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Revision range",
				},
				"output_dir": map[string]interface{}{
					"type":        "string",
					"description": "Output directory",
				},
			},
		}, e.toolGitFormatPatch)
}

func (e *GitExecutor) toolGitFormatPatch(ctx context.Context, params map[string]interface{}) (string, error) {
	revision, err := getString(params, "revision", true)
	if err != nil {
		return "", err
	}
	args := []string{"format-patch", revision}
	if outputDir, _ := getString(params, "output_dir", false); outputDir != "" {
		args = append(args, "-o", outputDir)
	}
	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitArchive() {
	e.engine.RegisterTool("git_archive",
		"Creates archive of files",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tree": map[string]interface{}{
					"type":        "string",
					"description": "Tree-ish (default: HEAD)",
				},
				"output": map[string]interface{}{
					"type":        "string",
					"description": "Output file",
				},
			},
		}, e.toolGitArchive)
}

func (e *GitExecutor) toolGitArchive(ctx context.Context, params map[string]interface{}) (string, error) {
	tree, _ := getString(params, "tree", false)
	if tree == "" {
		tree = "HEAD"
	}
	output, err := getString(params, "output", true)
	if err != nil {
		return "", err
	}
	return e.runGitCommand(ctx, "archive", "--output="+output, tree)
}

func (e *GitExecutor) registerGitBisect() {
	e.engine.RegisterTool("git_bisect",
		"Binary search to find bug commit",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: start, good, bad, reset",
					"enum":        []string{"start", "good", "bad", "reset"},
				},
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Revision to mark",
				},
			},
		}, e.toolGitBisect)
}

func (e *GitExecutor) toolGitBisect(ctx context.Context, params map[string]interface{}) (string, error) {
	action, err := getString(params, "action", true)
	if err != nil {
		return "", err
	}
	switch action {
	case "start":
		return e.runGitCommand(ctx, "bisect", "start")
	case "good":
		rev, _ := getString(params, "revision", false)
		if rev == "" {
			rev = "HEAD"
		}
		return e.runGitCommand(ctx, "bisect", "good", rev)
	case "bad":
		rev, _ := getString(params, "revision", false)
		if rev == "" {
			rev = "HEAD"
		}
		return e.runGitCommand(ctx, "bisect", "bad", rev)
	case "reset":
		return e.runGitCommand(ctx, "bisect", "reset")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (e *GitExecutor) registerGitGrep() {
	e.engine.RegisterTool("git_grep",
		"Prints lines matching pattern",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Pattern to search",
				},
				"ignore_case": map[string]interface{}{
					"type":        "boolean",
					"description": "Case insensitive",
				},
			},
		}, e.toolGitGrep)
}

func (e *GitExecutor) toolGitGrep(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern, err := getString(params, "pattern", true)
	if err != nil {
		return "", err
	}
	args := []string{"grep", "-n"}
	if getBool(params, "ignore_case", false) {
		args = append(args, "-i")
	}
	args = append(args, "--", pattern)
	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitShortlog() {
	e.engine.RegisterTool("git_shortlog",
		"Summarizes git log",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Revision range",
				},
			},
		}, e.toolGitShortlog)
}

func (e *GitExecutor) toolGitShortlog(ctx context.Context, params map[string]interface{}) (string, error) {
	args := []string{"shortlog", "-sn"}
	if revision, _ := getString(params, "revision", false); revision != "" {
		args = append(args, revision)
	}
	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitDescribe() {
	e.engine.RegisterTool("git_describe",
		"Shows most recent tag",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"commit": map[string]interface{}{
					"type":        "string",
					"description": "Commit (default: HEAD)",
				},
				"tags": map[string]interface{}{
					"type":        "boolean",
					"description": "Use any tag",
				},
			},
		}, e.toolGitDescribe)
}

func (e *GitExecutor) toolGitDescribe(ctx context.Context, params map[string]interface{}) (string, error) {
	commit, _ := getString(params, "commit", false)
	if commit == "" {
		commit = "HEAD"
	}
	args := []string{"describe"}
	if getBool(params, "tags", false) {
		args = append(args, "--tags")
	}
	args = append(args, commit)
	return e.runGitCommand(ctx, args...)
}

func (e *GitExecutor) registerGitSubmodule() {
	e.engine.RegisterTool("git_submodule",
		"Manages submodules",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: list, add, update, init",
					"enum":        []string{"list", "add", "update", "init"},
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Submodule path",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "URL (for add)",
				},
			},
		}, e.toolGitSubmodule)
}

func (e *GitExecutor) toolGitSubmodule(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := getString(params, "action", false)
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		return e.runGitCommand(ctx, "submodule", "status")
	case "add":
		url, err := getString(params, "url", true)
		if err != nil {
			return "", err
		}
		path, err := getString(params, "path", true)
		if err != nil {
			return "", err
		}
		return e.runGitCommand(ctx, "submodule", "add", url, path)
	case "update":
		return e.runGitCommand(ctx, "submodule", "update", "--init", "--recursive")
	case "init":
		return e.runGitCommand(ctx, "submodule", "init")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}
