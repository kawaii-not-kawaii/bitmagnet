package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

const (
	SessionCookieName = "bitmagnet_session"
	sessionSaltFile   = ".bitmagnet-session-salt"
	sessionLifetime   = 30 * 24 * time.Hour
)

type runtimeState struct {
	config          Config
	apiKey          CredentialResolver
	sessionKey      [sha256.Size]byte
	trustedNetworks []netip.Prefix
	trustedProxies  []netip.Prefix
}

// Authenticator owns the live authentication state shared by HTTP handlers.
type Authenticator struct {
	mu       sync.RWMutex
	state    runtimeState
	salt     []byte
	required AccessLevel
	now      func() time.Time
}

func NewAuthenticator(
	cfg Config,
	configPath configwrite.TargetPath,
	logger *zap.SugaredLogger,
) (*Authenticator, error) {
	salt, err := loadOrCreateSalt(filepath.Join(filepath.Dir(string(configPath)), sessionSaltFile))
	if err != nil {
		return nil, fmt.Errorf("auth: session salt: %w", err)
	}

	var resolver CredentialResolver
	if cfg.Disabled {
		logger.Warn(
			"GraphQL API authentication is DISABLED (auth.disabled=true): " +
				"every query and mutation is open to anyone who can reach the HTTP port",
		)
	} else {
		key := cfg.APIKey
		if key == "" {
			key, err = generateKey()
			if err != nil {
				return nil, fmt.Errorf("auth: generate api key: %w", err)
			}
			logger.Warnf("No auth.api_key configured; generated a temporary API key for this session: %s", key)
			logger.Warn("Set auth.api_key in your config to keep the machine credential across restarts")
		}

		resolver, err = newStaticKeyResolver(key)
		if err != nil {
			return nil, fmt.Errorf("auth: init resolver: %w", err)
		}
	}

	state, err := newRuntimeState(cfg, resolver, salt)
	if err != nil {
		return nil, err
	}

	return &Authenticator{
		state:    state,
		salt:     salt,
		required: AccessLevelAdmin,
		now:      time.Now,
	}, nil
}

func newRuntimeState(cfg Config, resolver CredentialResolver, salt []byte) (runtimeState, error) {
	if cfg.APIKey != "" {
		var err error
		resolver, err = newStaticKeyResolver(cfg.APIKey)
		if err != nil {
			return runtimeState{}, fmt.Errorf("auth: init resolver: %w", err)
		}
	}

	networks, err := parsePrefixes("trusted_networks", cfg.TrustedNetworks)
	if err != nil {
		return runtimeState{}, err
	}
	proxies, err := parsePrefixes("trusted_proxies", cfg.TrustedProxies)
	if err != nil {
		return runtimeState{}, err
	}

	var sessionKey [sha256.Size]byte
	reader := hkdf.New(sha256.New, []byte(cfg.PasswordHash), salt, []byte("bitmagnet session"))
	if _, err = io.ReadFull(reader, sessionKey[:]); err != nil {
		return runtimeState{}, fmt.Errorf("auth: derive session key: %w", err)
	}

	return runtimeState{
		config:          cfg,
		apiKey:          resolver,
		sessionKey:      sessionKey,
		trustedNetworks: networks,
		trustedProxies:  proxies,
	}, nil
}

func parsePrefixes(key string, values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, len(values))
	for i, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("auth.%s: invalid CIDR %q: %w", key, value, err)
		}
		prefixes[i] = prefix
	}
	return prefixes, nil
}

func loadOrCreateSalt(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return validateSalt(data)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	data = make([]byte, sha256.Size)
	if _, err = rand.Read(data); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return loadOrCreateSalt(path)
	}
	if err != nil {
		return nil, err
	}

	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	return data, nil
}

func validateSalt(salt []byte) ([]byte, error) {
	if len(salt) != sha256.Size {
		return nil, fmt.Errorf("invalid salt length %d", len(salt))
	}
	return salt, nil
}

func (a *Authenticator) applyConfig(cfg Config) error {
	a.mu.RLock()
	resolver := a.state.apiKey
	a.mu.RUnlock()

	state, err := newRuntimeState(cfg, resolver, a.salt)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.state = state
	a.mu.Unlock()
	return nil
}

func (a *Authenticator) snapshot() runtimeState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *Authenticator) config() Config {
	return a.snapshot().config
}

func (a *Authenticator) Disabled() bool {
	return a.snapshot().config.Disabled
}

func (a *Authenticator) NeedsSetup() bool {
	cfg := a.config()
	return !cfg.Disabled && (cfg.Username == "" || cfg.PasswordHash == "")
}

func (a *Authenticator) ValidateAPIKey(key string) bool {
	resolver := a.snapshot().apiKey
	if resolver == nil {
		return false
	}
	_, ok := resolver.Resolve(key)
	return ok
}

func (a *Authenticator) ValidatePassword(username, password string) bool {
	cfg := a.config()
	usernameOK := constantTimeStringEqual(username, cfg.Username)
	passwordOK := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(password)) == nil
	return usernameOK && passwordOK && cfg.Username != "" && cfg.PasswordHash != ""
}

func constantTimeStringEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return hmac.Equal(leftHash[:], rightHash[:])
}

func (a *Authenticator) TrustedBypass(request *http.Request) bool {
	state := a.snapshot()
	client, ok := effectiveClientIP(request, state.trustedProxies)
	return ok && containsAddress(state.trustedNetworks, client)
}

func (a *Authenticator) secureRequest(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}

	state := a.snapshot()
	direct, ok := directClientIP(request)
	if !ok || !containsAddress(state.trustedProxies, direct) {
		return false
	}

	proto, _, _ := strings.Cut(request.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func effectiveClientIP(request *http.Request, trustedProxies []netip.Prefix) (netip.Addr, bool) {
	direct, ok := directClientIP(request)
	if !ok || !containsAddress(trustedProxies, direct) {
		return direct, ok
	}

	forwarded, _, _ := strings.Cut(request.Header.Get("X-Forwarded-For"), ",")
	client, err := netip.ParseAddr(strings.TrimSpace(forwarded))
	if err != nil {
		return direct, true
	}
	return client.Unmap(), true
}

func directClientIP(request *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func containsAddress(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (a *Authenticator) SetSessionCookie(writer http.ResponseWriter, request *http.Request) {
	state := a.snapshot()
	expires := a.now().Add(sessionLifetime)
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    signSession(expires, state.sessionKey),
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   a.secureRequest(request),
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *Authenticator) ClearSessionCookie(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   a.secureRequest(request),
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *Authenticator) ValidateSession(request *http.Request) (valid, refresh bool) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil {
		return false, false
	}
	return validateSession(cookie.Value, a.snapshot().sessionKey, a.now())
}

func signSession(expires time.Time, key [sha256.Size]byte) string {
	payload := make([]byte, 8, 8+sha256.Size)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write(payload)
	payload = append(payload, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func validateSession(value string, key [sha256.Size]byte, now time.Time) (bool, bool) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(payload) != 8+sha256.Size {
		return false, false
	}

	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write(payload[:8])
	if !hmac.Equal(payload[8:], mac.Sum(nil)) {
		return false, false
	}

	expires := time.Unix(int64(binary.BigEndian.Uint64(payload[:8])), 0)
	if !expires.After(now) {
		return false, false
	}
	return true, expires.Sub(now) < sessionLifetime/2
}

func generateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

type staticKeyResolver struct {
	hmacKey []byte
	tag     []byte
}

func newStaticKeyResolver(key string) (staticKeyResolver, error) {
	hmacKey := make([]byte, 32)
	if _, err := rand.Read(hmacKey); err != nil {
		return staticKeyResolver{}, fmt.Errorf("auth: init resolver key: %w", err)
	}
	return staticKeyResolver{hmacKey: hmacKey, tag: tagOf(hmacKey, key)}, nil
}

func tagOf(hmacKey []byte, value string) []byte {
	mac := hmac.New(sha256.New, hmacKey)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (resolver staticKeyResolver) Resolve(credential string) (Principal, bool) {
	if hmac.Equal(tagOf(resolver.hmacKey, credential), resolver.tag) {
		return Principal{AccessLevel: AccessLevelAdmin}, true
	}
	return Principal{}, false
}
