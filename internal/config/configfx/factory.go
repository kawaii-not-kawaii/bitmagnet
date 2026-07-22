package configfx

import (
	"errors"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"go.uber.org/fx"
)

func NewConfigModule[T any](key string, defaultValue interface{}) fx.Option {
	return fx.Module(
		"config:"+key,
		fx.Provide(
			fx.Annotated{
				Group: "config_specs",
				Target: func() config.Spec {
					return config.Spec{
						Key:          key,
						DefaultValue: defaultValue,
					}
				},
			},
		),
		fx.Provide(
			fx.Annotated{
				Target: func(r config.ResolvedConfig) (cfg T, err error) {
					v, ok := r.NodeMap[key].Value.(T)
					if !ok {
						err = errors.New("unexpected config type")
						return
					}
					return v, nil
				},
			},
		),
		// Also provide the section behind an AtomicValue, seeded with the
		// resolved value. Subsystems that support live-apply depend on this
		// instead of T and read it at use-time, so a runtime config mutation
		// (which calls Set on the same instance) is observed without a restart.
		// Sections that do not opt in keep depending on the plain T above and
		// are simply restart-required.
		fx.Provide(
			func(cfg T) *concurrency.AtomicValue[T] {
				av := &concurrency.AtomicValue[T]{}
				av.Set(cfg)

				return av
			},
		),
	)
}
