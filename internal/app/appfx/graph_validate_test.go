package appfx_test

import (
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/app/appfx"
	"github.com/bitmagnet-io/bitmagnet/internal/logging/loggingfx"
	"go.uber.org/fx"
)

// TestGraphValidates checks the full DI graph is wire-able: every dependency is
// provided exactly once and there are no missing/duplicate providers. It does
// NOT run lifecycle hooks (no DB, no listeners), so it is a pure static check —
// enough to catch a broken fx wiring introduced by the auth provider/config.
func TestGraphValidates(t *testing.T) {
	if err := fx.ValidateApp(
		appfx.New(),
		loggingfx.WithLogger(),
	); err != nil {
		t.Fatalf("fx graph does not validate: %v", err)
	}
}
