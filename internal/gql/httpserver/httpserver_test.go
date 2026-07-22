package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/auth"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/gin-gonic/gin"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"
)

// stubSchema is a graphql.ExecutableSchema that is never actually executed in
// these tests: the 401 path aborts before Exec, and the sibling route does not
// use the gql handler. It exists only so builder.Apply -> newServer ->
// handler.New succeeds.
type stubSchema struct{}

func (stubSchema) Schema() *ast.Schema { return &ast.Schema{} }

func (stubSchema) Complexity(string, string, int, map[string]any) (int, bool) {
	return 0, false
}

func (stubSchema) Exec(context.Context) graphql.ResponseHandler {
	panic("stubSchema.Exec must not be called in these tests")
}

// gin.SetMode writes package-global state; calling it from parallel tests is a
// data race. Set it once for the whole package instead.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func applyBuilder(t *testing.T, a *auth.Authenticator) *gin.Engine {
	t.Helper()

	b := builder{
		schema: lazy.New(func() (graphql.ExecutableSchema, error) {
			return stubSchema{}, nil
		}),
		auth: a,
	}

	e := gin.New()
	if err := b.Apply(e); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// A sibling route standing in for torznab, registered WITHOUT the auth
	// middleware, on the same engine the builder just wrote to.
	e.GET("/torznab/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	return e
}

func request(e *gin.Engine, method, path string, headers map[string]string) int {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	return rec.Code
}

func enabledAuth(t *testing.T) *auth.Authenticator {
	t.Helper()

	a, err := auth.NewAuthenticator(
		auth.Config{APIKey: "s3cret"},
		configwrite.TargetPath(filepath.Join(t.TempDir(), "config.yml")),
		zap.NewNop().Sugar(),
	)
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	return a
}

// TestBuilder_GraphQLRequiresAuth: both the POST endpoint and the GET playground
// reject unauthenticated requests.
func TestBuilder_GraphQLRequiresAuth(t *testing.T) {
	t.Parallel()

	e := applyBuilder(t, enabledAuth(t))

	if code := request(e, http.MethodPost, "/graphql", nil); code != http.StatusUnauthorized {
		t.Errorf("POST /graphql unauthenticated: want 401, got %d", code)
	}

	if code := request(e, http.MethodGet, "/graphql", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /graphql (playground) unauthenticated: want 401, got %d", code)
	}
}

// TestBuilder_TorznabStaysOpen is the regression guard for the shared engine.
// The auth middleware MUST be attached per-route, not engine-wide; if a refactor
// switches to e.Use(), this sibling route would start returning 401 and break
// the *arr-stack integrations.
func TestBuilder_TorznabStaysOpen(t *testing.T) {
	t.Parallel()

	e := applyBuilder(t, enabledAuth(t))

	if code := request(e, http.MethodGet, "/torznab/ping", nil); code != http.StatusOK {
		t.Errorf("sibling non-graphql route must stay unauthenticated: got %d (engine-wide auth?)", code)
	}
}

// TestBuilder_DisabledLeavesGraphQLOpen: with auth disabled, /graphql is
// reachable without a credential (the request reaches the gql handler; we only
// assert it is not a 401).
func TestBuilder_DisabledLeavesGraphQLOpen(t *testing.T) {
	t.Parallel()

	a, err := auth.NewAuthenticator(
		auth.Config{Disabled: true},
		configwrite.TargetPath(filepath.Join(t.TempDir(), "config.yml")),
		zap.NewNop().Sugar(),
	)
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	e := applyBuilder(t, a)

	// GET /graphql serves the playground (static HTML), so with auth disabled
	// this reaches the handler and returns 200 rather than 401.
	if code := request(e, http.MethodGet, "/graphql", nil); code == http.StatusUnauthorized {
		t.Errorf("disabled auth should not 401 the playground: got %d", code)
	}
}
