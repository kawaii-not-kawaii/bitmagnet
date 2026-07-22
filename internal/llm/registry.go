package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
)

// ErrPersistenceDisabled is returned by Flush when the registry has no config
// file path. It is a distinct signal, not a failure: callers that treat
// persistence as optional can check errors.Is(err, ErrPersistenceDisabled) and
// ignore it, while callers that expected a write can tell it apart from a
// silent success. The previous implementation returned nil here, making
// "nothing was written" indistinguishable from "written successfully".
var ErrPersistenceDisabled = errors.New("llm registry: persistence disabled (no config path)")

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
	Enabled    bool                      `json:"enabled"            yaml:"enabled"`
	Providers  map[string]ProviderConfig `json:"providers"          yaml:"providers"`
	BatchSize  int                       `json:"batch_size"         yaml:"batch_size"`
	MaxContext int                       `json:"max_context_tokens" yaml:"max_context_tokens"`
	MaxTokens  int                       `json:"max_tokens"         yaml:"max_tokens"`
	Interval   time.Duration             `json:"interval"           yaml:"interval"`
	Timeout    time.Duration             `json:"timeout"            yaml:"timeout"`
}

// ProviderFactory creates a Provider from a ProviderConfig. It also receives
// the full RegistryConfig the provider is being built under, so registry-wide
// settings (batch size, flush interval, default timeout) are read from the
// config current at build time rather than captured once at startup — a
// runtime Update with, say, a new batch_size builds providers that honor it.
type ProviderFactory func(name string, cfg ProviderConfig, reg RegistryConfig) Provider

// Registry holds the current LLM providers and configuration.
// It supports live updates and graceful persistence on shutdown.
type Registry struct {
	updateMu   sync.Mutex
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
	if cfg.Enabled {
		for name, pCfg := range cfg.Providers {
			r.providers[name] = factory(name, pCfg, cfg)
		}
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
	r.Swap(cfg)()
}

// UpdateAndFlush persists a new configuration before applying it at runtime.
func (r *Registry) UpdateAndFlush(cfg RegistryConfig) error {
	r.updateMu.Lock()
	if r.configPath == "" {
		r.updateMu.Unlock()
		return ErrPersistenceDisabled
	}
	if err := configwrite.WriteSection(r.configPath, []string{"classifier", "llm"}, cfg); err != nil {
		r.updateMu.Unlock()
		return fmt.Errorf("llm registry: %w", err)
	}
	drain := r.Swap(cfg)
	r.updateMu.Unlock()
	drain()

	return nil
}

// Swap replaces providers from a new config like Update, but defers draining:
// it returns a func the caller MUST invoke — after releasing any locks it
// holds — to drain the evicted providers. This lets a caller that serializes
// config mutations under its own mutex avoid holding that mutex across a
// potentially slow drain (a batch flush is a network round-trip). Evicted
// providers are drained in sorted name order for determinism.
func (r *Registry) Swap(cfg RegistryConfig) (drain func()) {
	r.mu.Lock()
	old := r.providers

	newProviders := make(map[string]Provider, len(cfg.Providers))
	if cfg.Enabled {
		for name, pCfg := range cfg.Providers {
			newProviders[name] = r.factory(name, pCfg, cfg)
		}
	}

	r.providers = newProviders
	r.config = cfg
	r.mu.Unlock()

	return func() {
		names := make([]string, 0, len(old))
		for name := range old {
			names = append(names, name)
		}

		sort.Strings(names)

		for _, name := range names {
			if d, ok := old[name].(Drainer); ok {
				d.Drain()
			}
		}
	}
}

// Flush writes the current LLM config to the config file on disk under the
// key classifier.llm, leaving every other section of the file intact.
//
// It is safe against the two ways the previous implementation could lose data:
//
//   - It edits a yaml.Node tree rather than round-tripping through
//     map[string]interface{}, so comments and key ordering in sections it does
//     not own survive.
//   - A read or parse failure on the existing file ABORTS the write. The old
//     code discarded the unmarshal error, so a corrupt or unreadable config
//     degraded to writing a file containing only the classifier section —
//     silently destroying everything else. Here, the worst input produces no
//     write at all rather than the most destructive one.
//
// The write itself is atomic (temp file in the same directory, fsync, rename),
// so a crash mid-write leaves either the complete old file or the complete new
// one, never a truncated file. An absent file is not an error: it is created
// containing only the classifier section.
//
// Returns ErrPersistenceDisabled (not nil) when no config path is set, so the
// caller can distinguish "did nothing" from "wrote successfully".
func (r *Registry) Flush() error {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()

	r.mu.RLock()
	cfg := r.config
	r.mu.RUnlock()
	if r.configPath == "" {
		return ErrPersistenceDisabled
	}

	if err := configwrite.WriteSection(r.configPath, []string{"classifier", "llm"}, cfg); err != nil {
		return fmt.Errorf("llm registry: %w", err)
	}

	return nil
}

// ToJSON returns the current config as JSON bytes (for API responses or logging).
func (r *Registry) ToJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return json.Marshal(r.config)
}
