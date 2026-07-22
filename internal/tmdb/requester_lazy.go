package tmdb

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// requesterLazy defers instantiation of the requester (and possible failure)
// until the first request is made, avoiding failure when the TMDB client is not
// needed.
//
// The underlying requester bakes the config (base URL, API key, rate limits)
// into a resty client at build time, so it cannot observe a config change by
// itself. To support live-apply, requesterLazy holds the config behind an
// AtomicValue and rebuilds the requester whenever the value it was built from
// changes. tmdb.Config is all scalar fields, so the equality check is a plain
// comparison and the AtomicValue's shallow copy is safe.
type requesterLazy struct {
	mu        sync.Mutex
	config    *concurrency.AtomicValue[Config]
	logger    *zap.SugaredLogger
	built     bool
	builtFrom Config
	err       error
	requester Requester
	// build constructs a requester from a config; overridable in tests to
	// avoid the network API-key validation in newRequester. Defaults to
	// newRequester when nil.
	build func(context.Context, Config, *zap.SugaredLogger) (Requester, error)
}

func (r *requesterLazy) Request(
	ctx context.Context,
	path string,
	queryParams map[string]string,
	result interface{},
) (*resty.Response, error) {
	cfg := r.config.Get()

	requester, err := r.requesterFor(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return requester.Request(ctx, path, queryParams, result)
}

// requesterFor returns the requester built from cfg, rebuilding it if the
// current config differs from the one the cached requester was built with (or
// if none has been built yet). The lock is held across the rebuild — which
// performs a network API-key validation — but only when the config actually
// changed, so steady-state requests take the lock only briefly.
func (r *requesterLazy) requesterFor(ctx context.Context, cfg Config) (Requester, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.built || r.builtFrom != cfg {
		build := r.build
		if build == nil {
			build = newRequester
		}

		r.requester, r.err = build(ctx, cfg, r.logger)
		r.builtFrom = cfg
		r.built = true
	}

	return r.requester, r.err
}

func newRequester(ctx context.Context, config Config, logger *zap.SugaredLogger) (Requester, error) {
	if !config.Enabled {
		return nil, errors.New("TMDB is disabled")
	}

	if config.APIKey == defaultTmdbAPIKey {
		logger.Warnln(
			"you are using the default TMDB api key; TMDB requests will be limited to 1 per second; " +
				"to remove this warning please configure a personal TMDB api key",
		)

		config.RateLimit = time.Second
		config.RateLimitBurst = 8
	}

	r := requesterLogger{
		requester: requesterFailFast{
			requester: requesterSemaphore{
				requester: requesterLimiter{
					requester: requester{
						resty: resty.New().
							SetBaseURL(config.BaseURL).
							SetQueryParam("api_key", config.APIKey).
							SetRetryCount(3).
							SetRetryWaitTime(2 * time.Second).
							SetRetryMaxWaitTime(20 * time.Second).
							SetTimeout(10 * time.Second).
							EnableTrace().
							SetLogger(logger),
					},
					limiter: rate.NewLimiter(rate.Every(config.RateLimit), config.RateLimitBurst),
				},
				semaphore: semaphore.NewWeighted(2),
			},
			isUnauthorized: &concurrency.AtomicValue[bool]{},
		},
		logger: logger,
	}

	err := client{r}.ValidateAPIKey(ctx)
	if errors.Is(err, ErrUnauthorized) {
		if config.APIKey == defaultTmdbAPIKey {
			return r, fmt.Errorf("default api key is invalid: %w", err)
		}

		logger.Errorw("invalid api key, falling back to default", "error", err)

		config.APIKey = defaultTmdbAPIKey

		return newRequester(ctx, config, logger)
	}

	return r, err
}
