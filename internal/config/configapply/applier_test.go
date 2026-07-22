package configapply

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/go-playground/validator/v10"
	"go.uber.org/fx"
)

type testSection struct {
	Count   int           `validate:"gte=1"`
	Timeout time.Duration `validate:"gt=0"`
}

func TestSetSection_RejectsBeforeSideEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  map[string]any
	}{
		{
			name: "validation failure",
			raw:  map[string]any{"Count": float64(0), "Timeout": "45s"},
		},
		{
			name: "unknown field",
			raw:  map[string]any{"Count": float64(3), "Timeout": "45s", "Unexpected": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const originalFile = "sample:\n  count: 2\n  timeout: 1s\n"
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(originalFile), 0o644); err != nil {
				t.Fatalf("seed config: %v", err)
			}

			initial := testSection{Count: 2, Timeout: time.Second}
			calls := 0
			applier, resolved := newTestApplier(path, map[string]any{"sample": initial}, []LiveApplier{{
				Key: "sample",
				Apply: func(any) (func(), error) {
					calls++
					return nil, nil
				},
			}})

			if _, err := applier.SetSection("sample", tt.raw); err == nil {
				t.Fatal("SetSection unexpectedly succeeded")
			}
			if calls != 0 {
				t.Fatalf("live applier called %d times, want 0", calls)
			}

			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if string(contents) != originalFile {
				t.Fatalf("config changed on rejection:\n%s", contents)
			}
			if got := resolved.Get().NodeMap["sample"].Value; !reflect.DeepEqual(got, initial) {
				t.Fatalf("resolved value changed: %#v", got)
			}
		})
	}
}

func TestSetSection_OutcomesAndVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		live bool
	}{
		{name: "live", live: true},
		{name: "restart required", live: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			initial := testSection{Count: 1, Timeout: time.Second}
			want := testSection{Count: 7, Timeout: 45 * time.Second}
			var applied any
			var liveAppliers []LiveApplier
			if tt.live {
				liveAppliers = []LiveApplier{{
					Key: "sample",
					Apply: func(value any) (func(), error) {
						applied = value
						return nil, nil
					},
				}}
			}

			applier, resolved := newTestApplier(path, map[string]any{"sample": initial}, liveAppliers)
			outcome, err := applier.SetSection("sample", map[string]any{
				"Count":   float64(7),
				"Timeout": "45s",
			})
			if err != nil {
				t.Fatalf("SetSection: %v", err)
			}

			if outcome.Key != "sample" || outcome.Live != tt.live ||
				!reflect.DeepEqual(outcome.Value, want) {
				t.Fatalf("unexpected outcome: %#v", outcome)
			}
			if applier.IsLive("sample") != tt.live {
				t.Fatalf("IsLive = %v, want %v", applier.IsLive("sample"), tt.live)
			}
			if tt.live && !reflect.DeepEqual(applied, want) {
				t.Fatalf("live applier received %#v, want %#v", applied, want)
			}
			if got := resolved.Get().NodeMap["sample"].Value; !reflect.DeepEqual(got, want) {
				t.Fatalf("resolved value = %#v, want %#v", got, want)
			}

			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			for _, fragment := range []string{"count: 7", "timeout: 45s"} {
				if !strings.Contains(string(contents), fragment) {
					t.Errorf("persisted config missing %q:\n%s", fragment, contents)
				}
			}
		})
	}
}

func TestSetSection_SectionErrors(t *testing.T) {
	t.Parallel()

	initial := testSection{Count: 1, Timeout: time.Second}
	nodes := map[string]any{
		"postgres":    initial,
		"auth":        initial,
		"http_server": initial,
		"dht_server":  initial,
	}
	applier, _ := newTestApplier(filepath.Join(t.TempDir(), "config.yml"), nodes, nil)

	tests := []struct {
		key  string
		want error
	}{
		{key: "postgres", want: ErrSectionNotMutable},
		{key: "auth", want: ErrSectionNotMutable},
		{key: "http_server", want: ErrSectionNotMutable},
		{key: "dht_server", want: ErrSectionNotMutable},
		{key: "missing", want: ErrUnknownSection},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			_, err := applier.SetSection(tt.key, map[string]any{"Count": 2, "Timeout": "2s"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("SetSection error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSetSection_PersistenceFailureLeavesLiveApply(t *testing.T) {
	t.Parallel()

	initial := testSection{Count: 1, Timeout: time.Second}
	want := testSection{Count: 4, Timeout: 4 * time.Second}
	var liveValue any
	afterCalls := 0
	path := filepath.Join(t.TempDir(), "missing", "config.yml")
	applier, resolved := newTestApplier(path, map[string]any{"sample": initial}, []LiveApplier{{
		Key: "sample",
		Apply: func(value any) (func(), error) {
			liveValue = value
			return func() { afterCalls++ }, nil
		},
	}})

	_, err := applier.SetSection("sample", map[string]any{"Count": float64(4), "Timeout": "4s"})
	if err == nil {
		t.Fatal("SetSection unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "persist section") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(liveValue, want) {
		t.Fatalf("live value = %#v, want %#v", liveValue, want)
	}
	if afterCalls != 1 {
		t.Fatalf("after called %d times, want 1", afterCalls)
	}
	if got := resolved.Get().NodeMap["sample"].Value; !reflect.DeepEqual(got, initial) {
		t.Fatalf("resolved value changed despite persistence failure: %#v", got)
	}
}

func TestSetSection_AfterRunsOutsideMutex(t *testing.T) {
	t.Parallel()

	initial := testSection{Count: 1, Timeout: time.Second}
	secondary := testSection{Count: 3, Timeout: 3 * time.Second}
	path := filepath.Join(t.TempDir(), "config.yml")

	var applier *Applier
	var afterErr error
	afterCalls := 0
	applier, resolved := newTestApplier(path, map[string]any{
		"primary":   initial,
		"secondary": initial,
	}, []LiveApplier{{
		Key: "primary",
		Apply: func(any) (func(), error) {
			return func() {
				afterCalls++
				_, afterErr = applier.SetSection("secondary", map[string]any{
					"Count":   float64(3),
					"Timeout": "3s",
				})
			}, nil
		},
	}})

	done := make(chan error, 1)
	go func() {
		_, err := applier.SetSection("primary", map[string]any{"Count": float64(2), "Timeout": "2s"})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetSection: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetSection deadlocked while running after func")
	}

	if afterErr != nil {
		t.Fatalf("after SetSection: %v", afterErr)
	}
	if afterCalls != 1 {
		t.Fatalf("after called %d times, want 1", afterCalls)
	}
	if got := resolved.Get().NodeMap["secondary"].Value; !reflect.DeepEqual(got, secondary) {
		t.Fatalf("secondary resolved value = %#v, want %#v", got, secondary)
	}
}

func TestNew_ProvidesChangeability(t *testing.T) {
	t.Parallel()

	err := fx.ValidateApp(
		fx.Provide(
			func() *concurrency.AtomicValue[config.ResolvedConfig] {
				return &concurrency.AtomicValue[config.ResolvedConfig]{}
			},
			validator.New,
			func() configwrite.TargetPath { return "config.yml" },
			New,
		),
		fx.Invoke(func(*Applier, Changeability) {}),
	)
	if err != nil {
		t.Fatalf("validate app: %v", err)
	}
}

func newTestApplier(
	path string,
	values map[string]any,
	liveAppliers []LiveApplier,
) (*Applier, *concurrency.AtomicValue[config.ResolvedConfig]) {
	nodeMap := make(map[string]config.ResolvedNode, len(values))
	for key, value := range values {
		nodeMap[key] = config.ResolvedNode{
			Spec:     config.Spec{Key: key, DefaultValue: value},
			IsStruct: true,
			Type:     reflect.TypeOf(value),
			Value:    value,
			ValueRaw: value,
		}
	}

	resolved := &concurrency.AtomicValue[config.ResolvedConfig]{}
	resolved.Set(config.ResolvedConfig{NodeMap: nodeMap})
	result := New(Params{
		Appliers: liveAppliers,
		Resolved: resolved,
		Validate: validator.New(),
		Path:     configwrite.TargetPath(path),
	})

	return result.Applier, resolved
}
