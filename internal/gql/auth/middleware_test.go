package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testEngine(authenticator *Authenticator) *gin.Engine {
	engine := gin.New()
	engine.GET("/guarded", authenticator.Middleware(), func(ctx *gin.Context) {
		principal, ok := PrincipalFromContext(ctx.Request.Context())
		if !ok || principal.AccessLevel != AccessLevelAdmin {
			ctx.Status(http.StatusInternalServerError)
			return
		}
		ctx.Status(http.StatusOK)
	})
	return engine
}

func doRequest(engine *gin.Engine, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestMiddlewareHeaderClientsAndDisabledBypass(t *testing.T) {
	t.Parallel()
	authenticator, _ := newTestAuthenticator(t, Config{APIKey: "s3cret"})
	engine := testEngine(authenticator)

	for name, headers := range map[string]map[string]string{
		"x-api-key": {"X-Api-Key": "s3cret"},
		"bearer":    {"Authorization": "Bearer s3cret"},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/guarded", nil)
			for key, value := range headers {
				request.Header.Set(key, value)
			}
			if code := doRequest(engine, request).Code; code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
		})
	}

	missing := doRequest(engine, httptest.NewRequest(http.MethodGet, "/guarded", nil))
	wrongRequest := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	wrongRequest.Header.Set("X-Api-Key", "wrong")
	wrong := doRequest(engine, wrongRequest)
	if missing.Code != http.StatusUnauthorized || wrong.Code != missing.Code ||
		wrong.Body.String() != missing.Body.String() {
		t.Fatalf("missing and wrong credentials differ: %d/%q vs %d/%q",
			missing.Code, missing.Body.String(), wrong.Code, wrong.Body.String())
	}

	disabled, _ := newTestAuthenticator(t, Config{Disabled: true})
	if code := doRequest(testEngine(disabled), httptest.NewRequest(http.MethodGet, "/guarded", nil)).Code; code != http.StatusOK {
		t.Fatalf("disabled auth status = %d, want 200", code)
	}
}

func TestMiddlewareSessionAndSlidingRefresh(t *testing.T) {
	t.Parallel()
	authenticator, _ := newTestAuthenticator(t, Config{APIKey: "key"})
	now := time.Unix(1_700_000_000, 0)
	authenticator.now = func() time.Time { return now }
	cookie := &http.Cookie{
		Name:  SessionCookieName,
		Value: signSession(now.Add(sessionLifetime/2-time.Second), authenticator.snapshot().sessionKey),
	}
	request := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	request.AddCookie(cookie)

	response := doRequest(testEngine(authenticator), request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("Set-Cookie") == "" {
		t.Fatal("past-half-life session was not refreshed")
	}
}

func TestMiddlewareTrustedNetworkAndProxyRules(t *testing.T) {
	t.Parallel()
	authenticator, _ := newTestAuthenticator(t, Config{
		APIKey:          "key",
		TrustedNetworks: []string{"10.0.0.0/8"},
		TrustedProxies:  []string{"192.0.2.0/24"},
	})
	engine := testEngine(authenticator)

	direct := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	direct.RemoteAddr = "10.1.2.3:1234"
	if code := doRequest(engine, direct).Code; code != http.StatusOK {
		t.Fatalf("trusted direct client status = %d", code)
	}

	forged := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	forged.RemoteAddr = "203.0.113.10:1234"
	forged.Header.Set("X-Forwarded-For", "10.1.2.3")
	if code := doRequest(engine, forged).Code; code != http.StatusUnauthorized {
		t.Fatalf("forged XFF status = %d, want 401", code)
	}

	proxied := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	proxied.RemoteAddr = "192.0.2.10:1234"
	proxied.Header.Set("X-Forwarded-For", "10.1.2.3")
	if code := doRequest(engine, proxied).Code; code != http.StatusOK {
		t.Fatalf("trusted proxy status = %d, want 200", code)
	}
}
