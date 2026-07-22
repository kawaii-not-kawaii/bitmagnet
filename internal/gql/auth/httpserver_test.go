package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	rootconfig "github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type authHTTPHarness struct {
	engine        *gin.Engine
	authenticator *Authenticator
	configPath    string
}

func newAuthHTTPHarness(t *testing.T, cfg Config) authHTTPHarness {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yml")
	authenticator, err := NewAuthenticator(cfg, configwrite.TargetPath(configPath), zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	resolved := &concurrency.AtomicValue[rootconfig.ResolvedConfig]{}
	resolved.Set(rootconfig.ResolvedConfig{NodeMap: map[string]rootconfig.ResolvedNode{
		"auth": {Type: reflect.TypeOf(cfg), Value: cfg},
	}})
	result := configapply.New(configapply.Params{
		Appliers: []configapply.LiveApplier{NewLiveApplier(authenticator)},
		Resolved: resolved,
		Validate: validator.New(),
		Path:     configwrite.TargetPath(configPath),
	})

	engine := gin.New()
	if err = NewHTTPServer(authenticator, result.Applier).Apply(engine); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return authHTTPHarness{engine: engine, authenticator: authenticator, configPath: configPath}
}

func performJSON(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthStateAndSetupLifecycle(t *testing.T) {
	t.Parallel()
	harness := newAuthHTTPHarness(t, Config{APIKey: "machine-key"})

	state := performJSON(harness.engine, http.MethodGet, "/auth/state", "")
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"needsSetup":true`) {
		t.Fatalf("fresh state = %d %s", state.Code, state.Body.String())
	}

	invalid := performJSON(
		harness.engine,
		http.MethodPost,
		"/auth/setup",
		`{"username":"admin","password":"short"}`,
	)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", invalid.Code)
	}

	setup := performJSON(
		harness.engine,
		http.MethodPost,
		"/auth/setup",
		`{"username":"admin","password":"password123"}`,
	)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup status = %d body=%s", setup.Code, setup.Body.String())
	}
	cookies := setup.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" {
		t.Fatalf("setup cookie = %+v", cookies)
	}
	if harness.authenticator.NeedsSetup() {
		t.Fatal("setup did not live-apply credentials")
	}

	persisted, err := os.ReadFile(harness.configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(persisted)
	if strings.Contains(text, "password123") || !strings.Contains(text, "password_hash:") ||
		!strings.Contains(text, "username: admin") {
		t.Fatalf("persisted auth config is wrong:\n%s", text)
	}

	repeated := performJSON(
		harness.engine,
		http.MethodPost,
		"/auth/setup",
		`{"username":"other","password":"password456"}`,
	)
	if repeated.Code != http.StatusConflict || repeated.Body.String() != `{"error":"already configured"}` {
		t.Fatalf("repeated setup = %d %s", repeated.Code, repeated.Body.String())
	}
}

func TestAuthLoginCredentialsAndLogout(t *testing.T) {
	t.Parallel()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	harness := newAuthHTTPHarness(t, Config{
		APIKey: "machine-key", Username: "admin", PasswordHash: string(hash),
	})

	for name, body := range map[string]string{
		"password": `{"username":"admin","password":"password123"}`,
		"api-key":  `{"apiKey":"machine-key"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := performJSON(harness.engine, http.MethodPost, "/auth/login", body)
			if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
				t.Fatalf("login = %d %s", response.Code, response.Body.String())
			}
		})
	}

	for _, body := range []string{
		`{"username":"admin","password":"wrong-password"}`,
		`{"apiKey":"wrong"}`,
	} {
		response := performJSON(harness.engine, http.MethodPost, "/auth/login", body)
		if response.Code != http.StatusUnauthorized || response.Header().Get("Set-Cookie") != "" ||
			response.Body.String() != `{"error":"invalid credentials"}` {
			t.Fatalf("wrong login = %d %s cookie=%q", response.Code, response.Body.String(),
				response.Header().Get("Set-Cookie"))
		}
	}

	logout := performJSON(harness.engine, http.MethodPost, "/auth/logout", `{}`)
	if logout.Code != http.StatusNoContent || !strings.Contains(logout.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout = %d cookie=%q", logout.Code, logout.Header().Get("Set-Cookie"))
	}
}

func TestAuthDisabledAndTrustedState(t *testing.T) {
	t.Parallel()
	disabled := newAuthHTTPHarness(t, Config{Disabled: true})
	login := performJSON(disabled.engine, http.MethodPost, "/auth/login", `{}`)
	if login.Code != http.StatusOK || login.Header().Get("Set-Cookie") != "" ||
		login.Body.String() != `{"authDisabled":true,"ok":true}` {
		t.Fatalf("disabled login = %d %s", login.Code, login.Body.String())
	}
	setup := performJSON(
		disabled.engine,
		http.MethodPost,
		"/auth/setup",
		`{"username":"admin","password":"password123"}`,
	)
	if setup.Code != http.StatusConflict {
		t.Fatalf("disabled setup status = %d", setup.Code)
	}

	trusted := newAuthHTTPHarness(t, Config{
		APIKey: "key", TrustedNetworks: []string{"10.0.0.0/8"}, TrustedProxies: []string{"192.0.2.0/24"},
	})
	request := httptest.NewRequest(http.MethodGet, "/auth/state", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("X-Forwarded-For", "10.2.3.4")
	recorder := httptest.NewRecorder()
	trusted.engine.ServeHTTP(recorder, request)

	var state map[string]bool
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state["trustedBypass"] {
		t.Fatalf("trusted proxy state = %s", recorder.Body.String())
	}
}

func TestLoginCookieSecureOnlyForTrustedTLSProxy(t *testing.T) {
	t.Parallel()
	harness := newAuthHTTPHarness(t, Config{
		APIKey: "machine-key", TrustedProxies: []string{"192.0.2.0/24"},
	})

	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"apiKey":"machine-key"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()
	harness.engine.ServeHTTP(recorder, request)
	if cookies := recorder.Result().Cookies(); len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted HTTPS proxy cookie = %+v", cookies)
	}

	request = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"apiKey":"machine-key"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.RemoteAddr = "203.0.113.10:1234"
	recorder = httptest.NewRecorder()
	harness.engine.ServeHTTP(recorder, request)
	if cookies := recorder.Result().Cookies(); len(cookies) != 1 || cookies[0].Secure {
		t.Fatalf("untrusted proxy cookie = %+v", cookies)
	}
}
