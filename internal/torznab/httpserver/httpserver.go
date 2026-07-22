package httpserver

import (
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/httpserver"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab"
	"github.com/gin-gonic/gin"
)

// New takes the torznab config behind an AtomicValue so a runtime config
// mutation is observed by subsequent requests: the handler reads (and
// default-merges) the current value per request rather than capturing a
// snapshot at construction.
func New(
	lazyClient lazy.Lazy[torznab.Client],
	config *concurrency.AtomicValue[torznab.Config],
) httpserver.Option {
	return builder{
		lazyClient: lazyClient,
		config:     config,
	}
}

type builder struct {
	lazyClient lazy.Lazy[torznab.Client]
	config     *concurrency.AtomicValue[torznab.Config]
}

func (builder) Key() string {
	return "torznab"
}

func (b builder) Apply(e *gin.Engine) error {
	client, err := b.lazyClient.Get()
	if err != nil {
		return err
	}

	h := handler{
		config: b.config,
		client: client,
	}
	e.GET("/torznab/*any", h.handleRequest)

	return nil
}
