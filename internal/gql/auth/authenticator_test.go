package auth

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStaticKeyResolver_CorrectAndWrong(t *testing.T) {
	t.Parallel()

	r := newStaticKeyResolver("correct-horse")

	if p, ok := r.Resolve("correct-horse"); !ok || p.AccessLevel != AccessLevelAdmin {
		t.Errorf("correct key should resolve to admin: ok=%v level=%v", ok, p.AccessLevel)
	}

	if _, ok := r.Resolve("wrong"); ok {
		t.Error("wrong key should not resolve")
	}

	if _, ok := r.Resolve(""); ok {
		t.Error("empty credential should not resolve against a non-empty key")
	}
}

// TestNewAuthenticator_GeneratesKeyWhenMissing asserts the secure-by-default
// posture: enabled auth with no configured key generates one, logs it, and
// enforces it — rather than starting open or refusing to boot.
func TestNewAuthenticator_GeneratesKeyWhenMissing(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core).Sugar()

	a, err := NewAuthenticator(Config{}, logger)
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	if a.disabled {
		t.Fatal("authenticator should be enabled")
	}

	// The generated key must appear in the logs, and it must be the key the
	// authenticator now enforces.
	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected a startup warning about the generated key")
	}

	var loggedKey string

	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), "generated a temporary") {
			fields := strings.Fields(e.Message)
			if len(fields) > 0 {
				loggedKey = fields[len(fields)-1]
			}

			break
		}
	}

	if loggedKey == "" {
		t.Fatalf("generated key not found in logs: %+v", entries)
	}

	if _, ok := a.resolver.Resolve(loggedKey); !ok {
		t.Error("the authenticator does not enforce the key it logged")
	}

	// A message must warn the key is temporary.
	if !anyContains(entries, "temporary") && !anyContains(entries, "changes on every restart") {
		t.Error("expected a warning that the generated key is temporary")
	}
}

func TestNewAuthenticator_DisabledLogsWarning(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)

	a, err := NewAuthenticator(Config{Disabled: true}, zap.New(core).Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	if !a.disabled {
		t.Error("expected disabled authenticator")
	}

	if !anyContains(logs.All(), "DISABLED") {
		t.Error("disabling auth should log a warning")
	}
}

func TestNewAuthenticator_UsesConfiguredKey(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)

	a, err := NewAuthenticator(Config{APIKey: "configured"}, zap.New(core).Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	if _, ok := a.resolver.Resolve("configured"); !ok {
		t.Error("configured key should be enforced")
	}

	// A configured key must not be echoed to the logs.
	if anyContains(logs.All(), "configured") {
		t.Error("configured key must not be logged")
	}
}

func TestPrincipalFromContext_AbsentIsAnonymous(t *testing.T) {
	t.Parallel()

	p, ok := PrincipalFromContext(context.Background())
	if ok {
		t.Error("bare context should report no principal")
	}

	if p.AccessLevel != AccessLevelAnonymous {
		t.Errorf("absent principal should default to anonymous, got %v", p.AccessLevel)
	}
}

func anyContains(entries []observer.LoggedEntry, substr string) bool {
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), strings.ToLower(substr)) {
			return true
		}
	}

	return false
}
