package resolvers

import (
	"context"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/database/postgres"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
)

// liveResolved wraps a hand-built ResolvedConfig in the AtomicValue the
// resolver reads at request time, mirroring the configfx provider.
func liveResolved(r config.ResolvedConfig) *concurrency.AtomicValue[config.ResolvedConfig] {
	av := &concurrency.AtomicValue[config.ResolvedConfig]{}
	av.Set(r)

	return av
}

// Test section keys extracted to satisfy goconst (3+ repeated literals).
const (
	testSectionKeyPostgres   = "postgres"
	testSectionKeyClassifier = "classifier"
	testSectionKeyTmdb       = "tmdb"
	testSectionKeyMadeUp     = "made_up_section"
)

type classifierChangeability struct{}

func (classifierChangeability) IsLive(key string) bool {
	return key == testSectionKeyClassifier
}

// TestConfig_Resolver_GenericEnumeration_RedactsAllSections exercises the
// resolver's generic enumeration: it feeds a hand-built ResolvedConfig with
// three sections (postgres, tmdb, classifier) and asserts:
//   - all three sections appear (generic enumeration, no per-section code);
//   - postgres.Password, tmdb.APIKey, and classifier.LlmConfig.ProviderAPIKey
//     are redacted in the returned tree;
//   - classifier is LIVE_APPLY_AVAILABLE, the others are RESTART_REQUIRED;
//   - sections are sorted by key.
//
// This is the end-to-end proof that the settings API never leaks credentials
// and that new sections appear automatically.
func TestConfig_Resolver_GenericEnumeration_RedactsAllSections(t *testing.T) {
	t.Parallel()
	// Build a ResolvedConfig the way config.New would: NodeMap keyed by
	// section key, each Value is the resolved typed config.
	resolved := config.ResolvedConfig{
		NodeMap: map[string]config.ResolvedNode{
			testSectionKeyPostgres: {
				Spec: config.Spec{Key: testSectionKeyPostgres},
				Value: postgres.Config{
					Host:     "db.internal",
					User:     "app",
					Port:     5432,
					Name:     "bitmagnet",
					Password: "db-leak-me-please",
					SSLMode:  "require",
				},
			},
			testSectionKeyTmdb: {
				Spec: config.Spec{Key: testSectionKeyTmdb},
				Value: tmdb.Config{
					Enabled: true,
					BaseURL: "https://api.themoviedb.org/3",
					APIKey:  "tmdb-leak-me-please",
				},
			},
			testSectionKeyClassifier: {
				Spec: config.Spec{Key: testSectionKeyClassifier},
				Value: classifier.Config{
					Workflow:    "default",
					Concurrency: 10,
					Llm: classifier.LlmConfig{
						ProviderName:    "openai",
						ProviderBaseURL: "https://api.openai.com/v1",
						ProviderModel:   "gpt-4o-mini",
						ProviderAPIKey:  "llm-leak-me-please",
						BatchSize:       5,
					},
				},
			},
		},
	}
	r := &Resolver{
		ResolvedConfig: liveResolved(resolved),
		Changeability:  classifierChangeability{},
	}
	qr := &queryResolver{r}

	out, err := qr.Config(context.Background())
	if err != nil {
		t.Fatalf("Config resolver returned error: %v", err)
	}
	// Build a key->section index for assertions.
	byKey := make(map[string]gen.ConfigSection, len(out.Sections))
	for _, s := range out.Sections {
		byKey[s.Key] = s
	}

	if len(byKey) != 3 {
		t.Fatalf("expected 3 sections, got %d (%v)", len(byKey), sectionKeys(out.Sections))
	}
	// Sorted by key?
	keys := make([]string, 0, len(out.Sections))
	for _, s := range out.Sections {
		keys = append(keys, s.Key)
	}

	if keys[0] != testSectionKeyClassifier || keys[1] != testSectionKeyPostgres || keys[2] != testSectionKeyTmdb {
		t.Errorf("sections not sorted by key: got %v", keys)
	}
	// postgres: Password redacted, others preserved, RESTART_REQUIRED.
	pg, ok := byKey[testSectionKeyPostgres]
	if !ok {
		t.Fatal("postgres section missing")
	}

	pgMap, ok := pg.Value.(map[string]any)
	if !ok {
		t.Fatalf("postgres value not a map: %T", pg.Value)
	}

	if pgMap["Password"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("postgres.Password not redacted: got %v", pgMap["Password"])
	}

	if pgMap["Host"] != "db.internal" {
		t.Errorf("postgres.Host changed: got %v", pgMap["Host"])
	}

	if pg.RuntimeChangeable != gen.ConfigRuntimeChangeabilityRestartRequired {
		t.Errorf("postgres runtimeChangeable = %v, want RESTART_REQUIRED", pg.RuntimeChangeable)
	}
	// tmdb: APIKey redacted, BaseURL preserved, RESTART_REQUIRED.
	tm, ok := byKey[testSectionKeyTmdb]
	if !ok {
		t.Fatal("tmdb section missing")
	}

	tmMap, ok := tm.Value.(map[string]any)
	if !ok {
		t.Fatalf("tmdb value not a map: %T", tm.Value)
	}

	if tmMap["APIKey"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("tmdb.APIKey not redacted: got %v", tmMap["APIKey"])
	}

	if tmMap["BaseURL"] != "https://api.themoviedb.org/3" {
		t.Errorf("tmdb.BaseURL changed: got %v", tmMap["BaseURL"])
	}

	if tm.RuntimeChangeable != gen.ConfigRuntimeChangeabilityRestartRequired {
		t.Errorf("tmdb runtimeChangeable = %v, want RESTART_REQUIRED", tm.RuntimeChangeable)
	}
	// classifier: nested Llm.ProviderAPIKey redacted, workflow preserved,
	// LIVE_APPLY_AVAILABLE.
	cl, ok := byKey[testSectionKeyClassifier]
	if !ok {
		t.Fatal("classifier section missing")
	}

	clMap, ok := cl.Value.(map[string]any)
	if !ok {
		t.Fatalf("classifier value not a map: %T", cl.Value)
	}

	llm, ok := clMap["Llm"].(map[string]any)
	if !ok {
		t.Fatalf("classifier.Llm not a map: %T", clMap["Llm"])
	}

	if llm["ProviderAPIKey"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("classifier.Llm.ProviderAPIKey not redacted: got %v", llm["ProviderAPIKey"])
	}

	if llm["ProviderBaseURL"] != "https://api.openai.com/v1" {
		t.Errorf("classifier.Llm.ProviderBaseURL changed: got %v", llm["ProviderBaseURL"])
	}

	if clMap["Workflow"] != "default" {
		t.Errorf("classifier.Workflow changed: got %v", clMap["Workflow"])
	}

	if cl.RuntimeChangeable != gen.ConfigRuntimeChangeabilityLiveApplyAvailable {
		t.Errorf("classifier runtimeChangeable = %v, want LIVE_APPLY_AVAILABLE", cl.RuntimeChangeable)
	}
}

// TestConfig_Resolver_NewSectionAutoAppears verifies the generic enumeration
// claim: an arbitrary new section key registered via configfx.NewConfigModule
// would show up with no resolver changes. We simulate it by adding a section
// the resolver has never heard of.
func TestConfig_Resolver_NewSectionAutoAppears(t *testing.T) {
	t.Parallel()

	resolved := config.ResolvedConfig{
		NodeMap: map[string]config.ResolvedNode{
			testSectionKeyMadeUp: {
				Spec:  config.Spec{Key: testSectionKeyMadeUp},
				Value: map[string]any{"host": "x", "secret_token": "leak"},
			},
		},
	}
	r := &Resolver{ResolvedConfig: liveResolved(resolved)}
	qr := &queryResolver{r}

	out, err := qr.Config(context.Background())
	if err != nil {
		t.Fatalf("Config resolver returned error: %v", err)
	}

	if len(out.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(out.Sections))
	}

	s := out.Sections[0]
	if s.Key != testSectionKeyMadeUp {
		t.Errorf("section key = %q, want made_up_section", s.Key)
	}

	if s.RuntimeChangeable != gen.ConfigRuntimeChangeabilityRestartRequired {
		t.Errorf("unknown section runtimeChangeable = %v, want RESTART_REQUIRED", s.RuntimeChangeable)
	}

	m, ok := s.Value.(map[string]any)
	if !ok {
		t.Fatalf("value not a map: %T", s.Value)
	}

	if m["secret_token"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("secret_token not redacted in unknown section: got %v", m["secret_token"])
	}

	if m["host"] != "x" {
		t.Errorf("host changed in unknown section: got %v", m["host"])
	}
}

// TestConfig_Resolver_ReflectsLiveUpdate is the stage-3 acceptance test for
// the config mutation API: after a writer replaces the resolved snapshot via
// AtomicValue.Set (what the ConfigApplier will do), a subsequent read query
// returns the new value — still redacted — instead of the startup snapshot.
func TestConfig_Resolver_ReflectsLiveUpdate(t *testing.T) {
	t.Parallel()

	buildResolved := func(apiKey string) config.ResolvedConfig {
		return config.ResolvedConfig{
			NodeMap: map[string]config.ResolvedNode{
				testSectionKeyTmdb: {
					Spec: config.Spec{Key: testSectionKeyTmdb},
					Value: tmdb.Config{
						Enabled: true,
						BaseURL: "https://api.themoviedb.org/3",
						APIKey:  apiKey,
					},
				},
			},
		}
	}

	live := liveResolved(buildResolved("initial-secret"))
	r := &Resolver{ResolvedConfig: live}
	qr := &queryResolver{r}

	readTmdb := func() map[string]any {
		t.Helper()

		out, err := qr.Config(context.Background())
		if err != nil {
			t.Fatalf("Config resolver returned error: %v", err)
		}

		if len(out.Sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(out.Sections))
		}

		m, ok := out.Sections[0].Value.(map[string]any)
		if !ok {
			t.Fatalf("tmdb value not a map: %T", out.Sections[0].Value)
		}

		return m
	}

	before := readTmdb()
	if v, ok := before["Enabled"].(bool); !ok || !v {
		t.Fatalf("tmdb.Enabled = %v before update, want true", before["Enabled"])
	}

	// Simulate the applier: replace the whole snapshot with an updated section.
	updated := buildResolved("updated-secret")
	node := updated.NodeMap[testSectionKeyTmdb]
	node.Value = tmdb.Config{
		Enabled: false,
		BaseURL: "https://tmdb.example.com",
		APIKey:  "updated-secret",
	}
	updated.NodeMap[testSectionKeyTmdb] = node
	live.Set(updated)

	after := readTmdb()
	if v, ok := after["Enabled"].(bool); !ok || v {
		t.Errorf("tmdb.Enabled = %v after update, want false (stale snapshot served?)", after["Enabled"])
	}

	if after["BaseURL"] != "https://tmdb.example.com" {
		t.Errorf("tmdb.BaseURL = %v after update, want updated URL", after["BaseURL"])
	}

	if after["APIKey"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("tmdb.APIKey not redacted after live update: got %v", after["APIKey"])
	}
}

func sectionKeys(s []gen.ConfigSection) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = v.Key
	}

	return out
}
