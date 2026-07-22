package gqlmodel

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
)

// RedactedValuePlaceholder replaces every sensitive field's value in the
// output tree. Deliberately a visible, greppable string so it cannot be
// confused with a real credential. Aliases configapply.RedactedPlaceholder,
// which SetSection recognizes as "keep the existing value" on write-back.
const RedactedValuePlaceholder = configapply.RedactedPlaceholder

// sensitiveFieldNames is the set of (lowercased) field-name substrings that
// mark a field as sensitive. Matching is substring-based and case-insensitive
// so that compound names like "APIKey", "provider_api_key", "DbPassword",
// "authToken", "clientSecret" all match. Redaction fails closed: when in doubt
// (e.g. a name contains "key" or "secret" anywhere), the field is redacted.
//
// "key" is intentionally substring-matched — it catches APIKey, ProviderAPIKey,
// SecretKey, etc. It also matches unrelated names like "keyboard" or
// "keystore", but config fields with such names are rare in this codebase and
// the cost of over-redacting a non-sensitive field is far lower than the cost
// of leaking a credential.
//
// Known collateral: postgres.SSLKeyPath, SSLCertPath, SSLRootCertPath are
// redacted because "key" is a substring. These are file PATHS, not secrets.
// We deliberately do NOT special-case "*path" suffixes to exempt them: such a
// suffix exemption would let a future field like "apiKeyPath" or
// "secretKeyPath" slip through unredacted, which is exactly the fail-open
// case this redactor exists to prevent. Hiding a TLS key path makes the
// viewer slightly less useful for TLS debugging; leaking an API key makes it
// catastrophically wrong. The asymmetry favors keeping the redaction.
var sensitiveFieldNames = []string{
	"password",
	"secret",
	"token",
	"api_key",
	"apikey",
	"auth",
	"credential",
	"key",
}

// IsSensitiveFieldName reports whether name matches a sensitive-field pattern
// (case-insensitive substring match against sensitiveFieldNames). Exported so
// tests can pin the behavior.
func IsSensitiveFieldName(name string) bool {
	if name == "" {
		return false
	}

	lower := strings.ToLower(name)
	for _, s := range sensitiveFieldNames {
		if strings.Contains(lower, s) {
			return true
		}
	}

	return false
}

// Redact walks v reflectively and returns a new tree where every map key or
// struct field whose name matches IsSensitiveFieldName is replaced with
// RedactedValuePlaceholder. The input is not mutated. Non-sensitive scalar
// values (strings, ints, bools, durations, etc.) are passed through verbatim
// and will be serialized by the gqlgen JSON scalar.
//
// Redaction is structural, not value-based: we do NOT inspect string values
// for things that look like tokens. The threat model is "config field that
// holds a credential"; matching by field name covers every sensitive field
// present in the current config types (postgres.Password, tmdb.APIKey,
// classifier.LlmConfig.ProviderAPIKey) and any future one named sensibly.
//
// Returns nil for nil input.
func Redact(v any) any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr && !rv.IsNil() {
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Ptr {
		// nil pointer of some type — emit null
		return nil
	}

	return redactValue(rv)
}

func redactValue(rv reflect.Value) any {
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}

		return redactValue(rv.Elem())
	case reflect.Interface:
		if rv.IsNil() {
			return nil
		}

		return redactValue(rv.Elem())
	case reflect.Struct:
		return redactStruct(rv)
	case reflect.Map:
		return redactMap(rv)
	case reflect.Slice, reflect.Array:
		return redactSlice(rv)
	case reflect.String:
		// Name-based redaction has already been applied at the field/key
		// level. For strings that survive (non-sensitive field names),
		// apply value-level redaction: any URI with userinfo gets its
		// password portion redacted so credentials embedded in DSNs,
		// proxy URLs, and endpoint strings never leak. See redactStringValue.
		return redactStringValue(rv.String())
	default:
		// non-string scalars (int, bool, ...) and time.Duration (int64)
		// pass through verbatim. The gqlgen JSON scalar marshals them.
		return rv.Interface()
	}
}

func redactStruct(rv reflect.Value) map[string]any {
	t := rv.Type()
	out := make(map[string]any, t.NumField())

	for i := range t.NumField() {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			// unexported field — skip (would panic on Interface())
			continue
		}

		name := jsonName(sf)
		if IsSensitiveFieldName(name) {
			out[name] = RedactedValuePlaceholder
			continue
		}

		out[name] = redactValue(rv.Field(i))
	}

	return out
}

// jsonName returns the name a struct field would have in JSON output.
// Honors `json:"name"` tags (taking the first segment, ignoring "-"); falls
// back to the Go field name. This keeps redaction aligned with the shape an
// operator would see in a config.yml or a JSON dump.
func jsonName(sf reflect.StructField) string {
	if tag := sf.Tag.Get("json"); tag != "" {
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}

		if tag == "-" {
			// "-" means "do not serialize"; keep the Go name so the field is
			// still visible (and redacted by name) rather than silently dropped.
			return sf.Name
		}

		if tag != "" {
			return tag
		}
	}

	return sf.Name
}

func redactMap(rv reflect.Value) map[string]any {
	out := make(map[string]any, rv.Len())

	iter := rv.MapRange()
	for iter.Next() {
		k := iter.Key()
		// Only string keys are expected in config maps (configresolver uses
		// map[string]T); non-string keys fall through to k.String() which is
		// fine since non-string-keyed maps are not expected to carry sensitive
		// values by name.
		keyName := k.String()
		if IsSensitiveFieldName(keyName) {
			out[keyName] = RedactedValuePlaceholder
			continue
		}

		out[keyName] = redactValue(iter.Value())
	}

	return out
}

func redactSlice(rv reflect.Value) []any {
	if rv.Len() == 0 {
		return []any{}
	}

	out := make([]any, rv.Len())
	for i := range rv.Len() {
		out[i] = redactValue(rv.Index(i))
	}

	return out
}

// redactStringValue applies value-level (not name-level) redaction to a
// string. It catches credentials embedded in URIs that have no
// sensitive-sounding field name — the canonical case is postgres.Config.DSN,
// a connection string like "postgres://admin:hunter2@host:5432/db" whose
// field name "DSN" does not match any sensitive pattern but whose value
// embeds the DB password in the URI userinfo.
//
// The threat model: name-based redaction (redactStruct/redactMap) catches
// fields named password/secret/key/etc. It cannot catch a credential that
// lives inside a string VALUE under a benign field name. This function is
// the value-level backstop. It runs on every string that survives
// name-based redaction, so it also covers proxy URLs, torznab endpoints,
// and any future config string that embeds userinfo.
//
// Behavior:
//   - A string that parses as a URL with a non-empty password in userinfo
//     gets ONLY the password portion redacted:
//     postgres://admin:hunter2@host:5432/db
//     -> postgres://admin:***REDACTED***@host:5432/db
//     This keeps the field diagnostically useful (host/port/dbname visible)
//     which is the whole point of the settings viewer.
//   - A URL with userinfo but empty password (postgres://admin@host/db)
//     is returned UNCHANGED — there is no secret to redact.
//   - A URL with no userinfo (https://api.themoviedb.org/3) is returned
//     UNCHANGED.
//   - A string that does not parse as a URL is returned UNCHANGED. We do
//     NOT inspect non-URI strings for token-like content; the only
//     value-level threat we handle is URI-embedded credentials.
//
// Fails-closed note: the checks above establish whether a password EXISTS.
// Returning the string verbatim is safe only while the answer is "no" — a
// parse error, absent userinfo, or empty password all mean there is nothing
// to leak. Once a non-empty password is confirmed, the function is committed:
// it either splices the placeholder over the password span or redacts the
// whole value, and never returns the original string. Do not add an early
// return of s below that point.
func redactStringValue(s string) any {
	if s == "" {
		return s
	}

	u, err := url.Parse(s)
	if err != nil {
		// Not a valid URL — no userinfo to redact.
		return s
	}
	// User.Password is non-empty only when userinfo contains a password.
	// "postgres://admin@host" parses with User != nil but Password == "".
	if u.User == nil {
		return s
	}

	pw, hasPw := u.User.Password()
	if !hasPw || pw == "" {
		return s
	}

	// From here on we KNOW this string embeds a live password. Every
	// remaining exit path MUST redact: returning s verbatim would emit the
	// credential, and with no authentication on the GraphQL API this
	// function is the only control standing between the config viewer and
	// the secret.
	if redacted, ok := spliceUserinfoPassword(s); ok {
		return redacted
	}

	// The password span could not be located. Redact the entire value
	// rather than emit it — losing the host/port diagnostics is a much
	// better outcome than leaking the credential.
	return RedactedValuePlaceholder
}

// spliceUserinfoPassword replaces the password span of a URI's userinfo with
// the placeholder, operating on the RAW string so percent-escapes are
// preserved verbatim and the placeholder is not itself encoded (u.String()
// would turn "***REDACTED***" into "%2A%2A%2A...", breaking the greppable
// placeholder property).
//
// It deliberately does NOT compare the raw userinfo against url's canonical
// re-encoding. Those legitimately differ whenever the source is not
// canonically escaped — over-escaping such as "pass%77ord" (which re-encodes
// to "password"), or lowercase hex such as "%2f" — and an earlier version
// bailed out to the UNREDACTED string in exactly those cases, leaking the
// password for any tool-generated DSN that percent-encodes aggressively.
// url.Parse already guarantees the userinfo is the span between "//" and the
// authority's final "@", so the raw span is trustworthy on its own.
//
// Returns ok=false when the span cannot be located, so the caller fails closed.
func spliceUserinfoPassword(s string) (string, bool) {
	schemeEnd := strings.Index(s, "//")
	if schemeEnd < 0 {
		return "", false
	}

	authorityStart := schemeEnd + 2

	// The authority ends at the first '/', '?' or '#' after it. Bounding the
	// search matters: an '@' in the PATH (postgres://u:p@host/db@tag) would
	// otherwise be mistaken for the userinfo delimiter.
	authorityEnd := len(s)

	for i := authorityStart; i < len(s); i++ {
		if s[i] == '/' || s[i] == '?' || s[i] == '#' {
			authorityEnd = i
			break
		}
	}

	if authorityStart > authorityEnd {
		return "", false
	}

	authority := s[authorityStart:authorityEnd]

	// Userinfo runs to the LAST '@' in the authority: url.Parse tolerates a
	// raw '@' inside the password, and a host cannot contain one.
	atIdx := strings.LastIndex(authority, "@")
	if atIdx < 0 {
		return "", false
	}

	rawUserinfo := authority[:atIdx]

	// Split at the FIRST ':' — a username cannot contain one, a password can.
	colonIdx := strings.Index(rawUserinfo, ":")
	if colonIdx < 0 {
		return "", false
	}

	return s[:authorityStart] + rawUserinfo[:colonIdx] + ":" +
		RedactedValuePlaceholder + s[authorityStart+atIdx:], true
}
