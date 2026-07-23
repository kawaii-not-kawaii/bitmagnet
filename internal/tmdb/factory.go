package tmdb

import (
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/prometheus/client_golang/prometheus"
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
	Client              lazy.Lazy[Client]
	NegativeCacheEvents prometheus.Collector `group:"prometheus_collectors"`
}

func New(p Params) Result {
	// The negative cache lives outside the live-config rebuild layer
	// (requesterLazy): a tmdb config change rebuilds the requester, not this
	// wrapper, so cached entries survive.
	events := newNegativeCacheEvents()
	cache := newNegativeCache(negativeCacheTTL, negativeCacheMaxKeys, events)

	return Result{
		Client: lazy.New(func() (Client, error) {
			return &negativeCacheClient{
				Client: client{
					requester: &requesterLazy{
						config: p.Config,
						logger: p.Logger,
					},
				},
				cache: cache,
			}, nil
		}),
		NegativeCacheEvents: events,
	}
}
