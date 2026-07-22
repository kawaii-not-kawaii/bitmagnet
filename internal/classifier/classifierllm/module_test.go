package classifierllm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestRegistryConfig_MapsSectionOntoRegistry(t *testing.T) {
	t.Parallel()

	got := RegistryConfig(classifier.LlmConfig{
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

	got := RegistryConfig(classifier.LlmConfig{ProviderBaseURL: "https://llm.internal"})
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

// TestNew_RegistryAlwaysConstructed_LiveEnable: with no LLM configured at
// startup the registry and live applier must still support enabling the first
// provider without a restart.
func TestNew_RegistryAlwaysConstructed_LiveEnable(t *testing.T) {
	t.Parallel()

	lc := fxtest.NewLifecycle(t)
	res := New(Params{
		Config:    classifier.Config{},
		Logger:    zap.NewNop().Sugar(),
		Lifecycle: lc,
	})

	if res.Registry == nil {
		t.Fatal("registry must be constructed even with no LLM configured")
	}

	if len(res.Registry.All()) != 0 {
		t.Fatalf("expected zero providers at startup, got %v", res.Registry.All())
	}

	after, err := res.LiveApplier.Apply(classifier.Config{Llm: classifier.LlmConfig{
		ProviderName:    "late",
		ProviderBaseURL: "https://llm.internal",
		ProviderModel:   "m",
	}})
	if err != nil {
		t.Fatalf("apply runtime classifier config: %v", err)
	}
	after()

	if res.Registry.Get("late") == nil {
		t.Fatal("provider enabled at runtime not present in registry")
	}
}

// TestNew_WiresConfigPath: the registry must be constructed with the real
// persistence target so Flush actually writes (previously hardcoded "").
func TestNew_WiresConfigPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")
	res := New(Params{
		Config: classifier.Config{Llm: classifier.LlmConfig{
			ProviderName:    "gemma4",
			ProviderBaseURL: "https://llm.internal",
			ProviderModel:   "gemma-4",
		}},
		ConfigPath: configwrite.TargetPath(path),
		Logger:     zap.NewNop().Sugar(),
		Lifecycle:  fxtest.NewLifecycle(t),
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
