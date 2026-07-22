package auth

// Config controls GraphQL API authentication. It is registered as the "auth"
// config section, so it appears in the read-only settings API; the APIKey field
// is redacted there because its name matches the sensitive-field patterns.
type Config struct {
	// Disabled turns authentication off entirely. It must be set explicitly —
	// there is no way to end up unauthenticated by omission. When true the
	// server logs a warning at startup and treats every request as an
	// administrator.
	Disabled bool
	// APIKey is the shared secret required on every GraphQL request, presented
	// via the X-Api-Key header or Authorization: Bearer <key>. When auth is
	// enabled and this is empty, the server generates a random key at startup
	// and logs it, rather than refusing to boot or starting open.
	APIKey string
}

// NewDefaultConfig returns the default auth config: enabled, with no key. An
// empty key on an enabled config triggers startup key generation, so the
// out-of-the-box posture is "secure and bootable" — the API is closed and a
// usable credential is printed to the log.
func NewDefaultConfig() Config {
	return Config{
		Disabled: false,
		APIKey:   "",
	}
}
