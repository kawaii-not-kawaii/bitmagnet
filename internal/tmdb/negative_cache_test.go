package tmdb

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
)

func newTestNegativeCache() *negativeCache {
	return newNegativeCache(negativeCacheTTL, negativeCacheMaxKeys, newNegativeCacheEvents())
}

func TestNegativeCache_HitWithinTTL(t *testing.T) {
	t.Parallel()

	c := newTestNegativeCache()

	if c.get("k") {
		t.Fatal("empty cache should miss")
	}

	c.put("k")

	if !c.get("k") {
		t.Fatal("expected hit within TTL")
	}
}

func TestNegativeCache_ExpiryReQueries(t *testing.T) {
	t.Parallel()

	c := newTestNegativeCache()
	now := time.Now()
	c.now = func() time.Time { return now }

	c.put("k")

	now = now.Add(negativeCacheTTL + time.Second)

	if c.get("k") {
		t.Fatal("expired entry should miss")
	}

	if got := testutil.ToFloat64(c.events.WithLabelValues("expired")); got != 1 {
		t.Errorf("expected 1 expired event, got %v", got)
	}

	// The expired entry was dropped, so a refreshed store hits again.
	c.put("k")

	if !c.get("k") {
		t.Fatal("refreshed entry should hit")
	}
}

func TestNegativeCache_LRUEviction(t *testing.T) {
	t.Parallel()

	c := newNegativeCache(negativeCacheTTL, 2, newNegativeCacheEvents())

	c.put("a")
	c.put("b")

	// Touch "a" so "b" is the LRU entry, then overflow.
	if !c.get("a") {
		t.Fatal("expected hit for a")
	}

	c.put("c")

	if c.get("b") {
		t.Error("b should have been evicted as least recently used")
	}

	if !c.get("a") || !c.get("c") {
		t.Error("a and c should survive eviction")
	}
}

func TestNegativeCache_MetricsLabels(t *testing.T) {
	t.Parallel()

	c := newTestNegativeCache()
	now := time.Now()
	c.now = func() time.Time { return now }

	c.put("k")

	if !c.get("k") {
		t.Fatal("expected hit")
	}

	now = now.Add(negativeCacheTTL + time.Second)

	c.get("k")

	for label, want := range map[string]float64{"store": 1, "hit": 1, "expired": 1} {
		if got := testutil.ToFloat64(c.events.WithLabelValues(label)); got != want {
			t.Errorf("event %q: got %v, want %v", label, got, want)
		}
	}
}

// countingClient is an inner tmdb.Client that counts calls and serves canned
// responses/errors.
type countingClient struct {
	Client
	calls         atomic.Int64
	movieResponse SearchMovieResponse
	tvResponse    SearchTvResponse
	findResponse  FindByIDResponse
	err           error
}

func (c *countingClient) SearchMovie(context.Context, SearchMovieRequest) (SearchMovieResponse, error) {
	c.calls.Add(1)
	return c.movieResponse, c.err
}

func (c *countingClient) SearchTv(context.Context, SearchTvRequest) (SearchTvResponse, error) {
	c.calls.Add(1)
	return c.tvResponse, c.err
}

func (c *countingClient) FindByID(context.Context, FindByIDRequest) (FindByIDResponse, error) {
	c.calls.Add(1)
	return c.findResponse, c.err
}

func newTestDecorator(inner Client) *negativeCacheClient {
	return &negativeCacheClient{Client: inner, cache: newTestNegativeCache()}
}

func TestNegativeCacheClient_RepeatMissShortCircuits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inner := &countingClient{}
	c := newTestDecorator(inner)

	movieReq := SearchMovieRequest{Query: "nope", Year: 2020}
	tvReq := SearchTvRequest{Query: "nope", FirstAirDateYear: 2020}
	findReq := FindByIDRequest{ExternalSource: "imdb_id", ExternalID: "tt0"}

	for range 2 {
		if _, err := c.SearchMovie(ctx, movieReq); err != nil {
			t.Fatalf("SearchMovie: %v", err)
		}

		if _, err := c.SearchTv(ctx, tvReq); err != nil {
			t.Fatalf("SearchTv: %v", err)
		}

		if _, err := c.FindByID(ctx, findReq); err != nil {
			t.Fatalf("FindByID: %v", err)
		}
	}

	if got := inner.calls.Load(); got != 3 {
		t.Errorf("repeat misses should short-circuit; inner calls=%d, want 3", got)
	}
}

func TestNegativeCacheClient_DistinctKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inner := &countingClient{}
	c := newTestDecorator(inner)

	if _, err := c.SearchMovie(ctx, SearchMovieRequest{Query: "x", Year: 2019}); err != nil {
		t.Fatalf("SearchMovie 2019: %v", err)
	}

	// Different year, and same title/year on a different method, are distinct
	// entries: each contacts the inner client.
	if _, err := c.SearchMovie(ctx, SearchMovieRequest{Query: "x", Year: 2020}); err != nil {
		t.Fatalf("SearchMovie 2020: %v", err)
	}

	if _, err := c.SearchTv(ctx, SearchTvRequest{Query: "x", FirstAirDateYear: 2019}); err != nil {
		t.Fatalf("SearchTv 2019: %v", err)
	}

	if got := inner.calls.Load(); got != 3 {
		t.Errorf("distinct params should each reach the inner client; calls=%d, want 3", got)
	}
}

func TestNegativeCacheClient_TransientFailureNotCached(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inner := &countingClient{err: errors.New("timeout")}
	c := newTestDecorator(inner)

	for range 2 {
		if _, err := c.SearchMovie(ctx, SearchMovieRequest{Query: "x"}); err == nil {
			t.Fatal("expected error passed through")
		}
	}

	if got := inner.calls.Load(); got != 2 {
		t.Errorf("errors must not be cached; inner calls=%d, want 2", got)
	}
}

func TestNegativeCacheClient_SuccessNotCached(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inner := &countingClient{
		movieResponse: SearchMovieResponse{Results: []SearchMovieResult{{ID: 1, Title: "x"}}},
	}
	c := newTestDecorator(inner)

	for range 2 {
		res, err := c.SearchMovie(ctx, SearchMovieRequest{Query: "x"})
		if err != nil {
			t.Fatalf("SearchMovie: %v", err)
		}

		if len(res.Results) != 1 {
			t.Fatalf("non-empty result should pass through; got %d results", len(res.Results))
		}
	}

	if got := inner.calls.Load(); got != 2 {
		t.Errorf("successful results must not be cached; inner calls=%d, want 2", got)
	}
}

// TestNegativeCacheClient_SurvivesConfigRebuild pins the wiring decision: the
// negative cache wraps OUTSIDE the live-config rebuild layer (requesterLazy),
// so a tmdb config change rebuilds the inner requester without wiping cached
// entries.
func TestNegativeCacheClient_SurvivesConfigRebuild(t *testing.T) {
	t.Parallel()

	av := &concurrency.AtomicValue[Config]{}
	av.Set(Config{Enabled: true, APIKey: "key-1", BaseURL: "https://tmdb"})

	var builds atomic.Int64

	c := &negativeCacheClient{
		Client: client{
			requester: &requesterLazy{
				config: av,
				logger: zap.NewNop().Sugar(),
				build: func(context.Context, Config, *zap.SugaredLogger) (Requester, error) {
					builds.Add(1)
					return fakeRequester{}, nil
				},
			},
		},
		cache: newTestNegativeCache(),
	}

	ctx := context.Background()
	req := SearchMovieRequest{Query: "nope", Year: 2020}

	// fakeRequester leaves the response empty, so this is a definitive miss.
	if _, err := c.SearchMovie(ctx, req); err != nil {
		t.Fatalf("SearchMovie 1: %v", err)
	}

	if got := builds.Load(); got != 1 {
		t.Fatalf("expected 1 build, got %d", got)
	}

	// Live config change: the next non-cached request rebuilds the requester.
	av.Set(Config{Enabled: true, APIKey: "key-2", BaseURL: "https://tmdb"})

	// The cached miss is served without touching (or rebuilding) the inner
	// client.
	if _, err := c.SearchMovie(ctx, req); err != nil {
		t.Fatalf("SearchMovie 2: %v", err)
	}

	if got := builds.Load(); got != 1 {
		t.Errorf("cache hit must not reach the rebuilt inner client; builds=%d", got)
	}

	// A different lookup passes through, rebuilds the inner requester, and the
	// original cache entry still hits afterwards.
	if _, err := c.SearchMovie(ctx, SearchMovieRequest{Query: "other", Year: 2020}); err != nil {
		t.Fatalf("SearchMovie 3: %v", err)
	}

	if got := builds.Load(); got != 2 {
		t.Errorf("config change should rebuild the inner requester; builds=%d", got)
	}

	if got := testutil.ToFloat64(c.cache.events.WithLabelValues("hit")); got != 1 {
		t.Errorf("expected 1 cache hit across the rebuild, got %v", got)
	}
}

// TestNew_WrapsNegativeCache pins that the fx factory actually installs the
// decorator.
func TestNew_WrapsNegativeCache(t *testing.T) {
	t.Parallel()

	av := &concurrency.AtomicValue[Config]{}
	av.Set(NewDefaultConfig())

	result := New(Params{Config: av, Logger: zap.NewNop().Sugar()})

	c, err := result.Client.Get()
	if err != nil {
		t.Fatalf("Client.Get: %v", err)
	}

	if _, ok := c.(*negativeCacheClient); !ok {
		t.Errorf("expected *negativeCacheClient, got %T", c)
	}

	if result.NegativeCacheEvents == nil {
		t.Error("expected a prometheus collector for negative-cache events")
	}
}
