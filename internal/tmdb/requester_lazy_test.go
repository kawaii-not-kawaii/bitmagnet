package tmdb

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

// fakeRequester records the config it was built from and answers requests
// without any network I/O.
type fakeRequester struct {
	apiKey string
}

func (fakeRequester) Request(
	context.Context,
	string,
	map[string]string,
	any,
) (*resty.Response, error) {
	return &resty.Response{}, nil
}

// TestRequesterLazy_RebuildsOnConfigChange is the tmdb live-apply guarantee:
// because the requester bakes the config into a cached resty client, a config
// change must trigger a rebuild rather than being ignored by a one-time
// sync.Once. It also asserts an unchanged config does NOT rebuild.
func TestRequesterLazy_RebuildsOnConfigChange(t *testing.T) {
	t.Parallel()

	av := &concurrency.AtomicValue[Config]{}
	av.Set(Config{Enabled: true, APIKey: "key-1", BaseURL: "https://tmdb"})

	var builds atomic.Int64

	var lastKey atomic.Value

	lazy := &requesterLazy{
		config: av,
		logger: zap.NewNop().Sugar(),
		build: func(_ context.Context, cfg Config, _ *zap.SugaredLogger) (Requester, error) {
			builds.Add(1)
			lastKey.Store(cfg.APIKey)

			return fakeRequester{apiKey: cfg.APIKey}, nil
		},
	}

	ctx := context.Background()

	// First request builds once, from key-1.
	if _, err := lazy.Request(ctx, "/movie", nil, nil); err != nil {
		t.Fatalf("request 1: %v", err)
	}

	if got := builds.Load(); got != 1 {
		t.Fatalf("expected 1 build, got %d", got)
	}

	// Same config: no rebuild.
	if _, err := lazy.Request(ctx, "/movie", nil, nil); err != nil {
		t.Fatalf("request 2: %v", err)
	}

	if got := builds.Load(); got != 1 {
		t.Errorf("unchanged config should not rebuild; builds=%d", got)
	}

	// Change the config at runtime: the next request rebuilds from key-2.
	av.Set(Config{Enabled: true, APIKey: "key-2", BaseURL: "https://tmdb"})

	if _, err := lazy.Request(ctx, "/movie", nil, nil); err != nil {
		t.Fatalf("request 3: %v", err)
	}

	if got := builds.Load(); got != 2 {
		t.Errorf("changed config should rebuild once; builds=%d", got)
	}

	if got := lastKey.Load(); got != "key-2" {
		t.Errorf("rebuild used stale config: apiKey=%v", got)
	}
}
