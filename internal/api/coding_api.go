package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/coding"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// CodingAPI handles coding/iteration API endpoints
type CodingAPI struct {
	db               *gorm.DB
	branchManager    *coding.BranchManager
	iterationEngine  *coding.IterationEngine
	branchRepo       *repository.BranchRepository
	iterationRepo    *repository.IterationRepository
	taskRepo         *repository.TaskRepository
	commitRepo       *repository.CommitRepository
	statsRepo        *repository.CodingStatsRepository
	projectRepo      *repository.ProjectRepository
}

// NewCodingAPI creates a new coding API instance
func NewCodingAPI(db *gorm.DB) *CodingAPI {
	return &CodingAPI{
		db:              db,
		branchManager:   coding.NewBranchManager(db),
		branchRepo:      repository.NewBranchRepository(db),
		iterationRepo:   repository.NewIterationRepository(db),
		taskRepo:        repository.NewTaskRepository(db),
		commitRepo:      repository.NewCommitRepository(db),
		statsRepo:       repository.NewCodingStatsRepository(db),
		projectRepo:     repository.NewProjectRepository(db),
	}
}

// SetIterationEngine sets the iteration engine (must be called after engine is created)
func (api *CodingAPI) SetIterationEngine(engine *coding.IterationEngine) {
	api.iterationEngine = engine
}

// RegisterRoutes registers all coding API routes
func (api *CodingAPI) RegisterRoutes(r *gin.RouterGroup) {
	coding := r.Group("/coding")
	{
		// Branch management
		coding.POST("/branches", api.CreateBranch)
		coding.GET("/branches", api.ListBranches)
		coding.GET("/branches/:id", api.GetBranch)
		coding.PUT("/branches/:id/start", api.StartBranch)
		coding.PUT("/branches/:id/merge", api.MergeBranch)
		coding.DELETE("/branches/:id", api.DeleteBranch)
		coding.GET("/branches/:id/iterations", api.GetBranchIterations)
		coding.GET("/branches/:id/commits", api.GetBranchCommits)

		// Iteration management
		coding.POST("/iterations/start", api.StartIteration)
		coding.GET("/iterations", api.ListIterations)
		coding.GET("/iterations/:id", api.GetIteration)

		// Task management
		coding.POST("/tasks", api.CreateTask)
		coding.GET("/tasks", api.ListTasks)
		coding.PUT("/tasks/:id", api.UpdateTask)
		coding.DELETE("/tasks/:id", api.DeleteTask)

		// Commit history
		coding.GET("/commits", api.ListCommits)

		// Statistics
		coding.GET("/stats", api.GetStats)
		coding.GET("/stats/daily", api.GetDailyStats)

		// Status
		coding.GET("/status", api.GetStatus)

		// Project settings
		coding.PUT("/projects/:id/settings", api.UpdateProjectSettings)
	}
}

// CreateBranch creates a new feature branch
func (api *CodingAPI) CreateBranch(c *gin.Context) {
	var req coding.CreateFeatureBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	branch, err := api.branchManager.CreateFeatureBranch(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, branch)
}

// ListBranches lists all branches with optional filtering
func (api *CodingAPI) ListBranches(c *gin.Context) {
	projectID := c.Query("project_id")
	sessionID := c.Query("session_id")
	status := c.Query("status")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	branches, err := api.branchRepo.List(c.Request.Context(), &model.BranchListOptions{
		ProjectID: projectID,
		SessionID: sessionID,
		Status:    status,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"branches": branches})
}

// GetBranch retrieves a single branch
func (api *CodingAPI) GetBranch(c *gin.Context) {
	id := c.Param("id")

	branch, err := api.branchRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "branch not found"})
		return
	}

	c.JSON(http.StatusOK, branch)
}

// StartBranch starts development on a branch
func (api *CodingAPI) StartBranch(c *gin.Context) {
	id := c.Param("id")

	if err := api.branchManager.StartDevelopment(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

// MergeBranch merges a branch to its parent
func (api *CodingAPI) MergeBranch(c *gin.Context) {
	id := c.Param("id")

	commit, err := api.branchManager.MergeToParent(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, commit)
}

// DeleteBranch deletes a branch
func (api *CodingAPI) DeleteBranch(c *gin.Context) {
	id := c.Param("id")

	if err := api.branchRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// StartIteration starts continuous AI iteration
func (api *CodingAPI) StartIteration(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		SessionID string `json:"session_id,omitempty"`
		Goal      string `json:"goal" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if api.iterationEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "iteration engine not available"})
		return
	}

	if err := api.iterationEngine.StartContinuousIteration(c.Request.Context(), req.ProjectID, req.SessionID, req.Goal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "started", "message": "AI iteration started"})
}

// ListIterations lists iterations with filtering
func (api *CodingAPI) ListIterations(c *gin.Context) {
	projectID := c.Query("project_id")
	branchID := c.Query("branch_id")
	sessionID := c.Query("session_id")
	iterType := c.Query("type")
	status := c.Query("status")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	iterations, err := api.iterationRepo.List(c.Request.Context(), &model.IterationListOptions{
		ProjectID: projectID,
		BranchID:  branchID,
		SessionID: sessionID,
		Type:      iterType,
		Status:    status,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"iterations": iterations})
}

// GetIteration retrieves a single iteration
func (api *CodingAPI) GetIteration(c *gin.Context) {
	id := c.Param("id")

	iteration, err := api.iterationRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "iteration not found"})
		return
	}

	c.JSON(http.StatusOK, iteration)
}

// GetBranchIterations lists iterations for a specific branch
func (api *CodingAPI) GetBranchIterations(c *gin.Context) {
	branchID := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	iterations, err := api.iterationRepo.List(c.Request.Context(), &model.IterationListOptions{
		BranchID: branchID,
		Limit:    limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"iterations": iterations})
}

// CreateTask creates a new task
func (api *CodingAPI) CreateTask(c *gin.Context) {
	var req model.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := &model.Task{
		ProjectID:   req.ProjectID,
		BranchID:    req.BranchID,
		ParentID:    req.ParentID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Complexity:  req.Complexity,
		Status:      "todo",
	}

	// Handle tags
	if len(req.Tags) > 0 {
		task.Tags = formatTagsJSON(req.Tags)
	}

	if err := api.taskRepo.Create(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)
}

// ListTasks lists tasks with filtering
func (api *CodingAPI) ListTasks(c *gin.Context) {
	projectID := c.Query("project_id")
	branchID := c.Query("branch_id")
	status := c.Query("status")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	tasks, err := api.taskRepo.List(c.Request.Context(), projectID, branchID, status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// UpdateTask updates a task
func (api *CodingAPI) UpdateTask(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Status   *string   `json:"status"`
		Progress *float64  `json:"progress"`
		Title    *string   `json:"title"`
		Priority *int      `json:"priority"`
		Tags     []string  `json:"tags"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := api.taskRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	if req.Status != nil {
		task.Status = *req.Status
	}
	if req.Progress != nil {
		task.Progress = *req.Progress
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if len(req.Tags) > 0 {
		task.Tags = formatTagsJSON(req.Tags)
	}

	if err := api.taskRepo.Update(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// DeleteTask deletes a task
func (api *CodingAPI) DeleteTask(c *gin.Context) {
	id := c.Param("id")

	// Soft delete by updating status to deleted
	if err := api.taskRepo.UpdateStatus(c.Request.Context(), id, "deleted"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ListCommits lists commits with filtering
func (api *CodingAPI) ListCommits(c *gin.Context) {
	projectID := c.Query("project_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	commits, err := api.commitRepo.List(c.Request.Context(), projectID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"commits": commits})
}

// GetBranchCommits lists commits for a specific branch
func (api *CodingAPI) GetBranchCommits(c *gin.Context) {
	branchID := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	commits, err := api.commitRepo.GetByBranchID(c.Request.Context(), branchID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"commits": commits})
}

// GetStats retrieves coding statistics
func (api *CodingAPI) GetStats(c *gin.Context) {
	projectID := c.Query("project_id")
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}

	stats, err := api.statsRepo.GetByProject(c.Request.Context(), projectID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate totals
	totalIterations := 0
	totalCommits := 0
	totalBranches := 0
	mergedBranches := 0

	for _, s := range stats {
		totalIterations += s.TotalIterations
		totalCommits += s.TotalCommits
		totalBranches += s.TotalBranches
		mergedBranches += s.MergedBranches
	}

	c.JSON(http.StatusOK, gin.H{
		"stats":          stats,
		"totals": gin.H{
			"iterations":      totalIterations,
			"commits":         totalCommits,
			"branches":        totalBranches,
			"merged_branches": mergedBranches,
		},
	})
}

// GetDailyStats retrieves today's statistics
func (api *CodingAPI) GetDailyStats(c *gin.Context) {
	projectID := c.Query("project_id")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}

	stats, err := api.statsRepo.GetOrCreateTodayStats(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetStatus retrieves the current status of the coding system
func (api *CodingAPI) GetStatus(c *gin.Context) {
	projectID := c.Query("project_id")

	var status gin.H

	if projectID != "" {
		// Get project-specific status
		project, err := api.projectRepo.GetByID(c.Request.Context(), projectID)
		if err == nil {
			// Get active branches
			activeBranches, _ := api.branchRepo.GetActiveBranches(c.Request.Context(), projectID)

			// Get today's stats
			stats, _ := api.statsRepo.GetOrCreateTodayStats(c.Request.Context(), projectID)

			status = gin.H{
				"project":         project,
				"active_branches": len(activeBranches),
				"today_stats":     stats,
				"timestamp":       time.Now(),
			}
		}
	} else {
		// Get overall status
		status = gin.H{
			"status":    "running",
			"timestamp": time.Now(),
		}
	}

	c.JSON(http.StatusOK, status)
}

// UpdateProjectSettings updates project coding settings
func (api *CodingAPI) UpdateProjectSettings(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		MaxConcurrentBranches *int `json:"max_concurrent_branches,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	project, err := api.projectRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if req.MaxConcurrentBranches != nil {
		if *req.MaxConcurrentBranches < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_concurrent_branches must be at least 1"})
			return
		}
		if *req.MaxConcurrentBranches > 20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_concurrent_branches cannot exceed 20"})
			return
		}
		project.MaxConcurrentBranches = *req.MaxConcurrentBranches
	}

	if err := api.projectRepo.Update(c.Request.Context(), project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, project)
}

// Helper function to format tags as JSON
func formatTagsJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	var result string
	result = "["
	for i, tag := range tags {
		if i > 0 {
			result += ","
		}
		result += `"` + tag + `"`
	}
	result += "]"
	return result
}
