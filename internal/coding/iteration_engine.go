package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
)

// IterationEngine handles AI-driven development iterations
type IterationEngine struct {
	db              *gorm.DB
	chatExecutor    ChatExecutor
	branchManager   *BranchManager
	taskRepo        *repository.TaskRepository
	iterationRepo   *repository.IterationRepository
	branchRepo      *repository.BranchRepository
	projectRepo     *repository.ProjectRepository
	statsRepo       *repository.CodingStatsRepository
	config          *IterationConfig
	currentIteration int
}

// ChatExecutor is an interface for executing chat completions
type ChatExecutor interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// ChatRequest represents a request for chat completion
type ChatRequest struct {
	SessionID   string
	UserID      string
	Messages    []provider.Message
	Model       string
	Temperature float64
	PlanMode    bool
}

// ChatResponse represents the response from chat completion
type ChatResponse struct {
	Content      string
	ToolCalls    []provider.ToolCall
	Usage        provider.Usage
	FinishReason string
}

// IterationConfig configures the iteration engine behavior
type IterationConfig struct {
	MaxIterationsPerDay       int           // Maximum iterations per day (default: 500+)
	IterationDelay            time.Duration // Delay between iterations
	AutoBranch                bool          // Automatically create branches for complex tasks
	BranchComplexityThreshold int           // Complexity threshold for creating new branch
	AutoMerge                 bool          // Automatically merge completed branches
	ConflictResolution        string        // "auto" or "manual"
	MaxTokensPerIteration     int           // Safety limit
}

// DefaultIterationConfig returns default configuration
func DefaultIterationConfig() *IterationConfig {
	return &IterationConfig{
		MaxIterationsPerDay:      1000,
		IterationDelay:           2 * time.Second,
		AutoBranch:               true,
		BranchComplexityThreshold: 5,
		AutoMerge:                true,
		ConflictResolution:       "auto",
		MaxTokensPerIteration:    100000,
	}
}

// NewIterationEngine creates a new iteration engine
func NewIterationEngine(db *gorm.DB, chatExecutor ChatExecutor, config *IterationConfig) *IterationEngine {
	if config == nil {
		config = DefaultIterationConfig()
	}

	return &IterationEngine{
		db:            db,
		chatExecutor:  chatExecutor,
		branchManager: NewBranchManager(db),
		taskRepo:      repository.NewTaskRepository(db),
		iterationRepo: repository.NewIterationRepository(db),
		branchRepo:    repository.NewBranchRepository(db),
		projectRepo:   repository.NewProjectRepository(db),
		statsRepo:     repository.NewCodingStatsRepository(db),
		config:        config,
	}
}

// StartContinuousIteration starts continuous AI development
func (ie *IterationEngine) StartContinuousIteration(ctx context.Context, projectID, sessionID string, goal string) error {
	project, err := ie.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	log.Info().
		Str("project", project.Name).
		Str("goal", goal).
		Msg("starting continuous AI iteration")

	// Analyze goal and create initial tasks
	tasks, complexity, err := ie.analyzeAndCreateTasks(ctx, projectID, sessionID, goal)
	if err != nil {
		return fmt.Errorf("failed to analyze goal: %w", err)
	}

	// Create feature branch if complexity is high enough
	var branch *model.FeatureBranch
	if complexity >= ie.config.BranchComplexityThreshold && ie.config.AutoBranch {
		// Check concurrent branch limit first
		activeBranches, _ := ie.branchRepo.GetActiveBranches(ctx, projectID)
		if len(activeBranches) >= project.MaxConcurrentBranches {
			log.Info().
				Int("max_branches", project.MaxConcurrentBranches).
				Int("current_active", len(activeBranches)).
				Msg("concurrent branch limit reached, continuing on current branch")
			// Don't create new branch, work on current one
		} else {
			branch, err = ie.branchManager.CreateFeatureBranch(ctx, &CreateFeatureBranchRequest{
				ProjectID:    projectID,
				SessionID:    sessionID,
				Description:  goal,
				Complexity:   complexity,
				InitialTasks: getTaskDescriptions(tasks),
			})
			if err != nil {
				log.Warn().Err(err).Msg("failed to create feature branch, continuing on current branch")
			} else {
				// Start development
				_ = ie.branchManager.StartDevelopment(ctx, branch.ID)
			}
		}
	}

	// Start iteration loop
	go ie.iterationLoop(ctx, project, sessionID, branch, tasks)

	return nil
}

// iterationLoop runs the continuous development loop
func (ie *IterationEngine) iterationLoop(ctx context.Context, project *model.Project, sessionID string, branch *model.FeatureBranch, tasks []*model.Task) {
	iterationNum := 1

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("iteration loop stopped: context cancelled")
			return
		default:
		}

		// Check daily iteration limit
		todayCount, _ := ie.iterationRepo.GetTodayCount(ctx, project.ID)
		if todayCount >= int64(ie.config.MaxIterationsPerDay) {
			log.Info().Int64("count", todayCount).Msg("daily iteration limit reached, pausing")
			time.Sleep(time.Hour)
			continue
		}

		// Get next task
		task := ie.getNextTask(tasks)
		if task == nil {
			log.Info().Msg("all tasks completed")
			if branch != nil && ie.config.AutoMerge {
				_, _ = ie.branchManager.MergeToParent(ctx, branch.ID)
			}
			return
		}

		// Run iteration
		err := ie.runIteration(ctx, project, sessionID, branch, task, iterationNum)
		if err != nil {
			log.Error().Err(err).Str("task", task.Title).Msg("iteration failed")
			task.Status = "blocked"
			_ = ie.taskRepo.Update(ctx, task)
		}

		iterationNum++

		// Small delay between iterations
		time.Sleep(ie.config.IterationDelay)
	}
}

// runIteration executes a single development iteration
func (ie *IterationEngine) runIteration(ctx context.Context, project *model.Project, sessionID string, branch *model.FeatureBranch, task *model.Task, iterationNum int) error {
	startTime := time.Now()

	// Create iteration record
	iteration := &model.Iteration{
		ProjectID:       project.ID,
		SessionID:       sessionID,
		BranchID:        getBranchID(branch),
		IterationNumber: iterationNum,
		Type:            ie.determineIterationType(task),
		Description:     task.Title,
		Status:          "running",
		StartedAt:       &startTime,
	}

	if err := ie.iterationRepo.Create(ctx, iteration); err != nil {
		return fmt.Errorf("failed to create iteration: %w", err)
	}

	// Update task status
	task.Status = "in_progress"
	if task.StartedAt == nil {
		task.StartedAt = &startTime
	}
	_ = ie.taskRepo.Update(ctx, task)

	// Prepare context for AI
	workDir := project.WorkDir
	if branch != nil {
		// Ensure we're on the correct branch
		_ = ie.branchManager.StartDevelopment(ctx, branch.ID)
	}

	// Build AI prompt for this iteration
	prompt := ie.buildIterationPrompt(ctx, project, task, branch)

	// Execute AI
	response, err := ie.executeAI(ctx, sessionID, prompt, workDir, task)
	if err != nil {
		iteration.Status = "failed"
		iteration.Error = err.Error()
		_ = ie.iterationRepo.Update(ctx, iteration)
		return err
	}

	// Process AI response and create commits
	commits, changes := ie.processAIResponse(ctx, response, branch, project.WorkDir)

	// Update iteration with results
	completedTime := time.Now()
	iteration.Status = "completed"
	iteration.CompletedAt = &completedTime
	iteration.Duration = int(completedTime.Sub(startTime).Milliseconds())
	iteration.InputTokens = response.Usage.PromptTokens
	iteration.OutputTokens = response.Usage.CompletionTokens

	if len(changes) > 0 {
		changesJSON, _ := json.Marshal(changes)
		iteration.Changes = string(changesJSON)
	}
	_ = ie.iterationRepo.Update(ctx, iteration)

	// Update task progress
	task.Progress = calculateTaskProgress(task, commits)
	if task.Progress >= 100 {
		task.Status = "done"
		task.CompletedAt = &completedTime
		task.Progress = 100
	}
	_ = ie.taskRepo.Update(ctx, task)

	// Update branch progress
	if branch != nil {
		branch.Progress = calculateBranchProgress(ctx, branch.ID, ie.taskRepo)
		branch.TaskCount++
		_ = ie.branchRepo.Update(ctx, branch)
	}

	// Update stats
	_ = ie.updateStats(ctx, project.ID, iteration, commits)

	log.Info().
		Str("task", task.Title).
		Str("type", iteration.Type).
		Int("tokens", iteration.InputTokens+iteration.OutputTokens).
		Int("duration_ms", iteration.Duration).
		Int("commits", len(commits)).
		Msg("iteration completed")

	return nil
}

// AIResponse represents the response from AI execution
type AIResponse struct {
	Content    string
	Files      []FileChange
	Usage      provider.Usage
	FinishReason string
}

// FileChange represents a file change made by AI
type FileChange struct {
	Path      string
	Action    string // create, modify, delete
	Content   string
	Additions int
	Deletions int
}

// executeAI executes the AI for this iteration
func (ie *IterationEngine) executeAI(ctx context.Context, sessionID, prompt, workDir string, task *model.Task) (*AIResponse, error) {
	// Create a chat request
	req := ChatRequest{
		SessionID: sessionID,
		UserID:    "default",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		Model:       "",
		Temperature: 0.7,
		PlanMode:    false,
	}

	// Execute chat using the chat executor
	resp, err := ie.chatExecutor.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI execution failed: %w", err)
	}

	response := &AIResponse{
		Content:      resp.Content,
		Usage:        resp.Usage,
		FinishReason: resp.FinishReason,
	}

	// Parse file changes from response
	response.Files = ie.parseFileChanges(resp.Content, workDir)

	return response, nil
}

// parseFileChanges extracts file changes from AI response
func (ie *IterationEngine) parseFileChanges(content, workDir string) []FileChange {
	var files []FileChange

	// Look for code blocks with file paths
	// Format: ```filepath:src/file.go
	lines := strings.Split(content, "\n")
	var currentFile *FileChange
	var codeContent strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// Start of code block
				inCodeBlock = true
				codeContent.Reset()

				// Check for file path
				parts := strings.TrimPrefix(line, "```")
				if strings.HasPrefix(parts, "filepath:") || strings.HasPrefix(parts, "file:") {
					filePath := strings.TrimPrefix(strings.TrimPrefix(parts, "filepath:"), "file:")
					currentFile = &FileChange{
						Path:   strings.TrimSpace(filePath),
						Action: "modify",
					}
				} else if len(parts) > 0 && parts != " " {
					// Language detected, might be a file
					currentFile = &FileChange{
						Path:   fmt.Sprintf("new_file.%s", parts),
						Action: "create",
					}
				}
			} else {
				// End of code block
				inCodeBlock = false
				if currentFile != nil && codeContent.Len() > 0 {
					currentFile.Content = codeContent.String()
					currentFile.Additions = strings.Count(currentFile.Content, "\n") + 1
					files = append(files, *currentFile)
				}
				currentFile = nil
			}
		} else if inCodeBlock && currentFile != nil {
			codeContent.WriteString(line)
			codeContent.WriteString("\n")
		}
	}

	return files
}

// processAIResponse processes the AI response and creates commits
func (ie *IterationEngine) processAIResponse(ctx context.Context, response *AIResponse, branch *model.FeatureBranch, workDir string) ([]*model.Commit, map[string]interface{}) {
	changes := make(map[string]interface{})
	var commits []*model.Commit

	if len(response.Files) > 0 && branch != nil {
		// Commit file changes
		for _, file := range response.Files {
			// Write file (in real implementation)
			// For now, just track the change
			changes[file.Path] = map[string]interface{}{
				"action":     file.Action,
				"additions":  file.Additions,
				"deletions":  file.Deletions,
			}
		}

		// Create commit
		commit, err := ie.branchManager.CommitChanges(ctx, &CommitChangesRequest{
			BranchID:    branch.ID,
			Description: response.Content[:min(len(response.Content), 100)],
			Files:       extractFilePaths(response.Files),
		})

		if err == nil {
			commits = append(commits, commit)
		}
	}

	changes["ai_response_length"] = len(response.Content)
	changes["files_modified"] = len(response.Files)

	return commits, changes
}

// buildIterationPrompt builds the AI prompt for an iteration
func (ie *IterationEngine) buildIterationPrompt(ctx context.Context, project *model.Project, task *model.Task, branch *model.FeatureBranch) string {
	var prompt strings.Builder

	// Context
	prompt.WriteString(fmt.Sprintf("# Context\n"))
	prompt.WriteString(fmt.Sprintf("Project: %s\n", project.Name))
	prompt.WriteString(fmt.Sprintf("Working Directory: %s\n", project.WorkDir))
	prompt.WriteString(fmt.Sprintf("Tech Stack: %s\n", project.TechStack))
	if branch != nil {
		prompt.WriteString(fmt.Sprintf("Current Branch: %s (from %s)\n", branch.Name, branch.ParentBranch))
		prompt.WriteString(fmt.Sprintf("Branch Status: %s (%.0f%% complete)\n\n", branch.Status, branch.Progress))
	}

	// Task
	prompt.WriteString(fmt.Sprintf("# Current Task\n"))
	prompt.WriteString(fmt.Sprintf("Title: %s\n", task.Title))
	if task.Description != "" {
		prompt.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	}
	prompt.WriteString(fmt.Sprintf("Priority: %d/10\n", task.Priority))
	prompt.WriteString(fmt.Sprintf("Complexity: %d/10\n\n", task.Complexity))

	// Instructions
	prompt.WriteString("# Instructions\n")
	prompt.WriteString("You are an AI developer working on this task. Follow these guidelines:\n")
	prompt.WriteString("1. Make small, incremental changes\n")
	prompt.WriteString("2. Write clean, maintainable code\n")
	prompt.WriteString("3. Include comments for complex logic\n")
	prompt.WriteString("4. Follow the project's existing patterns\n")
	prompt.WriteString("5. Return file changes using the format: ```filepath:path/to/file```\n\n")

	// Related context
	prompt.WriteString("# Next Action\n")
	prompt.WriteString(fmt.Sprintf("Please work on: %s\n", task.Title))

	return prompt.String()
}

// analyzeAndCreateTasks analyzes the goal and creates tasks
func (ie *IterationEngine) analyzeAndCreateTasks(ctx context.Context, projectID, sessionID, goal string) ([]*model.Task, int, error) {
	// Use AI to break down goal into tasks
	prompt := fmt.Sprintf(`Break down the following goal into specific development tasks.

Project: %s
Goal: %s

Return a JSON array of tasks with:
- title: brief task title
- description: detailed description
- priority: 1-10 (higher = more important)
- complexity: 1-10 (higher = more complex)

Only return JSON, no other text.`, projectID, goal)

	req := ChatRequest{
		SessionID: sessionID,
		UserID:    "default",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		Model:       "",
		Temperature: 0.5,
	}

	resp, err := ie.chatExecutor.Chat(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	// Parse response
	var tasks []*model.Task
	var taskDescriptions []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		Complexity  int    `json:"complexity"`
	}

	// Extract JSON from response
	jsonStart := strings.Index(resp.Content, "[")
	jsonEnd := strings.LastIndex(resp.Content, "]")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := resp.Content[jsonStart : jsonEnd+1]
		if err := json.Unmarshal([]byte(jsonStr), &taskDescriptions); err == nil {
			totalComplexity := 0
			for i, td := range taskDescriptions {
				task := &model.Task{
					ProjectID:   projectID,
					Title:       td.Title,
					Description: td.Description,
					Priority:    td.Priority,
					Complexity:  td.Complexity,
					Status:      "todo",
				}
				if err := ie.taskRepo.Create(ctx, task); err == nil {
					tasks = append(tasks, task)
					totalComplexity += td.Complexity
				}
				log.Info().Str("task", td.Title).Int("index", i+1).Msg("created task")
			}
			return tasks, totalComplexity / len(tasks), nil
		}
	}

	// Fallback: create single task
	task := &model.Task{
		ProjectID:   projectID,
		Title:       goal,
		Description: "Complete the stated goal",
		Priority:    5,
		Complexity:  5,
		Status:      "todo",
	}
	_ = ie.taskRepo.Create(ctx, task)
	return []*model.Task{task}, 5, nil
}

// determineIterationType determines the type of iteration based on task
func (ie *IterationEngine) determineIterationType(task *model.Task) string {
	if task.Status == "todo" {
		return "planning"
	}
	if strings.Contains(strings.ToLower(task.Title), "test") {
		return "testing"
	}
	if strings.Contains(strings.ToLower(task.Title), "refactor") {
		return "refactoring"
	}
	if strings.Contains(strings.ToLower(task.Title), "fix") || strings.Contains(strings.ToLower(task.Title), "bug") {
		return "debugging"
	}
	return "coding"
}

// Helper functions

func (ie *IterationEngine) getNextTask(tasks []*model.Task) *model.Task {
	for _, task := range tasks {
		if task.Status == "todo" || task.Status == "in_progress" {
			return task
		}
	}
	return nil
}

func getBranchID(branch *model.FeatureBranch) string {
	if branch == nil {
		return ""
	}
	return branch.ID
}

func getTaskDescriptions(tasks []*model.Task) []string {
	var desc []string
	for _, t := range tasks {
		desc = append(desc, t.Title)
	}
	return desc
}

func calculateTaskProgress(task *model.Task, commits []*model.Commit) float64 {
	if len(commits) == 0 {
		return task.Progress
	}
	// Simple progress calculation
	increment := 10.0 * float64(len(commits))
	newProgress := task.Progress + increment
	if newProgress > 100 {
		return 100
	}
	return newProgress
}

func calculateBranchProgress(ctx context.Context, branchID string, taskRepo *repository.TaskRepository) float64 {
	tasks, _ := taskRepo.List(ctx, "", branchID, "", 100)
	if len(tasks) == 0 {
		return 0
	}
	total := 0.0
	for _, t := range tasks {
		total += t.Progress
	}
	return total / float64(len(tasks))
}

func (ie *IterationEngine) updateStats(ctx context.Context, projectID string, iteration *model.Iteration, commits []*model.Commit) error {
	updates := map[string]interface{}{
		"total_iterations": ie.getIncrement("total_iterations"),
		"total_commits":    ie.getIncrement("total_commits") + len(commits),
		"input_tokens":     ie.getIncrement("input_tokens") + iteration.InputTokens,
		"output_tokens":    ie.getIncrement("output_tokens") + iteration.OutputTokens,
	}

	for _, c := range commits {
		updates["files_modified"] = ie.getIncrement("files_modified") + 1
		updates["lines_added"] = ie.getIncrement("lines_added") + c.Additions
		updates["lines_deleted"] = ie.getIncrement("lines_deleted") + c.Deletions
	}

	return ie.statsRepo.Update(ctx, projectID, updates)
}

func (ie *IterationEngine) getIncrement(field string) int {
	return 1 // Simplified
}

func extractFilePaths(files []FileChange) []string {
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
