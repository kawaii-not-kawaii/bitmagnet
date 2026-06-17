package classifierllm

import (
	"context"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/openai"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config    classifier.Config
	Logger    *zap.SugaredLogger
	Lifecycle fx.Lifecycle
}

type Result struct {
	fx.Out
	Registry     *llm.Registry           `optional:"true"`
	LlmProviders map[string]llm.Provider `optional:"true"`
}

func New(p Params) Result {
	if len(p.Config.Llm.Providers) == 0 {
		return Result{}
	}

	regCfg := llm.RegistryConfig{
		Providers:  make(map[string]llm.ProviderConfig, len(p.Config.Llm.Providers)),
		BatchSize:  p.Config.Llm.BatchSize,
		MaxContext: p.Config.Llm.MaxContext,
		MaxTokens:  p.Config.Llm.MaxTokens,
		Interval:   p.Config.Llm.Interval,
		Timeout:    p.Config.Llm.Timeout,
	}
	for name, pCfg := range p.Config.Llm.Providers {
		p.Logger.Infof("llm provider config: name=%s base_url=%s model=%s", name, pCfg.BaseURL, pCfg.Model)
		regCfg.Providers[name] = llm.ProviderConfig{
			BaseURL:      pCfg.BaseURL,
			Model:        pCfg.Model,
			APIKey:       pCfg.APIKey,
			Timeout:      pCfg.Timeout,
			SystemPrompt: pCfg.SystemPrompt,
		}
	}

	defaultTimeout := regCfg.Timeout
	factory := func(name string, cfg llm.ProviderConfig) llm.Provider {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		p.Logger.Infof("llm provider '%s' ready: %s", name, cfg.BaseURL)
		return openai.New(openai.Config{
			Name:         name,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			APIKey:       cfg.APIKey,
			Timeout:      timeout,
			SystemPrompt: cfg.SystemPrompt,
		})
	}

	registry := llm.NewRegistry(regCfg, factory, "")
	providers := registry.All()
	// Register shutdown hook to flush LLM config changes.
	p.Lifecycle.Append(fx.Hook{
	OnStop: func(ctx context.Context) error {
			if err := registry.Flush(); err != nil {
				p.Logger.Warnf("failed to flush LLM config on shutdown: %v", err)
				return nil // don't block shutdown for config persistence
			}
			p.Logger.Info("LLM config flushed to disk")
			return nil
		},
	})

	return Result{
		Registry:     registry,
		LlmProviders: providers,
	}
}
