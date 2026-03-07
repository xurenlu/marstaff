package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/pipeline"
	"github.com/rocky/marstaff/internal/repository"
)

// PipelineAPI handles pipeline/workflow API endpoints
type PipelineAPI struct {
	pipelineRepo *repository.PipelineRepository
	engine       *pipeline.Engine
}

// NewPipelineAPI creates a new pipeline API
func NewPipelineAPI(db *gorm.DB, engine *pipeline.Engine) *PipelineAPI {
	return &PipelineAPI{
		pipelineRepo: repository.NewPipelineRepository(db),
		engine:       engine,
	}
}

// RegisterRoutes registers pipeline API routes
func (api *PipelineAPI) RegisterRoutes(router *gin.RouterGroup) {
	pipelines := router.Group("/pipelines")
	{
		pipelines.POST("", api.CreatePipeline)
		pipelines.GET("", api.ListPipelines)
		pipelines.GET("/:id", api.GetPipeline)
		pipelines.POST("/:id/execute", api.ExecutePipeline)
		pipelines.POST("/:id/cancel", api.CancelPipeline)
		pipelines.GET("/:id/steps", api.GetPipelineSteps)
	}
}

// CreatePipelineRequest is the request to create a pipeline
type CreatePipelineRequest struct {
	UserID      string            `json:"user_id" binding:"required"`
	SessionID   *string           `json:"session_id"`
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	Definition  model.PipelineDef `json:"definition" binding:"required"`
}

// CreatePipeline creates a new pipeline
func (api *PipelineAPI) CreatePipeline(c *gin.Context) {
	var req CreatePipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pipeReq := &pipeline.CreatePipelineRequest{
		UserID:      req.UserID,
		SessionID:   req.SessionID,
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
	}

	pipeline, err := api.engine.CreatePipeline(c.Request.Context(), pipeReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, pipeline)
}

// ListPipelines lists pipelines for a user
func (api *PipelineAPI) ListPipelines(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID != "" {
		pipelines, err := api.pipelineRepo.GetBySessionID(c.Request.Context(), sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"pipelines": pipelines})
		return
	}

	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id or session_id is required"})
		return
	}

	limit := 50
	pipelines, err := api.pipelineRepo.GetByUserID(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"pipelines": pipelines})
}

// GetPipeline retrieves a pipeline by ID
func (api *PipelineAPI) GetPipeline(c *gin.Context) {
	id := c.Param("id")
	var pipelineID uint
	if _, err := fmt.Sscanf(id, "%d", &pipelineID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline id"})
		return
	}

	pipeline, err := api.pipelineRepo.GetByID(c.Request.Context(), pipelineID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, pipeline)
}

// ExecutePipeline starts or resumes pipeline execution
func (api *PipelineAPI) ExecutePipeline(c *gin.Context) {
	id := c.Param("id")
	var pipelineID uint
	if _, err := fmt.Sscanf(id, "%d", &pipelineID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline id"})
		return
	}

	if err := api.engine.Execute(c.Request.Context(), pipelineID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pipeline, _ := api.pipelineRepo.GetByID(c.Request.Context(), pipelineID)

	c.JSON(http.StatusOK, gin.H{
		"pipeline_id": pipelineID,
		"status":      "running",
		"name":        pipeline.Name,
		"message":     "Pipeline is now running in the background",
	})
}

// CancelPipeline cancels a running pipeline
func (api *PipelineAPI) CancelPipeline(c *gin.Context) {
	id := c.Param("id")
	var pipelineID uint
	if _, err := fmt.Sscanf(id, "%d", &pipelineID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline id"})
		return
	}

	if err := api.engine.Cancel(c.Request.Context(), pipelineID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pipeline_id": pipelineID,
		"status":      "cancelled",
		"message":     "Pipeline cancelled successfully",
	})
}

// GetPipelineSteps retrieves all steps for a pipeline
func (api *PipelineAPI) GetPipelineSteps(c *gin.Context) {
	id := c.Param("id")
	var pipelineID uint
	if _, err := fmt.Sscanf(id, "%d", &pipelineID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline id"})
		return
	}

	steps, err := api.pipelineRepo.GetStepsByPipelineID(c.Request.Context(), pipelineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"steps": steps})
}
