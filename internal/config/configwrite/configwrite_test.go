package configwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeTemp(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	return path
}

func readBack(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	return string(data)
}

func TestWriteSection_EmptyKeyPath(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "config.yml", "a: b\n")
	if err := WriteSection(path, nil, map[string]any{"x": 1}); err == nil {
		t.Fatal("expected an error for an empty key path")
	}
}

// TestWriteSection_TopLevelSection writes a whole top-level section (the shape
// the config mutation API uses) and asserts siblings survive.
func TestWriteSection_TopLevelSection(t *testing.T) {
	t.Parallel()

	const original = `dht:
  bootstrap: router.example.com

tmdb:
  api_key: old
`

	path := writeTemp(t, "config.yml", original)

	err := WriteSection(path, []string{"tmdb"}, map[string]any{
		"api_key": "new",
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("WriteSection: %v", err)
	}

	got := readBack(t, path)

	for _, want := range []string{"dht:", "router.example.com", "api_key: new", "enabled: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "old") {
		t.Errorf("stale value survived:\n%s", got)
	}
}

// TestWriteSection_NestedKeyPath writes classifier.llm (the shape the LLM
// registry uses) without disturbing sibling classifier keys.
func TestWriteSection_NestedKeyPath(t *testing.T) {
	t.Parallel()

	const original = `classifier:
  # keep this comment
  flags: [a, b]
  llm:
    provider: old
`

	path := writeTemp(t, "config.yml", original)

	err := WriteSection(path, []string{"classifier", "llm"}, map[string]any{"provider": "new"})
	if err != nil {
		t.Fatalf("WriteSection: %v", err)
	}

	got := readBack(t, path)

	if !strings.Contains(got, "provider: new") {
		t.Errorf("nested value not written:\n%s", got)
	}

	for _, want := range []string{"# keep this comment", "flags:"} {
		if !strings.Contains(got, want) {
			t.Errorf("sibling classifier content lost %q:\n%s", want, got)
		}
	}
}

func TestWriteSection_PreservesCommentsAndOrdering(t *testing.T) {
	t.Parallel()

	const original = `# top of file
dht:
  # bootstrap node
  bootstrap: router.example.com
  port: 3334
`

	path := writeTemp(t, "config.yml", original)

	if err := WriteSection(path, []string{"tmdb"}, map[string]any{"enabled": true}); err != nil {
		t.Fatalf("WriteSection: %v", err)
	}

	got := readBack(t, path)

	for _, want := range []string{"# top of file", "# bootstrap node"} {
		if !strings.Contains(got, want) {
			t.Errorf("comment lost %q:\n%s", want, got)
		}
	}

	if bootstrapAt, portAt := strings.Index(got, "bootstrap:"), strings.Index(got, "port:"); bootstrapAt > portAt {
		t.Errorf("dht key ordering changed:\n%s", got)
	}
}

func TestWriteSection_UnparseableAborts(t *testing.T) {
	t.Parallel()

	const broken = "tmdb:\n  x: [unterminated\n\tbad: tab\n"
	path := writeTemp(t, "config.yml", broken)

	err := WriteSection(path, []string{"tmdb"}, map[string]any{"enabled": true})
	if err == nil {
		t.Fatal("expected WriteSection to fail on an unparseable file")
	}

	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("expected a parse error, got: %v", err)
	}

	if got := readBack(t, path); got != broken {
		t.Errorf("file modified despite parse failure:\n%q", got)
	}
}

func TestWriteSection_AbsentFileCreated(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")

	if err := WriteSection(path, []string{"tmdb"}, map[string]any{"enabled": true}); err != nil {
		t.Fatalf("WriteSection on absent file: %v", err)
	}

	got := readBack(t, path)
	if !strings.Contains(got, "tmdb:") || !strings.Contains(got, "enabled: true") {
		t.Errorf("expected a tmdb section, got:\n%s", got)
	}

	var check map[string]any
	if err := yaml.Unmarshal([]byte(got), &check); err != nil {
		t.Errorf("created file is not valid YAML: %v", err)
	}
}

func TestWriteSection_PreservesMode(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "config.yml", "tmdb:\n  enabled: false\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := WriteSection(path, []string{"tmdb"}, map[string]any{"enabled": true}); err != nil {
		t.Fatalf("WriteSection: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode not preserved: got %o want 600", got)
	}
}

func TestWriteSection_NoTempLeftBehind(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "config.yml", "tmdb: {}\n")

	if err := WriteSection(path, []string{"tmdb"}, map[string]any{"enabled": true}); err != nil {
		t.Fatalf("WriteSection: %v", err)
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
