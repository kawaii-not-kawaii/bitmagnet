package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testEngine builds a gin engine with the authenticator's middleware guarding a
// single /guarded route whose handler records the principal it observed.
func testEngine(t *testing.T, a *Authenticator) (*gin.Engine, **Principal) {
	t.Helper()

	var seen *Principal

	e := gin.New()
	e.GET("/guarded", a.Middleware(), func(c *gin.Context) {
		if p, ok := PrincipalFromContext(c.Request.Context()); ok {
			seen = &p
		}

		c.Status(http.StatusOK)
	})

	return e, &seen
}

func authEnabled(t *testing.T, key string) *Authenticator {
	t.Helper()

	a, err := NewAuthenticator(Config{APIKey: key}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	return a
}

func do(e *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	return rec
}

func TestMiddleware_ValidKeyViaXApiKey(t *testing.T) {
	t.Parallel()

	e, seen := testEngine(t, authEnabled(t, "s3cret"))

	rec := do(e, map[string]string{"X-Api-Key": "s3cret"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	if *seen == nil || (*seen).AccessLevel != AccessLevelAdmin {
		t.Errorf("expected an admin principal on context, got %+v", *seen)
	}
}

func TestMiddleware_ValidKeyViaBearer(t *testing.T) {
	t.Parallel()

	e, _ := testEngine(t, authEnabled(t, "s3cret"))

	rec := do(e, map[string]string{"Authorization": "Bearer s3cret"})
	if rec.Code != http.StatusOK {
		t.Errorf("Bearer alias should authenticate: got %d", rec.Code)
	}
}

// TestMiddleware_MissingAndWrongAreIdentical is the anti-oracle requirement: a
// wrong credential must be indistinguishable from a missing one.
func TestMiddleware_MissingAndWrongAreIdentical(t *testing.T) {
	t.Parallel()

	e, _ := testEngine(t, authEnabled(t, "s3cret"))

	missing := do(e, nil)
	wrong := do(e, map[string]string{"X-Api-Key": "nope"})
	malformed := do(e, map[string]string{"Authorization": "Basic zzz"})

	if missing.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: want 401, got %d", missing.Code)
	}

	if wrong.Code != missing.Code || wrong.Body.String() != missing.Body.String() {
		t.Errorf("wrong credential differs from missing: %d/%q vs %d/%q",
			wrong.Code, wrong.Body.String(), missing.Code, missing.Body.String())
	}

	if malformed.Code != missing.Code || malformed.Body.String() != missing.Body.String() {
		t.Errorf("malformed credential differs from missing: %d/%q vs %d/%q",
			malformed.Code, malformed.Body.String(), missing.Code, missing.Body.String())
	}
}

// TestMiddleware_XApiKeyPreferredOverBearer pins the precedence: when both
// headers are present, X-Api-Key wins.
func TestMiddleware_XApiKeyPreferredOverBearer(t *testing.T) {
	t.Parallel()

	e, _ := testEngine(t, authEnabled(t, "right"))

	rec := do(e, map[string]string{
		"X-Api-Key":     "right",
		"Authorization": "Bearer wrong",
	})
	if rec.Code != http.StatusOK {
		t.Errorf("X-Api-Key should take precedence: got %d", rec.Code)
	}
}

func TestMiddleware_DisabledAllowsAnonymousAdmin(t *testing.T) {
	t.Parallel()

	a, err := NewAuthenticator(Config{Disabled: true}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	e, seen := testEngine(t, a)

	rec := do(e, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("disabled auth should allow unauthenticated: got %d", rec.Code)
	}

	if *seen == nil || (*seen).AccessLevel != AccessLevelAdmin {
		t.Errorf("disabled auth should still place an admin principal: %+v", *seen)
	}
}

// TestMiddleware_BelowRequiredLevelForbidden exercises the 403 path. v1 issues
// only Admin, so this uses a hand-built authenticator whose resolver returns a
// below-admin principal, proving the level check is enforced (not stubbed) and
// ready for a future read-only tier.
func TestMiddleware_BelowRequiredLevelForbidden(t *testing.T) {
	t.Parallel()

	a := &Authenticator{
		disabled: false,
		required: AccessLevelAdmin,
		resolver: fixedResolver{level: AccessLevelAnonymous, ok: true},
	}

	e, _ := testEngine(t, a)

	rec := do(e, map[string]string{"X-Api-Key": "whatever"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("principal below required level: want 403, got %d", rec.Code)
	}
}

type fixedResolver struct {
	level AccessLevel
	ok    bool
}

func (f fixedResolver) Resolve(string) (Principal, bool) {
	return Principal{AccessLevel: f.level}, f.ok
}
