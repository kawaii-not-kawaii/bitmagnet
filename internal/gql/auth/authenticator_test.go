package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/bcrypt"
)

func newTestAuthenticator(t *testing.T, cfg Config) (*Authenticator, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yml")
	authenticator, err := NewAuthenticator(cfg, configwrite.TargetPath(configPath), zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}
	return authenticator, configPath
}

func TestStaticKeyResolver(t *testing.T) {
	t.Parallel()
	resolver, err := newStaticKeyResolver("correct-horse")
	if err != nil {
		t.Fatalf("newStaticKeyResolver: %v", err)
	}
	if principal, ok := resolver.Resolve("correct-horse"); !ok || principal.AccessLevel != AccessLevelAdmin {
		t.Fatalf("correct key did not resolve to admin: ok=%v principal=%+v", ok, principal)
	}
	if _, ok := resolver.Resolve("wrong"); ok {
		t.Fatal("wrong key resolved")
	}
}

func TestNewAuthenticatorGeneratesTemporaryKey(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.WarnLevel)
	configPath := filepath.Join(t.TempDir(), "config.yml")
	authenticator, err := NewAuthenticator(Config{}, configwrite.TargetPath(configPath), zap.New(core).Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	var loggedKey string
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "generated a temporary API key") {
			parts := strings.Fields(entry.Message)
			loggedKey = parts[len(parts)-1]
		}
	}
	if loggedKey == "" || !authenticator.ValidateAPIKey(loggedKey) {
		t.Fatalf("generated key missing or invalid: %q", loggedKey)
	}
}

func TestSessionSaltPersistsWithPrivateMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := configwrite.TargetPath(filepath.Join(dir, "config.yml"))
	hash, err := bcrypt.GenerateFromPassword([]byte("password-one"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{APIKey: "key", Username: "admin", PasswordHash: string(hash)}
	first, err := NewAuthenticator(cfg, configPath, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	first.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	first.SetSessionCookie(recorder, request)
	cookie := recorder.Result().Cookies()[0]

	second, err := NewAuthenticator(cfg, configPath, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	second.now = first.now
	request.AddCookie(cookie)
	if valid, _ := second.ValidateSession(request); !valid {
		t.Fatal("session did not survive authenticator reconstruction")
	}

	info, err := os.Stat(filepath.Join(dir, sessionSaltFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("salt mode = %o, want 600", mode)
	}
}

func TestSessionForgeryExpiryAndPasswordChange(t *testing.T) {
	t.Parallel()
	hash, err := bcrypt.GenerateFromPassword([]byte("password-one"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, _ := newTestAuthenticator(t, Config{
		APIKey: "key", Username: "admin", PasswordHash: string(hash),
	})
	now := time.Unix(1_700_000_000, 0)
	authenticator.now = func() time.Time { return now }
	key := authenticator.snapshot().sessionKey

	validValue := signSession(now.Add(time.Hour), key)
	request := httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: validValue})
	if valid, _ := authenticator.ValidateSession(request); !valid {
		t.Fatal("valid session rejected")
	}

	forged := []byte(validValue)
	forged[0] ^= 1
	request = httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: string(forged)})
	if valid, _ := authenticator.ValidateSession(request); valid {
		t.Fatal("forged session accepted")
	}

	request = httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: signSession(now.Add(-time.Second), key)})
	if valid, _ := authenticator.ValidateSession(request); valid {
		t.Fatal("expired session accepted")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("password-two"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := authenticator.config()
	cfg.PasswordHash = string(newHash)
	if err = authenticator.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: validValue})
	if valid, _ := authenticator.ValidateSession(request); valid {
		t.Fatal("old session survived password change")
	}
}

func anyContains(entries []observer.LoggedEntry, substring string) bool {
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(substring)) {
			return true
		}
	}
	return false
}
