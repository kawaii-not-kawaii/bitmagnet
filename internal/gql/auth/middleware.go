package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const bearerPrefix = "Bearer "

// Middleware returns a gin handler that enforces authentication for the routes
// it is attached to.
//
// It MUST be attached per-route (or per-group), NOT via engine-wide gin.Use:
// the GraphQL routes share their gin.Engine with torznab, telemetry, and the
// importer, and torznab is consumed by external tools (Prowlarr, the *arr
// stack) that authenticate with their own scheme. Gating the whole engine would
// break them. See gql/httpserver.builder.Apply.
func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.disabled {
			setPrincipal(c, Principal{AccessLevel: AccessLevelAdmin})
			c.Next()

			return
		}

		principal, ok := a.resolver.Resolve(extractCredential(c.Request))
		if !ok {
			// Identical response for missing, malformed, and wrong credentials:
			// the client learns only that it is unauthenticated, never why.
			c.AbortWithStatus(http.StatusUnauthorized)

			return
		}

		if principal.AccessLevel < a.required {
			c.AbortWithStatus(http.StatusForbidden)

			return
		}

		setPrincipal(c, principal)
		c.Next()
	}
}

// setPrincipal places p on both the gin context and the underlying request
// context, so resolvers reading from the standard context.Context see it.
func setPrincipal(c *gin.Context, p Principal) {
	c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), p))
}

// extractCredential reads the credential from X-Api-Key (canonical, matching
// the *arr ecosystem) or Authorization: Bearer <key> (alias). It returns the
// empty string when neither is present; an empty credential resolves to a
// rejection like any other invalid one.
func extractCredential(r *http.Request) string {
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return k
	}

	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, bearerPrefix) {
		return strings.TrimPrefix(a, bearerPrefix)
	}

	return ""
}
