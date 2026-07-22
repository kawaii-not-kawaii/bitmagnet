package tmdbfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configfx"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb/tmdbhealthcheck"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"tmdb",
		configfx.NewConfigModule[tmdb.Config]("tmdb", tmdb.NewDefaultConfig()),
		configapply.Live[tmdb.Config]("tmdb"),
		fx.Provide(
			tmdb.New,
			tmdbhealthcheck.New,
		),
	)
}
