package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const bearerPrefix = "Bearer "

// Middleware accepts a valid session, machine key, or trusted client address.
func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.Disabled() {
			setPrincipal(c, Principal{AccessLevel: AccessLevelAdmin})
			c.Next()
			return
		}

		validSession, refresh := a.ValidateSession(c.Request)
		authenticated := validSession ||
			a.ValidateAPIKey(extractCredential(c.Request)) ||
			a.TrustedBypass(c.Request)
		if !authenticated {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if refresh {
			a.SetSessionCookie(c.Writer, c.Request)
		}

		principal := Principal{AccessLevel: AccessLevelAdmin}
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
