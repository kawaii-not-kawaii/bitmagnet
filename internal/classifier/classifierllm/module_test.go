package classifierllm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	configpkg "github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestRegistryConfig_MapsSectionOntoRegistry(t *testing.T) {
	t.Parallel()

	got := RegistryConfig(classifier.LlmConfig{
		Enabled:         true,
		ProviderName:    "gemma4",
		ProviderBaseURL: "https://llm.internal",
		ProviderModel:   "gemma-4",
		ProviderAPIKey:  "k",
		BatchSize:       4,
		MaxContext:      7000,
		MaxTokens:       256,
		Timeout:         30 * time.Second,
		Interval:        5 * time.Second,
	})

	p, ok := got.Providers["gemma4"]
	if !ok {
		t.Fatalf("provider 'gemma4' missing: %v", got.Providers)
	}

	if p.BaseURL != "https://llm.internal" || p.Model != "gemma-4" || p.APIKey != "k" {
		t.Errorf("provider config mismapped: %+v", p)
	}

	if got.BatchSize != 4 || got.MaxContext != 7000 || got.MaxTokens != 256 {
		t.Errorf("registry-wide fields mismapped: %+v", got)
	}
}

func TestRegistryConfig_DefaultsProviderName(t *testing.T) {
	t.Parallel()

	got := RegistryConfig(classifier.LlmConfig{Enabled: true, ProviderBaseURL: "https://llm.internal"})
	if _, ok := got.Providers["default"]; !ok {
		t.Fatalf("unnamed provider should register as 'default': %v", got.Providers)
	}
}

func TestRegistryConfig_NoBaseURLYieldsZeroProviders(t *testing.T) {
	t.Parallel()

	got := RegistryConfig(classifier.LlmConfig{})
	if len(got.Providers) != 0 {
		t.Fatalf("expected zero providers without a base URL, got %v", got.Providers)
	}
}

// TestNew_RegistryAlwaysConstructed_LiveToggle verifies runtime config can
// enable and disable providers without a restart.
func TestNew_RegistryAlwaysConstructed_LiveToggle(t *testing.T) {
	t.Parallel()

	config := classifier.Config{Concurrency: 8}
	lc := fxtest.NewLifecycle(t)
	controller := classifier.NewConcurrencyController(config, nil, lc)
	res := New(Params{
		Config:     config,
		Logger:     zap.NewNop().Sugar(),
		Lifecycle:  lc,
		Controller: controller,
	})

	if res.Registry == nil {
		t.Fatal("registry must be constructed even with no LLM configured")
	}

	if len(res.Registry.All()) != 0 {
		t.Fatalf("expected zero providers at startup, got %v", res.Registry.All())
	}

	after, err := res.LiveApplier.Apply(classifier.Config{Llm: classifier.LlmConfig{
		Enabled:         true,
		ProviderName:    "late",
		ProviderBaseURL: "https://llm.internal",
		ProviderModel:   "m",
		BatchSize:       1,
		MaxTokens:       256,
	}})
	if err != nil {
		t.Fatalf("apply runtime classifier config: %v", err)
	}

	after()

	if res.Registry.Get("late") == nil {
		t.Fatal("provider enabled at runtime not present in registry")
	}

	after, err = res.LiveApplier.Apply(classifier.Config{Llm: classifier.LlmConfig{
		Enabled:         false,
		ProviderName:    "late",
		ProviderBaseURL: "https://llm.internal",
		ProviderModel:   "m",
	}})
	if err != nil {
		t.Fatalf("disable runtime classifier config: %v", err)
	}

	after()

	if len(res.Registry.All()) != 0 {
		t.Fatalf("disabled registry still has providers: %v", res.Registry.All())
	}
}

// TestNew_WiresConfigPath: the registry must be constructed with the real
// persistence target so Flush actually writes (previously hardcoded "").
func TestNew_WiresConfigPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")
	config := classifier.Config{
		Concurrency: 8,
		Llm: classifier.LlmConfig{
			Enabled:         true,
			ProviderName:    "gemma4",
			ProviderBaseURL: "https://llm.internal",
			ProviderModel:   "gemma-4",
		},
	}
	lifecycle := fxtest.NewLifecycle(t)
	res := New(Params{
		Config:     config,
		ConfigPath: configwrite.TargetPath(path),
		Logger:     zap.NewNop().Sugar(),
		Lifecycle:  lifecycle,
		Controller: classifier.NewConcurrencyController(config, nil, lifecycle),
	})

	if err := res.Registry.Flush(); err != nil {
		t.Fatalf("Flush with a wired path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read flushed config: %v", err)
	}

	if !strings.Contains(string(data), "classifier:") ||
		!strings.Contains(string(data), "base_url: https://llm.internal") {
		t.Errorf("flushed config missing classifier.llm section:\n%s", data)
	}
}

func TestLiveApplierConfiguresConcurrency(t *testing.T) {
	t.Parallel()

	config := classifier.Config{Concurrency: 8}
	recorder := llmobs.New()
	lifecycle := fxtest.NewLifecycle(t)
	controller := classifier.NewConcurrencyController(config, recorder, lifecycle)
	result := New(Params{
		Config:     config,
		Logger:     zap.NewNop().Sugar(),
		Lifecycle:  lifecycle,
		Controller: controller,
	})

	apply := func(config classifier.Config) {
		t.Helper()

		after, err := result.LiveApplier.Apply(config)
		require.NoError(t, err)
		after()
	}

	apply(classifier.Config{Concurrency: 8, AutoScale: true})
	assert.Equal(t, 1, controller.Effective())

	controller.SetEffective(4)
	apply(classifier.Config{Concurrency: 10, AutoScale: true})
	assert.Equal(t, 4, controller.Effective())

	apply(classifier.Config{Concurrency: 3, AutoScale: true})
	assert.Equal(t, 3, controller.Effective())

	apply(classifier.Config{Concurrency: 6})
	assert.Equal(t, 6, controller.Effective())

	stats := recorder.Stats(0)
	assert.Equal(t, 6, stats.Concurrency)
	assert.Equal(t, 6, stats.EffectiveConcurrency)
}

func TestConfigMutationSurfacesEnabledProviderValidation(t *testing.T) {
	t.Parallel()

	initial := classifier.NewDefaultConfig()
	validate := validator.New()
	resolvedResult, err := configpkg.New(configpkg.Params{
		Specs: []configpkg.Spec{{
			Key:          "classifier",
			DefaultValue: initial,
		}},
		Validate: validate,
	})
	require.NoError(t, err)

	resolved := &concurrency.AtomicValue[configpkg.ResolvedConfig]{}
	resolved.Set(resolvedResult.Resolved)

	path := filepath.Join(t.TempDir(), "config.yml")
	lifecycle := fxtest.NewLifecycle(t)
	result := New(Params{
		Config:     initial,
		ConfigPath: configwrite.TargetPath(path),
		Logger:     zap.NewNop().Sugar(),
		Lifecycle:  lifecycle,
		Controller: classifier.NewConcurrencyController(initial, nil, lifecycle),
	})
	applier := configapply.New(configapply.Params{
		Appliers: []configapply.LiveApplier{result.LiveApplier},
		Resolved: resolved,
		Validate: validate,
		Path:     configwrite.TargetPath(path),
	}).Applier

	_, err = applier.SetSection("classifier", map[string]any{
		"concurrency": 10,
		"llm": map[string]any{
			"enabled":           true,
			"provider_base_url": "/v1",
			"provider_model":    "model",
			"batch_size":        1,
			"max_tokens":        256,
		},
	})
	require.ErrorContains(t, err, "provider_base_url")

	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	assert.Empty(t, result.Registry.All())
}
