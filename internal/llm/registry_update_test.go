package llm

import (
	"sort"
	"sync"
	"testing"
	"time"
)

// drainRecorder implements Provider + Drainer, recording drains into a shared
// ordered log.
type drainRecorder struct {
	mockProvider
	log *drainLog
}

type drainLog struct {
	mu    sync.Mutex
	order []string
}

func (d *drainRecorder) Drain() {
	d.log.mu.Lock()
	defer d.log.mu.Unlock()
	d.log.order = append(d.log.order, d.name)
}

func twoProviderConfig(batchSize int) RegistryConfig {
	return RegistryConfig{
		Enabled: true,
		Providers: map[string]ProviderConfig{
			"alpha": {BaseURL: "https://a.internal", Model: "m"},
			"beta":  {BaseURL: "https://b.internal", Model: "m"},
		},
		BatchSize: batchSize,
		Timeout:   30 * time.Second,
	}
}

// TestRegistry_FactoryReceivesCurrentConfig pins the fix for the stale-closure
// bug: the factory must see the RegistryConfig current at build time, so a
// runtime Update with a new batch_size builds providers that honor it — not
// the value captured at startup.
func TestRegistry_FactoryReceivesCurrentConfig(t *testing.T) {
	t.Parallel()

	var seenBatchSizes []int

	factory := func(name string, _ ProviderConfig, reg RegistryConfig) Provider {
		seenBatchSizes = append(seenBatchSizes, reg.BatchSize)
		return &mockProvider{name: name}
	}

	r := NewRegistry(twoProviderConfig(1), factory, "")

	for _, got := range seenBatchSizes {
		if got != 1 {
			t.Fatalf("startup factory saw batch size %d, want 1", got)
		}
	}

	seenBatchSizes = nil

	r.Update(twoProviderConfig(8))

	if len(seenBatchSizes) != 2 {
		t.Fatalf("factory called %d times on update, want 2", len(seenBatchSizes))
	}

	for _, got := range seenBatchSizes {
		if got != 8 {
			t.Errorf("update factory saw batch size %d, want the updated 8", got)
		}
	}

	if r.Config().BatchSize != 8 {
		t.Errorf("registry config batch size = %d after update, want 8", r.Config().BatchSize)
	}
}

// TestRegistry_SwapDefersDrain: Swap must not drain evicted providers itself —
// draining happens only when the returned func is invoked, in sorted name
// order. This is what lets a mutation applier release its mutex before the
// (potentially slow) drain.
func TestRegistry_SwapDefersDrain(t *testing.T) {
	t.Parallel()

	log := &drainLog{}

	factory := func(name string, _ ProviderConfig, _ RegistryConfig) Provider {
		return &drainRecorder{mockProvider: mockProvider{name: name}, log: log}
	}

	r := NewRegistry(twoProviderConfig(1), factory, "")

	drain := r.Swap(RegistryConfig{Enabled: true, Providers: map[string]ProviderConfig{
		"gamma": {BaseURL: "https://c.internal", Model: "m"},
	}})

	if len(log.order) != 0 {
		t.Fatalf("Swap drained %v before the drain func was invoked", log.order)
	}
	// The new provider set must already be live before draining.
	if r.Get("gamma") == nil || r.Get("alpha") != nil {
		t.Fatal("Swap did not replace the provider set before drain")
	}

	drain()

	want := []string{"alpha", "beta"}
	if !sort.StringsAreSorted(log.order) || len(log.order) != len(want) {
		t.Fatalf("drain order = %v, want sorted %v", log.order, want)
	}

	for i, name := range want {
		if log.order[i] != name {
			t.Fatalf("drain order = %v, want %v", log.order, want)
		}
	}
}

// TestRegistry_UpdateStillDrains: the convenience Update wrapper keeps the old
// contract — evicted providers are drained before it returns.
func TestRegistry_UpdateStillDrains(t *testing.T) {
	t.Parallel()

	log := &drainLog{}

	factory := func(name string, _ ProviderConfig, _ RegistryConfig) Provider {
		return &drainRecorder{mockProvider: mockProvider{name: name}, log: log}
	}

	r := NewRegistry(twoProviderConfig(1), factory, "")
	r.Update(RegistryConfig{Providers: map[string]ProviderConfig{}})

	if len(log.order) != 2 {
		t.Fatalf("Update drained %d providers, want 2", len(log.order))
	}
}

func TestRegistry_DisabledSkipsProviderConstruction(t *testing.T) {
	t.Parallel()

	cfg := twoProviderConfig(1)
	cfg.Enabled = false
	factoryCalls := 0
	registry := NewRegistry(cfg, func(name string, _ ProviderConfig, _ RegistryConfig) Provider {
		factoryCalls++

		return &mockProvider{name: name}
	}, "")

	if factoryCalls != 0 || len(registry.All()) != 0 {
		t.Fatalf("disabled startup built %d providers: %v", factoryCalls, registry.All())
	}

	cfg.Enabled = true
	registry.Update(cfg)

	if factoryCalls != len(cfg.Providers) {
		t.Fatalf("enabled update built %d providers, want %d", factoryCalls, len(cfg.Providers))
	}

	cfg.Enabled = false
	registry.Update(cfg)

	if len(registry.All()) != 0 {
		t.Fatalf("disabled update left providers: %v", registry.All())
	}
}
