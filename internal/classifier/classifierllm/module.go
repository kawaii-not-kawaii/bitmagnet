package classifierllm

import (
	"context"
	"sort"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configfx"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/openai"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config     classifier.Config
	Logger     *zap.SugaredLogger
	Lifecycle  fx.Lifecycle
	ConfigPath configfx.WritePath
}

type Result struct {
	fx.Out
	Registry *llm.Registry
	Stats    *llm.Stats
}

func New(p Params) Result {
	cfg := p.Config.Llm
	name := cfg.ProviderName
	if name == "" {
		name = "default"
	}

	providers := make(map[string]llm.ProviderConfig)
	if cfg.ProviderBaseURL != "" {
		providers[name] = llm.ProviderConfig{
			BaseURL: cfg.ProviderBaseURL,
			Model:   cfg.ProviderModel,
			APIKey:  cfg.ProviderAPIKey,
			Timeout: cfg.Timeout,
		}
	}

	regCfg := llm.RegistryConfig{
		Enabled:    cfg.Enabled && cfg.ProviderBaseURL != "",
		Providers:  providers,
		BatchSize:  cfg.BatchSize,
		MaxContext: cfg.MaxContext,
		MaxTokens:  cfg.MaxTokens,
		Interval:   cfg.Interval,
		Timeout:    cfg.Timeout,
	}
	stats := llm.NewStats()

	factory := func(name string, providerCfg llm.ProviderConfig, registryCfg llm.RegistryConfig) llm.Provider {
		timeout := providerCfg.Timeout
		if timeout <= 0 {
			timeout = registryCfg.Timeout
		}
		flushAfter := registryCfg.Interval
		if flushAfter <= 0 {
			flushAfter = 5 * time.Second
		}

		p.Logger.Infof(
			"llm provider '%s' ready: %s (batch=%d)",
			name,
			providerCfg.BaseURL,
			registryCfg.BatchSize,
		)

		base := openai.New(openai.Config{
			Name:    name,
			BaseURL: providerCfg.BaseURL,
			Model:   providerCfg.Model,
			APIKey:  providerCfg.APIKey,
			Timeout: timeout,
			Observe: stats.Record,
		})
		if registryCfg.BatchSize > 1 {
			return openai.NewBatchClient(base, registryCfg.BatchSize, flushAfter)
		}

		return base
	}

	registry := llm.NewRegistry(regCfg, factory, string(p.ConfigPath))
	p.Lifecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			providers := registry.All()
			names := make([]string, 0, len(providers))
			for name := range providers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if drainer, ok := providers[name].(llm.Drainer); ok {
					drainer.Drain()
				}
			}
			return nil
		},
	})

	return Result{Registry: registry, Stats: stats}
}
