package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// TokenUsageRepository handles token usage data operations
type TokenUsageRepository struct {
	db *gorm.DB
}

// NewTokenUsageRepository creates a new token usage repository
func NewTokenUsageRepository(db *gorm.DB) *TokenUsageRepository {
	return &TokenUsageRepository{db: db}
}

// Create creates a new token usage record
func (r *TokenUsageRepository) Create(ctx context.Context, usage *model.TokenUsage) error {
	return r.db.WithContext(ctx).Create(usage).Error
}

// TimeGrouping represents how to group time periods for statistics
type TimeGrouping string

const (
	TimeGroupHour  TimeGrouping = "hour"
	TimeGroupDay   TimeGrouping = "day"
	TimeGroupWeek  TimeGrouping = "week"
	TimeGroupMonth TimeGrouping = "month"
)

// StatsQueryOptions defines options for querying token usage statistics
type StatsQueryOptions struct {
	StartTime *time.Time
	EndTime   *time.Time
	SessionID *string
	Provider  *string
	Model     *string
	Grouping  TimeGrouping
}

// TokenUsageStats represents aggregated token usage statistics
type TokenUsageStats struct {
	Date             string  `json:"date"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	CallType         string  `json:"call_type"`
	PromptTokens     uint    `json:"prompt_tokens"`
	CompletionTokens uint    `json:"completion_tokens"`
	TotalTokens      uint    `json:"total_tokens"`
	EstimatedCost    float64 `json:"estimated_cost"`
	CallCount        uint    `json:"call_count"`
}

// GetStats retrieves aggregated statistics based on the provided options
func (r *TokenUsageRepository) GetStats(ctx context.Context, opts StatsQueryOptions) ([]TokenUsageStats, error) {
	var results []TokenUsageStats

	query := r.db.WithContext(ctx).Model(&model.TokenUsage{})

	// Apply time filters
	if opts.StartTime != nil {
		query = query.Where("created_at >= ?", *opts.StartTime)
	}
	if opts.EndTime != nil {
		query = query.Where("created_at <= ?", *opts.EndTime)
	}

	// Apply optional filters
	if opts.SessionID != nil {
		query = query.Where("session_id = ?", *opts.SessionID)
	}
	if opts.Provider != nil {
		query = query.Where("provider = ?", *opts.Provider)
	}
	if opts.Model != nil {
		query = query.Where("model = ?", *opts.Model)
	}

	// Determine grouping and date format
	var dateFormat string
	switch opts.Grouping {
	case TimeGroupHour:
		dateFormat = "%Y-%m-%d %H:00:00"
	case TimeGroupDay:
		dateFormat = "%Y-%m-%d"
	case TimeGroupWeek:
		dateFormat = "%Y-%u"
	case TimeGroupMonth:
		dateFormat = "%Y-%m"
	default:
		dateFormat = "%Y-%m-%d"
	}

	// Build the aggregation query - call_type must always be in GROUP BY
	groupByCols := "DATE_FORMAT(created_at, '" + dateFormat + "'), call_type"
	if opts.Provider == nil && opts.Model == nil {
		groupByCols += ", provider, model"
	} else if opts.Provider == nil {
		groupByCols += ", provider"
	} else if opts.Model == nil {
		groupByCols += ", model"
	}

	err := query.
		Select(
			"DATE_FORMAT(created_at, '"+dateFormat+"') as date",
			"COALESCE(provider, 'all') as provider",
			"COALESCE(model, 'all') as model",
			"COALESCE(call_type, 'chat') as call_type",
			"SUM(prompt_tokens) as prompt_tokens",
			"SUM(completion_tokens) as completion_tokens",
			"SUM(total_tokens) as total_tokens",
			"SUM(estimated_cost) as estimated_cost",
			"COUNT(*) as call_count",
		).
		Group(groupByCols).
		Order("date ASC, provider ASC, model ASC").
		Scan(&results).Error

	return results, err
}

// GetTotalStats retrieves total statistics (overall summary)
func (r *TokenUsageRepository) GetTotalStats(ctx context.Context, startTime, endTime *time.Time) (*TokenUsageStats, error) {
	var result TokenUsageStats

	query := r.db.WithContext(ctx).Model(&model.TokenUsage{})

	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	err := query.
		Select(
			"COUNT(*) as call_count",
			"SUM(prompt_tokens) as prompt_tokens",
			"SUM(completion_tokens) as completion_tokens",
			"SUM(total_tokens) as total_tokens",
			"SUM(estimated_cost) as estimated_cost",
		).
		Scan(&result).Error

	return &result, err
}

// GetByProvider retrieves stats grouped by provider
func (r *TokenUsageRepository) GetByProvider(ctx context.Context, startTime, endTime *time.Time) ([]TokenUsageStats, error) {
	var results []TokenUsageStats

	query := r.db.WithContext(ctx).Model(&model.TokenUsage{})

	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	err := query.
		Select(
			"provider",
			"SUM(prompt_tokens) as prompt_tokens",
			"SUM(completion_tokens) as completion_tokens",
			"SUM(total_tokens) as total_tokens",
			"SUM(estimated_cost) as estimated_cost",
			"COUNT(*) as call_count",
		).
		Group("provider").
		Order("total_tokens DESC").
		Scan(&results).Error

	return results, err
}

// GetByModel retrieves stats grouped by model
func (r *TokenUsageRepository) GetByModel(ctx context.Context, startTime, endTime *time.Time) ([]TokenUsageStats, error) {
	var results []TokenUsageStats

	query := r.db.WithContext(ctx).Model(&model.TokenUsage{})

	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	err := query.
		Select(
			"provider",
			"model",
			"SUM(prompt_tokens) as prompt_tokens",
			"SUM(completion_tokens) as completion_tokens",
			"SUM(total_tokens) as total_tokens",
			"SUM(estimated_cost) as estimated_cost",
			"COUNT(*) as call_count",
		).
		Group("provider, model").
		Order("total_tokens DESC").
		Scan(&results).Error

	return results, err
}

// GetRecentUsage retrieves recent token usage records
func (r *TokenUsageRepository) GetRecentUsage(ctx context.Context, limit int, sessionID *string) ([]*model.TokenUsage, error) {
	var records []*model.TokenUsage

	query := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit)
	if sessionID != nil {
		query = query.Where("session_id = ?", *sessionID)
	}

	err := query.Find(&records).Error
	return records, err
}

// DeleteBySessionID deletes all token usage records for a session
func (r *TokenUsageRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&model.TokenUsage{}).Error
}
