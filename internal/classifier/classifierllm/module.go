package classifierllm

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/openai"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config     classifier.Config
	ConfigPath configwrite.TargetPath
	Logger     *zap.SugaredLogger
	Lifecycle  fx.Lifecycle
}

type Result struct {
	fx.Out
	Registry    *llm.Registry           `optional:"true"`
	LiveApplier configapply.LiveApplier `group:"config_live_appliers"`
	// LlmProviders is a static snapshot of the providers built at startup,
	// kept for one-shot CLI consumers (llm-bench). The live path — anything
	// that must observe runtime config updates — reads Registry.All() at
	// use-time instead.
	LlmProviders map[string]llm.Provider `optional:"true"`
}

// RegistryConfig maps the classifier's LLM section onto the registry's
// config. With no provider_base_url configured it yields a config with zero
// providers — the registry is still constructed so a later runtime config
// update can bring the first provider up without a restart.
func RegistryConfig(cfg classifier.LlmConfig) llm.RegistryConfig {
	providers := map[string]llm.ProviderConfig{}

	if cfg.Enabled && cfg.ProviderBaseURL != "" {
		name := cfg.ProviderName
		if name == "" {
			name = "default"
		}

		providers[name] = llm.ProviderConfig{
			BaseURL: cfg.ProviderBaseURL,
			Model:   cfg.ProviderModel,
			APIKey:  cfg.ProviderAPIKey,
			Timeout: cfg.Timeout,
		}
	}

	return llm.RegistryConfig{
		Enabled:    cfg.Enabled,
		Providers:  providers,
		BatchSize:  cfg.BatchSize,
		MaxContext: cfg.MaxContext,
		MaxTokens:  cfg.MaxTokens,
		Interval:   cfg.Interval,
		Timeout:    cfg.Timeout,
	}
}

func New(p Params) Result {
	cfg := p.Config.Llm
	regCfg := RegistryConfig(cfg)

	if len(regCfg.Providers) > 0 {
		p.Logger.Infof(
			"llm provider config: name=%s base_url=%s model=%s",
			cfg.ProviderName,
			cfg.ProviderBaseURL,
			cfg.ProviderModel,
		)
	}

	// Registry-wide settings (timeout fallback, batch size, flush interval)
	// are read from the RegistryConfig passed at build time, not captured
	// from the startup config: a runtime Update with a new batch_size must
	// build providers that honor it.
	factory := func(name string, cfg llm.ProviderConfig, reg llm.RegistryConfig) llm.Provider {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = reg.Timeout
		}

		flushAfter := reg.Interval
		if flushAfter <= 0 {
			flushAfter = 5 * time.Second
		}

		p.Logger.Infof("llm provider '%s' ready: %s (batch=%d)", name, cfg.BaseURL, reg.BatchSize)

		base := openai.New(openai.Config{
			Name:       name,
			BaseURL:    cfg.BaseURL,
			Model:      cfg.Model,
			APIKey:     cfg.APIKey,
			Timeout:    timeout,
			MaxContext: reg.MaxContext,
			MaxTokens:  reg.MaxTokens,
		})
		if reg.BatchSize > 1 {
			return openai.NewBatchClient(base, reg.BatchSize, flushAfter)
		}

		return base
	}

	registry := llm.NewRegistry(regCfg, factory, string(p.ConfigPath))

	p.Lifecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			// Drain the providers current at shutdown, not the startup
			// snapshot: a runtime Update swaps the registry's providers, and
			// the evicted ones are drained by the updater. Sorted for a
			// deterministic drain order.
			providers := registry.All()

			names := make([]string, 0, len(providers))
			for name := range providers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if d, ok := providers[name].(llm.Drainer); ok {
					d.Drain()
				}
			}
			return nil
		},
	})

	return Result{
		Registry: registry,
		LiveApplier: configapply.LiveApplier{
			Key: "classifier",
			Apply: func(value any) (func(), error) {
				cfg, ok := value.(classifier.Config)
				if !ok {
					return nil, fmt.Errorf(
						"configapply: section %q: expected %T, got %T",
						"classifier",
						cfg,
						value,
					)
				}

				return registry.Swap(RegistryConfig(cfg.Llm)), nil
			},
		},
		LlmProviders: registry.All(),
	}
}
