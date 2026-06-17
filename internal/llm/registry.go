package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ProviderConfig is the serializable config for a single LLM provider.
// Used for runtime updates and persistence.
type ProviderConfig struct {
	BaseURL      string        `json:"base_url"                yaml:"base_url"`
	Model        string        `json:"model"                   yaml:"model"`
	APIKey       string        `json:"api_key,omitempty"       yaml:"api_key"`
	Timeout      time.Duration `json:"timeout,omitempty"       yaml:"timeout"`
	SystemPrompt string        `json:"system_prompt,omitempty" yaml:"system_prompt"`
}

// RegistryConfig holds the full LLM configuration for persistence.
type RegistryConfig struct {
	Providers  map[string]ProviderConfig `json:"providers"          yaml:"providers"`
	BatchSize  int                       `json:"batch_size"         yaml:"batch_size"`
	MaxContext int                       `json:"max_context_tokens" yaml:"max_context_tokens"`
	MaxTokens  int                       `json:"max_tokens"         yaml:"max_tokens"`
	Interval   time.Duration             `json:"interval"           yaml:"interval"`
	Timeout    time.Duration             `json:"timeout"            yaml:"timeout"`
}

// ProviderFactory creates a Provider from a ProviderConfig.
type ProviderFactory func(name string, cfg ProviderConfig) Provider

// Registry holds the current LLM providers and configuration.
// It supports live updates and graceful persistence on shutdown.
type Registry struct {
	mu         sync.RWMutex
	providers  map[string]Provider
	config     RegistryConfig
	factory    ProviderFactory
	configPath string // path to the config file for persistence
}

// NewRegistry creates a new provider registry with the given initial config and factory.
func NewRegistry(cfg RegistryConfig, factory ProviderFactory, configPath string) *Registry {
	r := &Registry{
		config:     cfg,
		factory:    factory,
		configPath: configPath,
		providers:  make(map[string]Provider, len(cfg.Providers)),
	}
	for name, pCfg := range cfg.Providers {
		r.providers[name] = factory(name, pCfg)
	}

	return r
}

// Get returns the named provider, or nil if not found.
func (r *Registry) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.providers[name]
}

// All returns a snapshot of all providers.
func (r *Registry) All() map[string]Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Provider, len(r.providers))
	for k, v := range r.providers {
		result[k] = v
	}

	return result
}

// Config returns the current configuration snapshot.
func (r *Registry) Config() RegistryConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.config
}

// Update replaces providers from a new config.
// Existing providers not in the new config are removed.
// New providers are created via the factory.
// Any evicted provider that implements Drainer is drained before being discarded.
func (r *Registry) Update(cfg RegistryConfig) {
	r.mu.Lock()
	old := r.providers

	newProviders := make(map[string]Provider, len(cfg.Providers))
	for name, pCfg := range cfg.Providers {
		newProviders[name] = r.factory(name, pCfg)
	}

	r.providers = newProviders
	r.config = cfg
	r.mu.Unlock()

	for _, prov := range old {
		if d, ok := prov.(Drainer); ok {
			d.Drain()
		}
	}
}

// Flush writes the current LLM config to the config file on disk.
// Called during graceful shutdown to persist runtime changes.
func (r *Registry) Flush() error {
	if r.configPath == "" {
		return nil // no config file path — skip persistence
	}

	r.mu.RLock()
	cfg := r.config
	r.mu.RUnlock()

	// Read existing config file if it exists.
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(r.configPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// Merge LLM config into the existing structure.
	classifierSection, _ := existing["classifier"].(map[string]interface{})
	if classifierSection == nil {
		classifierSection = make(map[string]interface{})
	}

	classifierSection["llm"] = cfg
	existing["classifier"] = classifierSection

	// Write back.
	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("llm registry: marshal config: %w", err)
	}

	if err := os.WriteFile(r.configPath, data, 0o644); err != nil {
		return fmt.Errorf("llm registry: write config: %w", err)
	}

	return nil
}

// ToJSON returns the current config as JSON bytes (for API responses or logging).
func (r *Registry) ToJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return json.Marshal(r.config)
}
