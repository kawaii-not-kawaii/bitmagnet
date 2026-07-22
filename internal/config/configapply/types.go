// Package configapply coordinates runtime config mutations: validate a raw
// section value, apply it live where a subsystem supports that, persist it to
// the config file, and refresh the live resolved-config snapshot the settings
// read query serves.
//
// Subsystems opt into live-apply by contributing a LiveApplier to the
// Group value group; a section with no registered applier is persist-only
// (restart required). The Applier (see applier.go) consumes the group.
package configapply

import (
	"fmt"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"go.uber.org/fx"
)

// Group is the fx value-group name LiveAppliers are collected under.
const Group = "config_live_appliers"

// LiveApplier applies a validated, decoded section value to a running
// subsystem without a restart.
type LiveApplier struct {
	// Key is the config section key ("tmdb", "log", ...).
	Key string
	// Apply applies value, which is the section's decoded Go config type
	// (the same concrete type as the section's registered default). It
	// returns an optional after func the caller must invoke once it has
	// released any serialization locks — used for slow cleanup such as
	// draining evicted LLM providers. after may be nil.
	Apply func(value any) (after func(), err error)
}

// Changeability reports whether a section has a live-apply path. The settings
// read resolver depends on this interface (implemented by the Applier), so
// the advertised runtimeChangeable label and the mutation behavior derive
// from the same applier registry.
type Changeability interface {
	IsLive(sectionKey string) bool
}

// Live returns an fx option registering the canonical live applier for a
// section whose subsystem reads its config from an AtomicValue at use-time:
// applying is simply a whole-value Set. Register it in the subsystem's
// module alongside configfx.NewConfigModule[T].
func Live[T any](key string) fx.Option {
	return fx.Provide(fx.Annotated{
		Group: Group,
		Target: func(av *concurrency.AtomicValue[T]) LiveApplier {
			return LiveApplier{
				Key: key,
				Apply: func(value any) (func(), error) {
					cfg, ok := value.(T)
					if !ok {
						return nil, fmt.Errorf(
							"configapply: section %q: expected %T, got %T", key, cfg, value)
					}

					av.Set(cfg)

					return nil, nil
				},
			}
		},
	})
}
