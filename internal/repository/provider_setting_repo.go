package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// ProviderSettingRepository handles provider config overrides (e.g. API keys from settings)
type ProviderSettingRepository struct {
	db *gorm.DB
}

// NewProviderSettingRepository creates a new provider setting repository
func NewProviderSettingRepository(db *gorm.DB) *ProviderSettingRepository {
	return &ProviderSettingRepository{db: db}
}

// GetByProvider returns all overrides for a provider as key->value map
func (r *ProviderSettingRepository) GetByProvider(ctx context.Context, provider string) (map[string]string, error) {
	var settings []model.ProviderSetting
	if err := r.db.WithContext(ctx).
		Where("provider = ?", provider).
		Find(&settings).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, s := range settings {
		out[s.Key] = s.Value
	}
	return out, nil
}

// GetAll returns overrides for all providers: provider -> key -> value
func (r *ProviderSettingRepository) GetAll(ctx context.Context) (map[string]map[string]string, error) {
	var settings []model.ProviderSetting
	if err := r.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, err
	}
	out := make(map[string]map[string]string)
	for _, s := range settings {
		if out[s.Provider] == nil {
			out[s.Provider] = make(map[string]string)
		}
		out[s.Provider][s.Key] = s.Value
	}
	return out, nil
}

// Set sets a single override (upsert)
func (r *ProviderSettingRepository) Set(ctx context.Context, provider, key, value string) error {
	return r.db.WithContext(ctx).
		Where("provider = ? AND `key` = ?", provider, key).
		Assign(map[string]interface{}{"value": value}).
		FirstOrCreate(&model.ProviderSetting{
			Provider: provider,
			Key:      key,
			Value:    value,
		}).Error
}

// SetBatch sets multiple overrides for a provider
func (r *ProviderSettingRepository) SetBatch(ctx context.Context, provider string, overrides map[string]string) error {
	for k, v := range overrides {
		if err := r.Set(ctx, provider, k, v); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an override
func (r *ProviderSettingRepository) Delete(ctx context.Context, provider, key string) error {
	return r.db.WithContext(ctx).
		Where("provider = ? AND `key` = ?", provider, key).
		Delete(&model.ProviderSetting{}).Error
}
