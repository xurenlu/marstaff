package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// EnvVarRepository handles environment variable settings from the settings UI
type EnvVarRepository struct {
	db *gorm.DB
}

// NewEnvVarRepository creates a new env var repository
func NewEnvVarRepository(db *gorm.DB) *EnvVarRepository {
	return &EnvVarRepository{db: db}
}

// GetAll returns all env vars as key->value map
func (r *EnvVarRepository) GetAll(ctx context.Context) (map[string]string, error) {
	var settings []model.EnvVarSetting
	if err := r.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, s := range settings {
		out[s.Key] = s.Value
	}
	return out, nil
}

// Set sets a single env var (upsert)
func (r *EnvVarRepository) Set(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).
		Where("`key` = ?", key).
		Assign(map[string]interface{}{"value": value}).
		FirstOrCreate(&model.EnvVarSetting{
			Key:   key,
			Value: value,
		}).Error
}

// SetBatch sets multiple env vars, and removes keys not in the new map
func (r *EnvVarRepository) SetBatch(ctx context.Context, vars map[string]string) error {
	existing, err := r.GetAll(ctx)
	if err != nil {
		return err
	}

	// Delete keys that exist in DB but not in new vars
	for k := range existing {
		if _, ok := vars[k]; !ok {
			if err := r.Delete(ctx, k); err != nil {
				return err
			}
		}
	}

	// Upsert new/updated vars
	for k, v := range vars {
		if err := r.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an env var by key
func (r *EnvVarRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).
		Where("`key` = ?", key).
		Delete(&model.EnvVarSetting{}).Error
}
