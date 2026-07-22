package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

	resolver, err := newStaticKeyResolver(key)
	if err != nil {
		return nil, fmt.Errorf("auth: init resolver: %w", err)
	}

	return &Authenticator{
		disabled: false,
		required: AccessLevelAdmin,
		resolver: resolver,
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
//
// It compares HMAC-SHA256 tags rather than the raw strings. Two properties
// matter and neither is "password storage":
//   - Both tags are a fixed 32 bytes, so hmac.Equal (constant-time) does not
//     leak the key's length the way a raw ConstantTimeCompare would (that
//     short-circuits on differing lengths).
//   - The HMAC key is random and per-process, so the stored tag reveals nothing
//     about the credential even if it were exposed, and tags cannot be
//     precomputed across restarts.
//
// A fast hash is appropriate here: the credential is a high-entropy API key,
// not a low-entropy user password being stored for later verification.
type staticKeyResolver struct {
	hmacKey []byte
	tag     []byte
}

func newStaticKeyResolver(key string) (staticKeyResolver, error) {
	hmacKey := make([]byte, 32)
	if _, err := rand.Read(hmacKey); err != nil {
		return staticKeyResolver{}, fmt.Errorf("auth: init resolver key: %w", err)
	}

	return staticKeyResolver{
		hmacKey: hmacKey,
		tag:     tagOf(hmacKey, key),
	}, nil
}

func tagOf(hmacKey []byte, s string) []byte {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(s))

	return mac.Sum(nil)
}

func (r staticKeyResolver) Resolve(credential string) (Principal, bool) {
	if hmac.Equal(tagOf(r.hmacKey, credential), r.tag) {
		return Principal{AccessLevel: AccessLevelAdmin}, true
	}

	return Principal{}, false
}
