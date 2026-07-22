package llm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// parsedConfig mirrors the slice of a bitmagnet config file that Flush
// touches, for asserting the write round-trips losslessly. Named (rather than
// an inline anonymous struct) to satisfy revive's nested-structs rule.
type classifierSection struct {
	Llm RegistryConfig `yaml:"llm"`
}

type parsedConfig struct {
	Classifier classifierSection `yaml:"classifier"`
	Other      map[string]string `yaml:"other"`
}

// noopFactory builds providers that do nothing; Flush never touches providers.
func noopFactory(name string, _ ProviderConfig, _ RegistryConfig) Provider {
	return &mockProvider{name: name}
}

func testRegistry(t *testing.T, configPath string) *Registry {
	t.Helper()

	return NewRegistry(RegistryConfig{
		Enabled: true,
		Providers: map[string]ProviderConfig{
			"gemma": {BaseURL: "https://llm.internal", Model: "gemma-4"},
		},
		BatchSize: 1,
		MaxTokens: 256,
	}, noopFactory, configPath)
}

// writeFile writes contents to a fresh file in a temp dir and returns its path.
func writeFile(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	return path
}

// TestFlush_PreservesUnrelatedSections is the core data-loss regression. The
// previous implementation, given a file it could not fully parse, wrote back a
// file containing ONLY the classifier section. Even on the happy path, this
// asserts that a write of classifier.llm leaves every sibling section present.
func TestFlush_PreservesUnrelatedSections(t *testing.T) {
	t.Parallel()

	const original = `dht:
  bootstrap: router.example.com

http_server:
  local_address: ":3333"

classifier:
  llm:
    providers:
      old:
        base_url: https://stale
`

	path := writeFile(t, "config.yml", original)
	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	got := readBack(t, path)

	for _, want := range []string{"dht:", "router.example.com", "http_server:", ":3333"} {
		if !strings.Contains(got, want) {
			t.Errorf("unrelated section lost %q:\n%s", want, got)
		}
	}

	// Our section must reflect the registry's providers, not the stale ones.
	if !strings.Contains(got, "gemma") {
		t.Errorf("classifier.llm not updated to current providers:\n%s", got)
	}

	if strings.Contains(got, "https://stale") {
		t.Errorf("stale provider survived under classifier.llm:\n%s", got)
	}
}

// TestFlush_PreservesCommentsAndOrdering pins the yaml.Node behavior: comments
// and key ordering in sections Flush does not own must survive. A
// map[string]interface{} round-trip destroys both.
func TestFlush_PreservesCommentsAndOrdering(t *testing.T) {
	t.Parallel()

	const original = `# top of file
dht:
  # bootstrap node, do not remove
  bootstrap: router.example.com
  port: 3334

http_server:
  local_address: ":3333"
`

	path := writeFile(t, "config.yml", original)
	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	got := readBack(t, path)

	for _, want := range []string{
		"# top of file",
		"# bootstrap node, do not remove",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("comment lost %q:\n%s", want, got)
		}
	}

	// Ordering within dht: bootstrap must still precede port.
	if bootstrapAt, portAt := strings.Index(got, "bootstrap:"), strings.Index(got, "port:"); bootstrapAt > portAt {
		t.Errorf("dht key ordering changed:\n%s", got)
	}
}

// TestFlush_UnparseableFileAborts asserts the fail-closed inversion: a file
// that cannot be parsed is left untouched rather than overwritten.
func TestFlush_UnparseableFileAborts(t *testing.T) {
	t.Parallel()

	// Valid-looking start followed by a broken mapping/sequence mix.
	const broken = "classifier:\n  llm: [unterminated\n\ttab: nope\n"
	path := writeFile(t, "config.yml", broken)

	err := testRegistry(t, path).Flush()
	if err == nil {
		t.Fatal("expected Flush to fail on an unparseable file")
	}

	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("expected a parse error, got: %v", err)
	}

	if got := readBack(t, path); got != broken {
		t.Errorf("file was modified despite parse failure:\ngot:  %q\nwant: %q", got, broken)
	}
}

// TestFlush_AbsentFileCreatesWithOnlyOurSection: a missing file is not an
// error; it is created containing just the classifier section.
func TestFlush_AbsentFileCreatesWithOnlyOurSection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")

	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush on absent file: %v", err)
	}

	got := readBack(t, path)
	if !strings.Contains(got, "classifier:") || !strings.Contains(got, "gemma") {
		t.Errorf("expected a classifier section, got:\n%s", got)
	}

	// Round-trips as valid YAML.
	var check map[string]any
	if err := yaml.Unmarshal([]byte(got), &check); err != nil {
		t.Errorf("created file is not valid YAML: %v", err)
	}
}

// TestFlush_DisabledReturnsSentinel: an empty config path is reported as
// disabled, distinguishable from a successful write.
func TestFlush_DisabledReturnsSentinel(t *testing.T) {
	t.Parallel()

	err := testRegistry(t, "").Flush()
	if !errors.Is(err, ErrPersistenceDisabled) {
		t.Errorf("expected ErrPersistenceDisabled, got: %v", err)
	}
}

// TestFlush_AtomicWritePreservesMode: the rewritten file keeps its original
// permission bits rather than resetting to a default.
func TestFlush_AtomicWritePreservesMode(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "config.yml", "classifier:\n  llm: {}\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode not preserved: got %o want 600", got)
	}
}

// TestFlush_NoTempFilesLeftBehind: the atomic write must not leave its temp
// file in the directory after success.
func TestFlush_NoTempFilesLeftBehind(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "config.yml", "classifier:\n  llm: {}\n")

	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// TestFlush_RewrittenConfigStillParses: the classifier.llm we wrote must be
// readable back as a RegistryConfig, i.e. the round-trip is lossless for our
// own section.
func TestFlush_RewrittenConfigStillParses(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "config.yml", "other:\n  keep: me\n")

	if err := testRegistry(t, path).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	var doc parsedConfig
	if err := yaml.Unmarshal([]byte(readBack(t, path)), &doc); err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if _, ok := doc.Classifier.Llm.Providers["gemma"]; !ok {
		t.Errorf("gemma provider not round-tripped: %+v", doc.Classifier.Llm)
	}

	if doc.Other["keep"] != "me" {
		t.Errorf("sibling scalar section not preserved: %+v", doc.Other)
	}
}

func TestUpdateAndFlushAppliesRuntimeConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")

	var factoryConfig RegistryConfig

	registry := NewRegistry(
		RegistryConfig{Enabled: false},
		func(name string, _ ProviderConfig, cfg RegistryConfig) Provider {
			factoryConfig = cfg
			return &mockProvider{name: name}
		},
		path,
	)

	cfg := RegistryConfig{
		Enabled: true,
		Providers: map[string]ProviderConfig{
			"local": {BaseURL: "http://localhost:8080", Model: "gemma"},
		},
		BatchSize: 7,
		Interval:  3 * time.Second,
	}
	if err := registry.UpdateAndFlush(cfg); err != nil {
		t.Fatalf("UpdateAndFlush: %v", err)
	}

	if len(registry.All()) != 1 {
		t.Fatalf("provider was not enabled: %+v", registry.All())
	}

	if factoryConfig.BatchSize != 7 || factoryConfig.Interval != 3*time.Second {
		t.Fatalf("factory received stale registry config: %+v", factoryConfig)
	}

	if !strings.Contains(readBack(t, path), "enabled: true") {
		t.Fatalf("enabled state was not persisted:\n%s", readBack(t, path))
	}

	cfg.Enabled = false
	registry.Update(cfg)

	if len(registry.All()) != 0 {
		t.Fatalf("provider was not disabled: %+v", registry.All())
	}
}

func readBack(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	return string(data)
}
