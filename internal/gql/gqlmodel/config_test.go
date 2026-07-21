package gqlmodel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/database/postgres"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab"
)

// Test string constants extracted to satisfy goconst (3+ repeated literals).
// These are test-fixture values, not domain constants — they exist only to
// keep the linter quiet without obscuring what each test asserts.
const (
	testHost           = "localhost"
	testDbName         = "bitmagnet"
	testWorkflow       = "default"
	testTmdbBaseURL    = "https://api.themoviedb.org/3"
	testMapKeyPassword = "password"
	testMapKeyAPIKey   = "api_key"
	testMapKeyToken    = "token"
	testMapKeyName     = "name"
)

// TestRedact_PostgresPassword_Redacted verifies that postgres.Config.Password
// is replaced with the redaction placeholder. This is the canonical secret
// the settings API must never leak.
func TestRedact_PostgresPassword_Redacted(t *testing.T) {
	t.Parallel()

	cfg := postgres.Config{
		Host:     testHost,
		User:     "postgres",
		Port:     5432,
		Name:     testDbName,
		Password: "super-secret-db-password-123",
		SSLMode:  "disable",
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["Password"] != RedactedValuePlaceholder {
		t.Errorf("Password not redacted: got %v", m["Password"])
	}

	if m["Host"] != testHost {
		t.Errorf("Host changed unexpectedly: got %v", m["Host"])
	}

	if m["Name"] != testDbName {
		t.Errorf("Name changed unexpectedly: got %v", m["Name"])
	}

	if m["User"] != "postgres" {
		t.Errorf("User changed unexpectedly: got %v", m["User"])
	}

	if m["Port"] != uint(5432) {
		t.Errorf("Port changed unexpectedly: got %v", m["Port"])
	}
}

// TestRedact_TmdbAPIKey_Redacted verifies that tmdb.Config.APIKey is replaced
// with the redaction placeholder. The default tmdb config ships with a real
// API key constant; this must never reach the operator's GraphQL response.
func TestRedact_TmdbAPIKey_Redacted(t *testing.T) {
	t.Parallel()

	cfg := tmdb.Config{
		Enabled:        true,
		BaseURL:        testTmdbBaseURL,
		APIKey:         "9c6689fa83ae6814fbfb200d70bba3a8",
		RateLimit:      0,
		RateLimitBurst: 5,
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["APIKey"] != RedactedValuePlaceholder {
		t.Errorf("APIKey not redacted: got %v", m["APIKey"])
	}

	if m["BaseURL"] != testTmdbBaseURL {
		t.Errorf("BaseURL changed unexpectedly: got %v", m["BaseURL"])
	}

	if v, _ := m["Enabled"].(bool); !v {
		t.Errorf("Enabled changed unexpectedly: got %v", m["Enabled"])
	}
}

// TestRedact_ClassifierProviderAPIKey_Redacted verifies the nested
// classifier.LlmConfig.ProviderAPIKey is redacted through a struct field one
// level down. This is the field that cost real debugging time per the task
// background.
func TestRedact_ClassifierProviderAPIKey_Redacted(t *testing.T) {
	t.Parallel()

	cfg := classifier.Config{
		Workflow:    testWorkflow,
		Concurrency: 10,
		Llm: classifier.LlmConfig{
			ProviderName:    "openai",
			ProviderBaseURL: "https://api.openai.com/v1",
			ProviderModel:   "gpt-4o-mini",
			ProviderAPIKey:  "sk-real-key-do-not-leak",
			BatchSize:       5,
		},
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	llm, ok := m["Llm"].(map[string]any)
	if !ok {
		t.Fatalf("Llm not a map: got %T", m["Llm"])
	}

	if llm["ProviderAPIKey"] != RedactedValuePlaceholder {
		t.Errorf("Llm.ProviderAPIKey not redacted: got %v", llm["ProviderAPIKey"])
	}

	if llm["ProviderBaseURL"] != "https://api.openai.com/v1" {
		t.Errorf("ProviderBaseURL changed unexpectedly: got %v", llm["ProviderBaseURL"])
	}

	if llm["ProviderModel"] != "gpt-4o-mini" {
		t.Errorf("ProviderModel changed unexpectedly: got %v", llm["ProviderModel"])
	}

	if m["Workflow"] != testWorkflow {
		t.Errorf("Workflow changed unexpectedly: got %v", m["Workflow"])
	}
}

// TestRedact_TorznabProfiles_NoSecretsByDefault confirms the torznab Profile
// has no sensitive fields today; if one is ever added with a sensitive name it
// will be picked up by the generic redactor. This test pins the current shape.
func TestRedact_TorznabProfiles_NoSecretsByDefault(t *testing.T) {
	t.Parallel()

	cfg := torznab.Config{
		BaseURL: "https://example.com",
		Profiles: []torznab.Profile{
			{
				ID:           testWorkflow,
				Title:        testDbName,
				DefaultLimit: 100,
				MaxLimit:     100,
				BaseURL:      model.NewNullString("https://example.com"),
			},
		},
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["BaseURL"] != "https://example.com" {
		t.Errorf("BaseURL changed unexpectedly: got %v", m["BaseURL"])
	}

	profiles, ok := m["Profiles"].([]any)
	if !ok {
		t.Fatalf("Profiles not a slice: got %T", m["Profiles"])
	}

	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	pm, ok := profiles[0].(map[string]any)
	if !ok {
		t.Fatalf("profile[0] not a map: got %T", profiles[0])
	}

	if pm["ID"] != testWorkflow {
		t.Errorf("profile ID changed: got %v", pm["ID"])
	}

	if pm["Title"] != testDbName {
		t.Errorf("profile Title changed: got %v", pm["Title"])
	}
}

// TestRedact_MapWithSecretKey verifies that a map[string]any carrying a
// credential under a sensitive key is redacted. The classifier config has
// Keywords/Extensions/Flags maps that come through configresolver; none of
// those carry secrets today, but the redactor must handle map keys correctly.
func TestRedact_MapWithSecretKey(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"host":             testHost,
		testMapKeyPassword: "leak-me",
		testMapKeyAPIKey:   "leak-me-too",
		"nested": map[string]any{
			testMapKeyToken: "leak-nested",
			testMapKeyName:  "keep-me",
		},
	}
	out := Redact(in)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m[testMapKeyPassword] != RedactedValuePlaceholder {
		t.Errorf("password not redacted: got %v", m[testMapKeyPassword])
	}

	if m[testMapKeyAPIKey] != RedactedValuePlaceholder {
		t.Errorf("api_key not redacted: got %v", m[testMapKeyAPIKey])
	}

	if m["host"] != testHost {
		t.Errorf("host changed: got %v", m["host"])
	}

	nested, ok := m["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested not a map: got %T", m["nested"])
	}

	if nested[testMapKeyToken] != RedactedValuePlaceholder {
		t.Errorf("nested.token not redacted: got %v", nested[testMapKeyToken])
	}

	if nested[testMapKeyName] != "keep-me" {
		t.Errorf("nested.name changed: got %v", nested[testMapKeyName])
	}
}

// TestRedact_PointerNotMutated verifies the input is not mutated — Redact must
// return a fresh tree, leaving the resolved config (which backs live LLM
// calls) untouched.
func TestRedact_PointerNotMutated(t *testing.T) {
	t.Parallel()

	cfg := &postgres.Config{
		Host:     testHost,
		Password: "original-secret",
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["Password"] != RedactedValuePlaceholder {
		t.Errorf("Password not redacted: got %v", m["Password"])
	}

	if cfg.Password != "original-secret" {
		t.Errorf("input was mutated: Password is now %q", cfg.Password)
	}

	if cfg.Host != testHost {
		t.Errorf("input Host was mutated: %q", cfg.Host)
	}
}

// TestIsSensitiveFieldName pins the sensitive-name matcher so future field
// additions are covered predictably. If the matcher is relaxed, this test
// fails; if it's tightened, add the new pattern here.
func TestIsSensitiveFieldName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want bool
	}{
		{testMapKeyPassword, true},
		{"Password", true},
		{"DbPassword", true},
		{testMapKeyAPIKey, true},
		{"APIKey", true},
		{"apiKey", true},
		{"ProviderAPIKey", true},
		{"secret", true},
		{"clientSecret", true},
		{testMapKeyToken, true},
		{"authToken", true},
		{"auth", true},
		{"Authorization", true},
		{"credential", true},
		{"credentials", true},
		{"key", true},
		{"privateKey", true},
		// non-sensitive names
		{"host", false},
		{"port", false},
		{testMapKeyName, false},
		{"baseURL", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsSensitiveFieldName(c.name); got != c.want {
			t.Errorf("IsSensitiveFieldName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestRedact_Nil verifies nil-safety — ResolvedNode.Value may be nil for
// sections with no resolved value.
func TestRedact_Nil(t *testing.T) {
	t.Parallel()

	if out := Redact(nil); out != nil {
		t.Errorf("Redact(nil) = %v, want nil", out)
	}

	if out := Redact((*postgres.Config)(nil)); out != nil {
		t.Errorf("Redact((*postgres.Config)(nil)) = %v, want nil", out)
	}
}

// TestRedact_NoSensitiveFields_Passthrough verifies a struct with no
// sensitive fields round-trips as a plain map with no placeholders.
func TestRedact_NoSensitiveFields_Passthrough(t *testing.T) {
	t.Parallel()

	type innocent struct {
		Host string
		Port int
		On   bool
	}

	out := Redact(innocent{Host: "h", Port: 7, On: true})
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if on, _ := m["On"].(bool); m["Host"] != "h" || m["Port"] != 7 || !on {
		t.Errorf("passthrough corrupted: %v", m)
	}

	for k, v := range m {
		if s, ok := v.(string); ok && s == RedactedValuePlaceholder {
			t.Errorf("unexpected redaction of non-sensitive field %q", k)
		}
	}
}

// keep the reflect import used in a sanity assertion even if other tests move.
var _ = reflect.TypeOf

// TestRedact_PostgresDSN_PasswordInUserinfo_Redacted pins the credential leak
// found by adversarial verification: postgres.Config.DSN is a connection
// string like "postgres://admin:HUNTER2_LEAKED@db.internal:5432/bitmagnet".
// The field name "DSN" contains no sensitive substring, so name-based
// redaction leaves it untouched — but the value embeds the DB password in
// the URI userinfo. Value-level redaction must redact ONLY the password
// portion, keeping host/port/dbname visible for diagnosis.
func TestRedact_PostgresDSN_PasswordInUserinfo_Redacted(t *testing.T) {
	t.Parallel()

	cfg := postgres.Config{
		Host: "db.internal",
		User: "admin",
		Port: 5432,
		Name: testDbName,
		DSN:  "postgres://admin:HUNTER2_LEAKED@db.internal:5432/bitmagnet?sslmode=require",
	}
	out := Redact(cfg)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}
	// Password field is name-sensitive -> full placeholder.
	if m["Password"] != RedactedValuePlaceholder {
		t.Errorf("Password not redacted: got %v", m["Password"])
	}
	// DSN field is NOT name-sensitive, but its value embeds a password in
	// userinfo -> value-level redaction rewrites only the password portion.
	dsn, ok := m["DSN"].(string)
	if !ok {
		t.Fatalf("DSN not a string: got %T", m["DSN"])
	}

	if strings.Contains(dsn, "HUNTER2_LEAKED") {
		t.Errorf("DSN leaks embedded password: %q", dsn)
	}

	if !strings.Contains(dsn, "***REDACTED***") {
		t.Errorf("DSN password not redacted: %q", dsn)
	}
	// Diagnostically-useful parts must remain visible.
	for _, want := range []string{"postgres://", "admin", "db.internal", "5432", testDbName, "sslmode=require"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN lost diagnostic part %q: %q", want, dsn)
		}
	}
}

// TestRedact_URLWithUserinfo_RedactsOnlyPassword verifies the value-level
// redactor rewrites ONLY the password portion of a URI with userinfo,
// leaving scheme, username, host, port, path, and query intact.
func TestRedact_URLWithUserinfo_RedactsOnlyPassword(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"proxy":    "http://svc:secret-token@proxy.internal:8080/upstream?timeout=30s",
		"endpoint": "amqp://guest:guest@rabbitmq:5672/vhost",
	}
	out := Redact(in)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	proxy, ok := m["proxy"].(string)
	if !ok {
		t.Fatalf("proxy not a string: got %T", m["proxy"])
	}

	if strings.Contains(proxy, "secret-token") {
		t.Errorf("proxy password leaked: %q", proxy)
	}

	for _, want := range []string{"http://", "svc", "proxy.internal", "8080", "upstream", "timeout=30s"} {
		if !strings.Contains(proxy, want) {
			t.Errorf("proxy lost diagnostic part %q: %q", want, proxy)
		}
	}

	endpoint, ok := m["endpoint"].(string)
	if !ok {
		t.Fatalf("endpoint not a string: got %T", m["endpoint"])
	}

	if strings.Contains(endpoint, "guest:guest") {
		t.Errorf("endpoint password leaked: %q", endpoint)
	}

	if !strings.Contains(endpoint, "rabbitmq") || !strings.Contains(endpoint, "5672") ||
		!strings.Contains(endpoint, "vhost") {
		t.Errorf("endpoint lost diagnostic parts: %q", endpoint)
	}
}

// TestRedact_URLWithNoUserinfo_Unchanged verifies a URL with no userinfo
// (the tmdb BaseURL shape) is returned fully visible and unredacted. This
// is the negative case for value-level redaction: no userinfo means no
// secret to redact.
func TestRedact_URLWithNoUserinfo_Unchanged(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"base_url": testTmdbBaseURL,
		"web_url":  "https://bitmagnet.io/docs?ref=abc#section",
	}
	out := Redact(in)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["base_url"] != testTmdbBaseURL {
		t.Errorf("base_url changed unexpectedly: got %v", m["base_url"])
	}

	if m["web_url"] != "https://bitmagnet.io/docs?ref=abc#section" {
		t.Errorf("web_url changed unexpectedly: got %v", m["web_url"])
	}

	for k, v := range m {
		if s, ok := v.(string); ok && strings.Contains(s, RedactedValuePlaceholder) {
			t.Errorf("non-userinfo URL %q unexpectedly redacted (field %q)", s, k)
		}
	}
}

// TestRedact_URLWithUserinfoNoPassword_Unchanged verifies a URL with a
// username but no password (e.g. postgres://admin@host/db) is returned
// unchanged — there is no secret to redact.
func TestRedact_URLWithUserinfoNoPassword_Unchanged(t *testing.T) {
	t.Parallel()

	in := map[string]any{"dsn": "postgres://admin@db.internal:5432/bitmagnet"}
	out := Redact(in)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m["dsn"] != "postgres://admin@db.internal:5432/bitmagnet" {
		t.Errorf("DSN without password changed unexpectedly: got %v", m["dsn"])
	}
}

// TestRedact_NonURLString_Unchanged verifies a string that does not parse
// as a URL is returned verbatim by the value-level redactor. Value-level
// redaction only handles URI-embedded credentials; it does not scan
// arbitrary strings for token-like content.
func TestRedact_NonURLString_Unchanged(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		testMapKeyName: "Some Torrent Release",
		"workflow":     testWorkflow,
		"random":       "not-a-url-but-contains-colon://weird",
		"empty":        "",
	}
	out := Redact(in)
	m, ok := out.(map[string]any)

	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}

	if m[testMapKeyName] != "Some Torrent Release" {
		t.Errorf("name changed: got %v", m[testMapKeyName])
	}

	if m["workflow"] != testWorkflow {
		t.Errorf("workflow changed: got %v", m["workflow"])
	}

	if m["random"] != "not-a-url-but-contains-colon://weird" {
		t.Errorf("random string changed: got %v", m["random"])
	}

	if m["empty"] != "" {
		t.Errorf("empty string changed: got %v", m["empty"])
	}
}

// TestRedact_NonCanonicalEscaping_DoesNotLeak is the regression test for a
// fail-open bug in redactStringValue. The previous implementation compared
// the raw userinfo span against url.Userinfo.String()'s canonical
// re-encoding and, when they differed, returned the ORIGINAL STRING —
// password and all.
//
// Raw and canonical differ for any source that is not canonically escaped,
// which is common in tool-generated DSNs: over-escaped characters
// ("pass%77ord" re-encodes to "password") and lowercase hex escapes
// ("%2f" re-encodes to "%2F"). Each case below leaked the full credential.
func TestRedact_NonCanonicalEscaping_DoesNotLeak(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{
			name:   "over-escaped character in password",
			in:     "postgres://admin:pass%77ord@db.internal:5432/bitmagnet",
			secret: "pass%77ord",
		},
		{
			name:   "lowercase hex escape in password",
			in:     "postgres://admin:pa%2fss@db.internal:5432/bitmagnet",
			secret: "pa%2fss",
		},
		{
			name:   "escaped at-sign in password",
			in:     "postgres://admin:p%40ssword@db.internal:5432/bitmagnet",
			secret: "p%40ssword",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, ok := Redact(map[string]any{"dsn": tc.in}).(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any")
			}

			got, ok := out["dsn"].(string)
			if !ok {
				t.Fatalf("dsn not a string: got %T", out["dsn"])
			}

			if strings.Contains(got, tc.secret) {
				t.Errorf("password leaked: %q", got)
			}

			if !strings.Contains(got, RedactedValuePlaceholder) {
				t.Errorf("no redaction placeholder present: %q", got)
			}
		})
	}
}

// TestRedact_AtSignInPath_RedactsCorrectly guards the authority-bounding
// logic. Searching the whole string for the last '@' would pick the one in
// the path and splice the placeholder over the host instead of the password.
func TestRedact_AtSignInPath_RedactsCorrectly(t *testing.T) {
	t.Parallel()

	out, ok := Redact(map[string]any{
		"dsn": "postgres://admin:hunter2@db.internal:5432/bitmagnet@v2",
	}).(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any")
	}

	got, ok := out["dsn"].(string)
	if !ok {
		t.Fatalf("dsn not a string: got %T", out["dsn"])
	}

	if strings.Contains(got, "hunter2") {
		t.Errorf("password leaked: %q", got)
	}

	// The host and the path's '@' must both survive intact.
	for _, want := range []string{"db.internal", "5432", "bitmagnet@v2"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost diagnostic part %q: %q", want, got)
		}
	}
}

// TestRedact_PasswordPresent_NeverReturnsInputVerbatim pins the invariant
// the fix establishes: once a non-empty userinfo password is confirmed, the
// function must never return its input unchanged. If a future edit adds an
// early `return s` below that point, this fails.
func TestRedact_PasswordPresent_NeverReturnsInputVerbatim(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"postgres://admin:hunter2@db.internal:5432/bitmagnet",
		"postgres://admin:pass%77ord@db.internal:5432/bitmagnet",
		"http://svc:tok@proxy.internal:8080/up?t=30s",
		"amqp://guest:guest@rabbitmq:5672/vhost",
		"redis://:justapassword@cache.internal:6379/0",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			out, ok := Redact(map[string]any{"dsn": in}).(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any")
			}

			if got := out["dsn"]; got == in {
				t.Errorf("returned input verbatim with a live password: %q", got)
			}
		})
	}
}
