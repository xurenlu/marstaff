package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/repository"
)

// TokenUsageAPI handles token usage statistics
type TokenUsageAPI struct {
	tokenRepo *repository.TokenUsageRepository
}

// NewTokenUsageAPI creates a new token usage API
func NewTokenUsageAPI(db *gorm.DB) *TokenUsageAPI {
	return &TokenUsageAPI{
		tokenRepo: repository.NewTokenUsageRepository(db),
	}
}

// RegisterRoutes registers all routes
func (api *TokenUsageAPI) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/token/stats", api.GetStats)
	router.GET("/token/stats/total", api.GetTotalStats)
	router.GET("/token/stats/by-provider", api.GetByProvider)
	router.GET("/token/stats/by-model", api.GetByModel)
	router.GET("/token/recent", api.GetRecentUsage)
}

// GetStatsRequest is the request for getting token usage statistics
type GetStatsRequest struct {
	StartTime *time.Time `form:"start_time" time_format:"2006-01-02T15:04:05Z07:00" time_utc:"1"`
	EndTime   *time.Time `form:"end_time" time_format:"2006-01-02T15:04:05Z07:00" time_utc:"1"`
	SessionID *string    `form:"session_id"`
	Provider  *string    `form:"provider"`
	Model     *string    `form:"model"`
	Grouping  *string    `form:"grouping"` // hour, day, week, month
}

// GetStats returns token usage statistics grouped by time and optionally by provider/model
func (api *TokenUsageAPI) GetStats(c *gin.Context) {
	var req GetStatsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set default time range to last 7 days if not provided
	if req.StartTime == nil && req.EndTime == nil {
		now := time.Now()
		weekAgo := now.AddDate(0, 0, -7)
		req.StartTime = &weekAgo
		req.EndTime = &now
	}

	// Set default grouping to day
	grouping := repository.TimeGroupDay
	if req.Grouping != nil {
		switch *req.Grouping {
		case "hour":
			grouping = repository.TimeGroupHour
		case "day":
			grouping = repository.TimeGroupDay
		case "week":
			grouping = repository.TimeGroupWeek
		case "month":
			grouping = repository.TimeGroupMonth
		}
	}

	opts := repository.StatsQueryOptions{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		SessionID: req.SessionID,
		Provider:  req.Provider,
		Model:     req.Model,
		Grouping:  grouping,
	}

	stats, err := api.tokenRepo.GetStats(c.Request.Context(), opts)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"stats": stats})
}

// GetTotalStats returns total token usage statistics
func (api *TokenUsageAPI) GetTotalStats(c *gin.Context) {
	var req GetStatsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set default time range to today if not provided
	if req.StartTime == nil && req.EndTime == nil {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		req.StartTime = &startOfDay
		req.EndTime = &now
	}

	stats, err := api.tokenRepo.GetTotalStats(c.Request.Context(), req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, stats)
}

// GetByProvider returns token usage statistics grouped by provider
func (api *TokenUsageAPI) GetByProvider(c *gin.Context) {
	var req GetStatsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set default time range to last 30 days if not provided
	if req.StartTime == nil && req.EndTime == nil {
		now := time.Now()
		monthAgo := now.AddDate(0, 0, -30)
		req.StartTime = &monthAgo
		req.EndTime = &now
	}

	stats, err := api.tokenRepo.GetByProvider(c.Request.Context(), req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"stats": stats})
}

// GetByModel returns token usage statistics grouped by model
func (api *TokenUsageAPI) GetByModel(c *gin.Context) {
	var req GetStatsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set default time range to last 30 days if not provided
	if req.StartTime == nil && req.EndTime == nil {
		now := time.Now()
		monthAgo := now.AddDate(0, 0, -30)
		req.StartTime = &monthAgo
		req.EndTime = &now
	}

	stats, err := api.tokenRepo.GetByModel(c.Request.Context(), req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"stats": stats})
}

// GetRecentUsageRequest is the request for getting recent token usage records
type GetRecentUsageRequest struct {
	Limit     *int    `form:"limit" binding:"omitempty,min=1,max=100"`
	SessionID *string `form:"session_id"`
}

// GetRecentUsage returns recent token usage records
func (api *TokenUsageAPI) GetRecentUsage(c *gin.Context) {
	var req GetRecentUsageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set default limit to 20
	limit := 20
	if req.Limit != nil {
		limit = *req.Limit
	}

	records, err := api.tokenRepo.GetRecentUsage(c.Request.Context(), limit, req.SessionID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"records": records})
}
