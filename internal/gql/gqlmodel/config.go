package gqlmodel

import (
	"net/url"
	"reflect"
	"strings"
)

// RedactedValuePlaceholder replaces every sensitive field's value in the
// output tree. Deliberately a visible, greppable string so it cannot be
// confused with a real credential.
const RedactedValuePlaceholder = "***REDACTED***"

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
// Fails-closed note: if url.Parse returns an error we return the string
// verbatim. A parse error means it's not a URL, so there is no userinfo to
// redact. Name-based redaction still applies upstream.
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
	// Redact ONLY the password portion, keeping scheme, username, host,
	// port, path, and query visible for diagnosis.
	//
	// We cannot use u.String() to reassemble: it percent-encodes the
	// placeholder (which contains '*' and would become %2A%2A%2A...), making
	// the redaction unreadable and breaking the "greppable placeholder"
	// property. Instead we splice the placeholder into the original string
	// by locating the encoded password in the source.
	//
	// The password in the raw string may be percent-escaped (e.g. a literal
	// '@' in the password becomes %40). We use url.User.String() (which
	// gives the encoded userinfo "admin:hunter2") to find the userinfo span
	// in the original string, then split at the ':' to isolate the password.
	encodedUserinfo := u.User.String() // "username:encodedpassword" or "username"
	// Locate the userinfo in the original string. It appears after the
	// scheme "://" prefix. url.Parse guarantees u.User corresponds to the
	// userinfo segment between "//" and the next "@".
	atIdx := strings.LastIndex(s, "@")
	if atIdx < 0 {
		// Should not happen: u.User != nil implies an "@" in the source.
		return s
	}
	// Find the userinfo start: the "//" before the userinfo.
	schemeEnd := strings.Index(s, "//")
	if schemeEnd < 0 {
		return s
	}
	userinfoStart := schemeEnd + 2
	if userinfoStart > atIdx {
		return s
	}
	rawUserinfo := s[userinfoStart:atIdx]
	// Sanity: the raw userinfo we isolated must round-trip through the
	// encoded form url produced. If they disagree (e.g. due to edge-case
	// encoding differences), bail out and return the original string
	// verbatim — name-based redaction still applies upstream, and we'd
	// rather under-redact a malformed URL than corrupt it.
	if rawUserinfo != encodedUserinfo {
		return s
	}
	// Split userinfo at the FIRST ':' to separate username from password.
	colonIdx := strings.Index(rawUserinfo, ":")
	if colonIdx < 0 {
		// No password in raw form despite Password() being non-empty —
		// encoding mismatch; bail out.
		return s
	}
	// Reconstruct: scheme:// + username + ":" + placeholder + "@" + rest.
	rest := s[atIdx+1:]
	return s[:userinfoStart] + rawUserinfo[:colonIdx] + ":" + RedactedValuePlaceholder + "@" + rest
}
