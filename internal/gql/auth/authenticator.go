package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"

	"go.uber.org/zap"
)

// Authenticator holds the resolved authentication state for the process: either
// disabled, or enabled with an effective credential (configured or generated).
// It is constructed once at startup, where any warning or key generation is
// logged, and then only read.
type Authenticator struct {
	disabled bool
	// required is the minimum access level every gated operation demands.
	// Operations do not declare their own level in v1, so this defaults to the
	// most restrictive issued level (Admin) — the spec's "default most
	// restrictive" rule, enforced rather than stubbed.
	required AccessLevel
	resolver CredentialResolver
}

// NewAuthenticator resolves the effective auth state from cfg, logging exactly
// once at startup. It never returns an Authenticator that is silently open: the
// only unauthenticated mode is the explicit cfg.Disabled, which is logged as a
// warning.
func NewAuthenticator(cfg Config, logger *zap.SugaredLogger) (*Authenticator, error) {
	if cfg.Disabled {
		logger.Warn(
			"GraphQL API authentication is DISABLED (auth.disabled=true): " +
				"every query and mutation is open to anyone who can reach the HTTP port",
		)

		return &Authenticator{disabled: true}, nil
	}

	key := cfg.APIKey
	if key == "" {
		generated, err := generateKey()
		if err != nil {
			return nil, fmt.Errorf("auth: generate api key: %w", err)
		}

		key = generated

		logger.Warnf(
			"No auth.api_key configured; generated a temporary GraphQL API key "+
				"for this session: %s",
			key,
		)
		logger.Warn(
			"Set auth.api_key in your config (or via the web UI) to make it " +
				"permanent — the generated key changes on every restart",
		)
	}

	return &Authenticator{
		disabled: false,
		required: AccessLevelAdmin,
		resolver: newStaticKeyResolver(key),
	}, nil
}

func generateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// staticKeyResolver compares a presented credential against one fixed key.
type staticKeyResolver struct {
	// keyHash is the SHA-256 of the key. Hashing both sides to a fixed 32-byte
	// width before the constant-time compare keeps the comparison from leaking
	// the key's length (subtle.ConstantTimeCompare short-circuits on differing
	// lengths and is only constant-time for equal-length inputs).
	keyHash [32]byte
}

func newStaticKeyResolver(key string) staticKeyResolver {
	return staticKeyResolver{keyHash: sha256.Sum256([]byte(key))}
}

func (r staticKeyResolver) Resolve(credential string) (Principal, bool) {
	got := sha256.Sum256([]byte(credential))
	if subtle.ConstantTimeCompare(got[:], r.keyHash[:]) == 1 {
		return Principal{AccessLevel: AccessLevelAdmin}, true
	}

	return Principal{}, false
}
