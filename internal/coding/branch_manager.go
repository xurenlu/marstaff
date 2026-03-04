package coding

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// BranchManager handles git branch operations for feature development
type BranchManager struct {
	db          *gorm.DB
	branchRepo  *repository.BranchRepository
	commitRepo  *repository.CommitRepository
	projectRepo *repository.ProjectRepository
}

// NewBranchManager creates a new branch manager
func NewBranchManager(db *gorm.DB) *BranchManager {
	return &BranchManager{
		db:          db,
		branchRepo:  repository.NewBranchRepository(db),
		commitRepo:  repository.NewCommitRepository(db),
		projectRepo: repository.NewProjectRepository(db),
	}
}

// runGitCommand executes a git command in the specified directory
func (bm *BranchManager) runGitCommand(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), fmt.Errorf("%s", string(output)))
	}

	return string(output), nil
}

// CreateFeatureBranch creates a new feature branch for development
func (bm *BranchManager) CreateFeatureBranch(ctx context.Context, req *CreateFeatureBranchRequest) (*model.FeatureBranch, error) {
	// Get project to validate and get work directory
	project, err := bm.projectRepo.GetByID(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Default parent branch to "develop" if not specified
	parentBranch := req.ParentBranch
	if parentBranch == "" {
		parentBranch = "develop"
	}

	// Generate branch name if not provided
	branchName := req.Name
	if branchName == "" {
		branchName = fmt.Sprintf("feature/%s", generateBranchSlug(req.Description))
	}

	// Check if branch already exists in database
	existing, _ := bm.branchRepo.GetByName(ctx, req.ProjectID, branchName)
	if existing != nil {
		return nil, fmt.Errorf("branch already exists: %s", branchName)
	}

	// Check concurrent branch limit
	activeBranches, err := bm.branchRepo.GetActiveBranches(ctx, req.ProjectID)
	if err == nil && len(activeBranches) >= project.MaxConcurrentBranches {
		// Try to find branches that can be auto-merged (completed but not merged)
		for _, b := range activeBranches {
			if b.Progress >= 100 && b.Status != "merged" {
				log.Info().Str("branch", b.Name).Msg("auto-merging completed branch to free up slot")
				_, _ = bm.MergeToParent(ctx, b.ID)
			}
		}
		// Re-check after auto-merge attempts
		activeBranches, _ = bm.branchRepo.GetActiveBranches(ctx, req.ProjectID)
		if len(activeBranches) >= project.MaxConcurrentBranches {
			return nil, fmt.Errorf("maximum concurrent branches (%d) reached. Current active branches: %d. Please wait for some branches to complete or merge them manually.",
				project.MaxConcurrentBranches, len(activeBranches))
		}
	}

	// Ensure we're on the parent branch
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", parentBranch); err != nil {
		// Try to create develop branch if it doesn't exist
		if parentBranch == "develop" {
			if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", "-b", "develop"); err != nil {
				return nil, fmt.Errorf("failed to create develop branch: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to checkout parent branch %s: %w", parentBranch, err)
		}
	}

	// Pull latest changes from parent branch
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "pull"); err != nil {
		log.Warn().Err(err).Msg("failed to pull latest changes, continuing anyway")
	}

	// Create and checkout new feature branch
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", "-b", branchName); err != nil {
		return nil, fmt.Errorf("failed to create feature branch: %w", err)
	}

	// Create branch record in database
	branch := &model.FeatureBranch{
		ProjectID:    req.ProjectID,
		SessionID:    req.SessionID,
		Name:         branchName,
		ParentBranch: parentBranch,
		Description:  req.Description,
		Status:       "planning",
		Complexity:   req.Complexity,
		Metadata:     fmt.Sprintf(`{"tasks": %d}`, len(req.InitialTasks)),
	}

	if err := bm.branchRepo.Create(ctx, branch); err != nil {
		return nil, fmt.Errorf("failed to create branch record: %w", err)
	}

	log.Info().
		Str("project_id", req.ProjectID).
		Str("branch", branchName).
		Str("parent", parentBranch).
		Msg("created feature branch")

	return branch, nil
}

// StartDevelopment starts development on a feature branch
func (bm *BranchManager) StartDevelopment(ctx context.Context, branchID string) error {
	branch, err := bm.branchRepo.GetByID(ctx, branchID)
	if err != nil {
		return fmt.Errorf("branch not found: %w", err)
	}

	project, err := bm.projectRepo.GetByID(ctx, branch.ProjectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// Checkout the branch
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", branch.Name); err != nil {
		return fmt.Errorf("failed to checkout branch: %w", err)
	}

	// Update branch status
	now := time.Now()
	branch.Status = "active"
	branch.StartedAt = &now

	if err := bm.branchRepo.Update(ctx, branch); err != nil {
		return fmt.Errorf("failed to update branch: %w", err)
	}

	log.Info().
		Str("branch", branch.Name).
		Msg("started development on branch")

	return nil
}

// CommitChanges commits changes to the current branch
func (bm *BranchManager) CommitChanges(ctx context.Context, req *CommitChangesRequest) (*model.Commit, error) {
	branch, err := bm.branchRepo.GetByID(ctx, req.BranchID)
	if err != nil {
		return nil, fmt.Errorf("branch not found: %w", err)
	}

	project, err := bm.projectRepo.GetByID(ctx, branch.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Stage changes
	for _, file := range req.Files {
		if _, err := bm.runGitCommand(ctx, project.WorkDir, "add", file); err != nil {
			log.Warn().Err(err).Str("file", file).Msg("failed to stage file")
		}
	}

	// If no specific files, add all changes
	if len(req.Files) == 0 {
		if _, err := bm.runGitCommand(ctx, project.WorkDir, "add", "-A"); err != nil {
			return nil, fmt.Errorf("failed to stage changes: %w", err)
		}
	}

	// Create commit
	commitMsg := req.Message
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("AI: %s", req.Description)
	}

	output, err := bm.runGitCommand(ctx, project.WorkDir, "commit", "-m", commitMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	// Extract hash from output
	hash := extractCommitHash(output)
	shortHash := hash
	if len(hash) > 7 {
		shortHash = hash[:7]
	}

	// Get diff stats for additions/deletions
	additions, deletions := 0, 0
	if diff, err := bm.runGitCommand(ctx, project.WorkDir, "diff", "--shortstat", "HEAD~1", "HEAD"); err == nil {
		additions, deletions = parseDiffStatsSimple(diff)
	}

	// Create commit record
	commit := &model.Commit{
		BranchID:    req.BranchID,
		ProjectID:   branch.ProjectID,
		Hash:        hash,
		ShortHash:   shortHash,
		Message:     commitMsg,
		Author:      "marstaff-ai",
		Files:       formatFileList(req.Files),
		Additions:   additions,
		Deletions:   deletions,
		IsAutomated: true,
	}

	if err := bm.commitRepo.Create(ctx, commit); err != nil {
		log.Warn().Err(err).Msg("failed to create commit record")
	}

	// Update branch commit count
	branch.CommitCount++
	if err := bm.branchRepo.Update(ctx, branch); err != nil {
		log.Warn().Err(err).Msg("failed to update branch commit count")
	}

	log.Info().
		Str("branch", branch.Name).
		Str("hash", shortHash).
		Str("message", commitMsg).
		Int("additions", additions).
		Int("deletions", deletions).
		Msg("committed changes")

	return commit, nil
}

// MergeToParent merges a feature branch to its parent branch
func (bm *BranchManager) MergeToParent(ctx context.Context, branchID string) (*model.Commit, error) {
	branch, err := bm.branchRepo.GetByID(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("branch not found: %w", err)
	}

	project, err := bm.projectRepo.GetByID(ctx, branch.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Checkout parent branch
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", branch.ParentBranch); err != nil {
		return nil, fmt.Errorf("failed to checkout parent branch: %w", err)
	}

	// Pull latest changes
	if _, err := bm.runGitCommand(ctx, project.WorkDir, "pull"); err != nil {
		log.Warn().Err(err).Msg("failed to pull parent branch, continuing")
	}

	// Attempt merge
	mergeMsg := fmt.Sprintf("Merge branch '%s'", branch.Name)
	hash, shortHash, err := "", "", error(nil)

	output, err := bm.runGitCommand(ctx, project.WorkDir, "merge", "-m", mergeMsg, branch.Name)
	if err != nil {
		// Handle merge conflict - try to resolve automatically
		if strings.Contains(err.Error(), "conflict") || strings.Contains(string(output), "conflict") {
			log.Warn().Msg("merge conflict detected, attempting automatic resolution")

			// Abort current merge attempt
			_, _ = bm.runGitCommand(ctx, project.WorkDir, "merge", "--abort")

			// Try conflict resolution strategy
			hash, shortHash, err = bm.resolveMergeConflict(ctx, project, branch)
			if err != nil {
				branch.Status = "failed"
				_ = bm.branchRepo.Update(ctx, branch)
				return nil, fmt.Errorf("failed to resolve merge conflicts: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to merge branch: %w", err)
		}
	} else {
		hash = extractCommitHash(string(output))
		shortHash = hash
		if len(hash) > 7 {
			shortHash = hash[:7]
		}
	}

	// Create merge commit record
	commit := &model.Commit{
		BranchID:    branchID,
		ProjectID:   branch.ProjectID,
		Hash:        hash,
		ShortHash:   shortHash,
		Message:     mergeMsg,
		Author:      "marstaff-ai",
		IsMerge:     true,
		IsAutomated: true,
	}

	if err := bm.commitRepo.Create(ctx, commit); err != nil {
		log.Warn().Err(err).Msg("failed to create merge commit record")
	}

	// Update branch status
	now := time.Now()
	branch.Status = "merged"
	branch.MergedAt = &now
	branch.Progress = 100
	if err := bm.branchRepo.Update(ctx, branch); err != nil {
		log.Warn().Err(err).Msg("failed to update branch status")
	}

	log.Info().
		Str("branch", branch.Name).
		Str("parent", branch.ParentBranch).
		Str("hash", shortHash).
		Msg("merged feature branch")

	return commit, nil
}

// resolveMergeConflict attempts to automatically resolve merge conflicts
func (bm *BranchManager) resolveMergeConflict(ctx context.Context, project *model.Project, branch *model.FeatureBranch) (hash, shortHash string, err error) {
	// Strategy 1: Use "ours" for non-code files, attempt smart merge for code files
	// Strategy 2: Use AI to analyze and resolve conflicts

	// For now, use a simple strategy: prefer feature branch changes

	// Get conflicted files using git diff --name-only --diff-filter=U
	output, err := bm.runGitCommand(ctx, project.WorkDir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return "", "", fmt.Errorf("failed to get conflicted files: %w", err)
	}

	conflictedFiles := strings.Fields(strings.TrimSpace(output))
	log.Info().Int("conflicts", len(conflictedFiles)).Msg("attempting to resolve conflicts")

	// Mark all conflicts as resolved using the feature branch version
	for _, file := range conflictedFiles {
		ext := strings.ToLower(filepath.Ext(file))

		// For certain file types, prefer incoming (feature branch)
		if contains([]string{".go", ".js", ".ts", ".py", ".java", ".rb", ".php"}, ext) {
			// For code files, use --theirs (feature branch)
			if _, err := bm.runGitCommand(ctx, project.WorkDir, "checkout", "--theirs", file); err != nil {
				log.Warn().Err(err).Str("file", file).Msg("failed to checkout theirs version")
			}
		}

		// Add the resolved file
		if _, err := bm.runGitCommand(ctx, project.WorkDir, "add", file); err != nil {
			log.Warn().Err(err).Str("file", file).Msg("failed to stage resolved file")
		}
	}

	// Complete the merge
	commitOutput, err := bm.runGitCommand(ctx, project.WorkDir, "commit", "-m",
		fmt.Sprintf("Merge branch '%s' (auto-resolved conflicts)", branch.Name))
	if err != nil {
		return "", "", fmt.Errorf("failed to complete merge after conflict resolution: %w", err)
	}

	hash = extractCommitHash(commitOutput)
	shortHash = hash
	if len(hash) > 7 {
		shortHash = hash[:7]
	}

	log.Info().Msg("successfully resolved merge conflicts")

	return hash, shortHash, nil
}

// GetBranchStatus gets the current status of a feature branch
func (bm *BranchManager) GetBranchStatus(ctx context.Context, branchID string) (*BranchStatus, error) {
	branch, err := bm.branchRepo.GetByID(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("branch not found: %w", err)
	}

	project, err := bm.projectRepo.GetByID(ctx, branch.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Get current branch name using git rev-parse
	currentBranch := ""
	if output, err := bm.runGitCommand(ctx, project.WorkDir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		currentBranch = strings.TrimSpace(output)
	}

	// Get git status
	status := ""
	if output, err := bm.runGitCommand(ctx, project.WorkDir, "status", "-sb"); err == nil {
		status = output
	}

	// Get recent commits
	commits, _ := bm.commitRepo.GetByBranchID(ctx, branchID, 10)

	return &BranchStatus{
		Branch:        branch,
		CurrentBranch: currentBranch,
		GitStatus:     status,
		RecentCommits: commits,
		IsBehind:      bm.isBranchBehind(ctx, project.WorkDir, branch.Name, branch.ParentBranch),
	}, nil
}

// CreateFeatureBranchRequest defines the request to create a feature branch
type CreateFeatureBranchRequest struct {
	ProjectID    string   `json:"project_id" binding:"required"`
	SessionID    string   `json:"session_id,omitempty"`
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description" binding:"required"`
	ParentBranch string   `json:"parent_branch,omitempty"`
	Complexity   int      `json:"complexity,omitempty"`
	InitialTasks []string `json:"initial_tasks,omitempty"`
}

// CommitChangesRequest defines the request to commit changes
type CommitChangesRequest struct {
	BranchID    string   `json:"branch_id" binding:"required"`
	Message     string   `json:"message,omitempty"`
	Description string   `json:"description" binding:"required"`
	Files       []string `json:"files,omitempty"`
}

// BranchStatus represents the status of a feature branch
type BranchStatus struct {
	Branch        *model.FeatureBranch
	CurrentBranch string
	GitStatus     string
	RecentCommits []*model.Commit
	IsBehind      bool
}

// Helper functions

func generateBranchSlug(description string) string {
	// Convert description to a URL-friendly branch name
	slug := strings.ToLower(description)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	// Remove special characters
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()
	// Limit length
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return slug
}

func parseDiffStats(diff string) (additions, deletions int) {
	// Simple parser for git diff --numstat output
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			fmt.Sscanf(parts[0], "%d", &additions)
			fmt.Sscanf(parts[1], "%d", &deletions)
		}
	}
	return
}

func formatFileList(files []string) string {
	if len(files) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range files {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%q", f))
	}
	sb.WriteString("]")
	return sb.String()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (bm *BranchManager) isBranchBehind(ctx context.Context, workDir, branch, parent string) bool {
	// Check if branch is behind parent
	_, err := bm.runGitCommand(ctx, workDir,
		"rev-parse", "--short", fmt.Sprintf("%s..%s", parent, branch),
	)
	return err != nil
}

// extractCommitHash extracts commit hash from git command output
func extractCommitHash(output string) string {
	// Git commit output format: [hash abc123...] commit message
	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		// Look for hash in first line
		parts := strings.Fields(lines[0])
		for _, part := range parts {
			if strings.HasPrefix(part, "[") && len(part) > 3 {
				hash := strings.Trim(part, "[]")
				if len(hash) >= 7 {
					return hash
				}
			}
			// Check if it looks like a hash (40 hex chars)
			if len(part) == 40 && isHexHash(part) {
				return part
			}
			// Check if it looks like a short hash
			if len(part) >= 7 && len(part) <= 12 && isHexHash(part) {
				return part
			}
		}
	}
	return "unknown"
}

// isHexHash checks if string is a hexadecimal hash
func isHexHash(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// parseDiffStatsSimple parses git diff --shortstat output
func parseDiffStatsSimple(diff string) (additions, deletions int) {
	// Format: " 5 file changed, 10 insertions(+), 3 deletions(-)"
	lower := strings.ToLower(diff)

	// Parse additions
	if strings.Contains(lower, "insertion") {
		parts := strings.Fields(lower)
		for i, part := range parts {
			if strings.Contains(part, "insertion") && i > 0 {
				fmt.Sscanf(parts[i-1], "%d", &additions)
				break
			}
		}
	}

	// Parse deletions
	if strings.Contains(lower, "deletion") {
		parts := strings.Fields(lower)
		for i, part := range parts {
			if strings.Contains(part, "deletion") && i > 0 {
				fmt.Sscanf(parts[i-1], "%d", &deletions)
				break
			}
		}
	}

	return
}
