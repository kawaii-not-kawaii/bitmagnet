package classifierllm

import (
	"context"
	"time"

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
	cfg := p.Config.Llm
	if cfg.ProviderBaseURL == "" {
		return Result{}
	}

	name := cfg.ProviderName
	if name == "" {
		name = "default"
	}

	p.Logger.Infof("llm provider config: name=%s base_url=%s model=%s", name, cfg.ProviderBaseURL, cfg.ProviderModel)

	regCfg := llm.RegistryConfig{
		Providers: map[string]llm.ProviderConfig{
			name: {
				BaseURL: cfg.ProviderBaseURL,
				Model:   cfg.ProviderModel,
				APIKey:  cfg.ProviderAPIKey,
				Timeout: cfg.Timeout,
			},
		},
		BatchSize:  cfg.BatchSize,
		MaxContext: cfg.MaxContext,
		MaxTokens:  cfg.MaxTokens,
		Interval:   cfg.Interval,
		Timeout:    cfg.Timeout,
	}

	defaultTimeout := regCfg.Timeout
	batchSize := regCfg.BatchSize
	flushAfter := regCfg.Interval
	if flushAfter <= 0 {
		flushAfter = 5 * time.Second
	}
	factory := func(name string, cfg llm.ProviderConfig) llm.Provider {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		p.Logger.Infof("llm provider '%s' ready: %s (batch=%d)", name, cfg.BaseURL, batchSize)
		base := openai.New(openai.Config{
			Name:    name,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			Timeout: timeout,
		})
		if batchSize > 1 {
			return openai.NewBatchClient(base, batchSize, flushAfter)
		}
		return base
	}

	registry := llm.NewRegistry(regCfg, factory, "")
	providers := registry.All()

	p.Lifecycle.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if err := registry.Flush(); err != nil {
				p.Logger.Warnf("failed to flush LLM config on shutdown: %v", err)
				return nil
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
