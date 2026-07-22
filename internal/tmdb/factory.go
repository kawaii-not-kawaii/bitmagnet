package tmdb

import (
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config *concurrency.AtomicValue[Config]
	Logger *zap.SugaredLogger
}

type Result struct {
	fx.Out
	Client lazy.Lazy[Client]
}

func New(p Params) Result {
	return Result{
		Client: lazy.New(func() (Client, error) {
			return client{
				requester: &requesterLazy{
					config: p.Config,
					logger: p.Logger,
				},
			}, nil
		}),
	}
}
