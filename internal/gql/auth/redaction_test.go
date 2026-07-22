package auth_test

import (
	"strings"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/gql/auth"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel"
)

// TestConfig_APIKeyRedacted pins that the settings API will not leak the auth
// credential. The config section is served through gqlmodel.Redact, which
// matches the APIKey field name against its sensitive-field patterns
// ("apikey"). This is the single control keeping the credential out of the
// read-only settings query, so it is asserted here rather than assumed.
func TestConfig_APIKeyRedacted(t *testing.T) {
	t.Parallel()

	const secret = "super-secret-key-value"

	redacted := gqlmodel.Redact(auth.Config{APIKey: secret})

	out, ok := redacted.(map[string]any)
	if !ok {
		t.Fatalf("Redact returned %T, want map[string]any", redacted)
	}

	apiKey, _ := out["APIKey"].(string)
	if strings.Contains(apiKey, secret) {
		t.Errorf("auth api key leaked through redaction: %q", apiKey)
	}

	if apiKey != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("APIKey not redacted to the placeholder: got %q", apiKey)
	}
}
