// Package auth provides authentication and authorization for the GraphQL API.
//
// It is deliberately small in v1: a single configured (or auto-generated)
// credential maps to a single administrative access level. The shape, however,
// is built for what upstream tracks as "Authentication, API keys, access
// levels etc." — the enforcement boundary resolves each request to a Principal
// carrying an ordered AccessLevel, and credential resolution sits behind the
// CredentialResolver interface. A future API-key store can supply principals
// by implementing that interface, without changing the middleware or any
// resolver.
package auth

import "context"

// AccessLevel is an ordered privilege tier. Higher values are strictly more
// privileged, so authorization checks are written as `have >= required`. Only
// two levels exist today; the ordering is the seam that lets a read-only tier
// slot in between without touching the comparison sites.
type AccessLevel int

const (
	// AccessLevelAnonymous is an unauthenticated request. It is the zero value
	// so that a Principal read from a context that never had one set is, by
	// default, unprivileged rather than accidentally trusted.
	AccessLevelAnonymous AccessLevel = iota
	// AccessLevelAdmin permits every operation. It is the only non-anonymous
	// level issued in v1.
	AccessLevelAdmin
)

// Principal is the authenticated identity of a request.
type Principal struct {
	AccessLevel AccessLevel
}

// CredentialResolver maps a presented credential to a Principal. Returning
// ok=false means the credential is not valid; the caller MUST NOT distinguish,
// in its response, why (missing, malformed, or wrong). The one implementation
// today compares against a static key; an API-key store is a drop-in later.
type CredentialResolver interface {
	Resolve(credential string) (principal Principal, ok bool)
}

type principalCtxKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFromContext returns the principal placed on ctx by the auth
// middleware. ok is false when no principal is present, in which case the
// returned principal is the zero value (anonymous) — callers can therefore use
// the returned principal directly and fail closed.
func PrincipalFromContext(ctx context.Context) (principal Principal, ok bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)

	return p, ok
}
