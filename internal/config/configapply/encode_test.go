package configapply

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configresolver"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

type encodeNested struct {
	RetryDelay time.Duration
	Label      string
}

type encodeFixture struct {
	BaseURL string
	Timeout time.Duration
	Nested  encodeNested
	Tags    []string
}

func TestEncodeSection_RoundTripsThroughResolver(t *testing.T) {
	t.Parallel()

	want := encodeFixture{
		BaseURL: "https://example.com",
		Timeout: 90 * time.Second,
		Nested: encodeNested{
			RetryDelay: 250 * time.Millisecond,
			Label:      "fast",
		},
		Tags: []string{"one", "two"},
	}

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := configwrite.WriteSection(path, []string{"sample"}, encodeSection(want)); err != nil {
		t.Fatalf("WriteSection: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var document map[string]any

	if err = yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	section, ok := document["sample"].(map[string]any)
	if !ok {
		t.Fatalf("sample section has type %T", document["sample"])
	}

	if section["base_url"] != want.BaseURL || section["timeout"] != "1m30s" {
		t.Fatalf("unexpected scalar encoding: %#v", section)
	}

	nested, ok := section["nested"].(map[string]any)
	if !ok || nested["retry_delay"] != "250ms" || nested["label"] != "fast" {
		t.Fatalf("unexpected nested encoding: %#v", section["nested"])
	}

	if _, exists := section["baseurl"]; exists {
		t.Fatalf("non-snake-case key written: %#v", section)
	}

	if tags, ok := section["tags"].([]any); !ok || !reflect.DeepEqual(tags, []any{"one", "two"}) {
		t.Fatalf("unexpected slice encoding: %#v", section["tags"])
	}

	validate := validator.New()

	resolver, err := configresolver.NewFromYamlFile(path, false, validate)
	if err != nil {
		t.Fatalf("NewFromYamlFile: %v", err)
	}

	resolved, err := config.New(config.Params{
		Specs:     []config.Spec{{Key: "sample", DefaultValue: encodeFixture{}}},
		Resolvers: []configresolver.Resolver{resolver},
		Validate:  validate,
	})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	if got := resolved.Resolved.NodeMap["sample"].Value; !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip value = %#v, want %#v", got, want)
	}
}

func TestEncodeSection_RecursesCollections(t *testing.T) {
	t.Parallel()

	encoded, ok := encodeSection(struct {
		Backoffs []time.Duration
		ByName   map[string]time.Duration
	}{
		Backoffs: []time.Duration{time.Second, 2 * time.Second},
		ByName:   map[string]time.Duration{"fast": time.Millisecond},
	}).(map[string]any)
	if !ok {
		t.Fatalf("encoded section has type %T", encoded)
	}

	if !reflect.DeepEqual(encoded["backoffs"], []any{"1s", "2s"}) {
		t.Fatalf("backoffs = %#v", encoded["backoffs"])
	}

	if !reflect.DeepEqual(encoded["by_name"], map[string]any{"fast": "1ms"}) {
		t.Fatalf("by_name = %#v", encoded["by_name"])
	}
}

func TestPreserveRedactedRestoresCurrentValues(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"keywords":   map[string]any{"movie": []any{"remux"}},
		"max_tokens": 256,
		"llm":        map[string]any{"provider_api_key": "real-key"},
	}
	raw := map[string]any{
		"keywords":   RedactedPlaceholder,
		"max_tokens": RedactedPlaceholder,
		"Llm":        map[string]any{"ProviderAPIKey": RedactedPlaceholder, "ProviderModel": "m"},
		"workflow":   "default",
	}

	preserved, ok := preserveRedacted(raw, current).(map[string]any)
	if !ok {
		t.Fatalf("preserved has type %T", preserved)
	}

	if !reflect.DeepEqual(preserved["keywords"], current["keywords"]) {
		t.Fatalf("keywords = %#v", preserved["keywords"])
	}

	if preserved["max_tokens"] != 256 {
		t.Fatalf("max_tokens = %#v", preserved["max_tokens"])
	}

	llm, ok := preserved["Llm"].(map[string]any)
	if !ok || llm["ProviderAPIKey"] != "real-key" || llm["ProviderModel"] != "m" {
		t.Fatalf("llm = %#v", preserved["Llm"])
	}

	if preserved["workflow"] != "default" {
		t.Fatalf("workflow = %#v", preserved["workflow"])
	}
}
