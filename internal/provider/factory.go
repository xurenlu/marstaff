package provider

import (
	"context"
	"sync"

	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/repository"
)

// ConfigurableProviderFactory creates providers with config merged with DB overrides.
// DB overrides (e.g. api_key from settings) take precedence over config file.
type ConfigurableProviderFactory struct {
	cfg  *config.Config
	repo *repository.ProviderSettingRepository
	get  func(*config.Config, string) map[string]interface{}

	mu      sync.RWMutex
	cache   map[string]Provider
	enabled []string
}

// NewConfigurableProviderFactory creates a factory that merges config with DB overrides
func NewConfigurableProviderFactory(cfg *config.Config, repo *repository.ProviderSettingRepository, getConfig func(*config.Config, string) map[string]interface{}) *ConfigurableProviderFactory {
	return &ConfigurableProviderFactory{
		cfg:     cfg,
		repo:    repo,
		get:     getConfig,
		cache:   make(map[string]Provider),
		enabled: []string{"zai", "qwen", "gemini", "deepseek", "minimax", "minimax_intl", "ollama", "vllm", "poe"},
	}
}

// GetProvider returns a provider by name. Merges config file + DB overrides (DB wins).
func (f *ConfigurableProviderFactory) GetProvider(ctx context.Context, name string) (Provider, bool) {
	base := f.get(f.cfg, name)
	if base == nil {
		return nil, false
	}
	merged := make(map[string]interface{})
	for k, v := range base {
		merged[k] = v
	}
	if f.repo != nil {
		overrides, err := f.repo.GetByProvider(ctx, name)
		if err == nil {
			for k, v := range overrides {
				merged[k] = v
			}
		}
	}
	prov, err := CreateProvider(name, merged)
	if err != nil {
		return nil, false
	}
	return prov, true
}

// GetProviderCached returns cached provider or creates and caches. Call Invalidate after settings save.
func (f *ConfigurableProviderFactory) GetProviderCached(ctx context.Context, name string) (Provider, bool) {
	f.mu.RLock()
	if p, ok := f.cache[name]; ok {
		f.mu.RUnlock()
		return p, true
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.cache[name]; ok {
		return p, true
	}
	p, ok := f.GetProvider(ctx, name)
	if !ok {
		return nil, false
	}
	f.cache[name] = p
	return p, true
}

// Invalidate clears cache for name (or all if name empty). Call after saving provider settings.
func (f *ConfigurableProviderFactory) Invalidate(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "" {
		f.cache = make(map[string]Provider)
		return
	}
	delete(f.cache, name)
}

// EnabledNames returns provider names to offer in UI
func (f *ConfigurableProviderFactory) EnabledNames() []string {
	return f.enabled
}

// List returns provider names that have base config (for settings UI)
func (f *ConfigurableProviderFactory) List() []string {
	var out []string
	for _, name := range f.enabled {
		if f.get(f.cfg, name) != nil {
			out = append(out, name)
		}
	}
	return out
}
