package tmdb

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	negativeCacheTTL     = 24 * time.Hour
	negativeCacheMaxKeys = 10_000
)

func newNegativeCacheEvents() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "bitmagnet",
		Subsystem: "tmdb",
		Name:      "negative_cache_events_total",
		Help:      "TMDB negative-cache events by type (hit, store, expired).",
	}, []string{"event"})
}

type negativeCacheEntry struct {
	key       string
	expiresAt time.Time
}

// negativeCache is a small LRU+TTL set of keys with definitively-negative
// TMDB answers. ponytail: global mutex; fine at ~3.4k lookups/hr, shard if it
// ever shows up in a profile.
type negativeCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	maxKeys int
	entries map[string]*list.Element
	order   *list.List // front = most recently used
	now     func() time.Time
	events  *prometheus.CounterVec
}

func newNegativeCache(ttl time.Duration, maxKeys int, events *prometheus.CounterVec) *negativeCache {
	return &negativeCache{
		ttl:     ttl,
		maxKeys: maxKeys,
		entries: make(map[string]*list.Element),
		order:   list.New(),
		now:     time.Now,
		events:  events,
	}
}

// get reports whether key holds an unexpired negative entry, refreshing its
// LRU position on a hit and dropping it on expiry.
func (c *negativeCache) get(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return false
	}

	entry := el.Value.(*negativeCacheEntry)
	if c.now().After(entry.expiresAt) {
		c.order.Remove(el)
		delete(c.entries, key)
		c.events.WithLabelValues("expired").Inc()

		return false
	}

	c.order.MoveToFront(el)
	c.events.WithLabelValues("hit").Inc()

	return true
}

// put stores (or refreshes) a negative entry, evicting the least recently
// used entry when over capacity.
func (c *negativeCache) put(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt := c.now().Add(c.ttl)

	if el, ok := c.entries[key]; ok {
		el.Value.(*negativeCacheEntry).expiresAt = expiresAt
		c.order.MoveToFront(el)
		c.events.WithLabelValues("store").Inc()

		return
	}

	c.entries[key] = c.order.PushFront(&negativeCacheEntry{key: key, expiresAt: expiresAt})
	c.events.WithLabelValues("store").Inc()

	for len(c.entries) > c.maxKeys {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*negativeCacheEntry).key)
	}
}

// negativeCacheClient decorates a tmdb.Client, remembering definitive
// not-found outcomes (2xx with an empty result set) for SearchMovie, SearchTv
// and FindByID and short-circuiting repeats within the TTL. Errors of any
// kind, and successful non-empty results, pass through uncached.
type negativeCacheClient struct {
	Client
	cache *negativeCache
}

func (c *negativeCacheClient) SearchMovie(
	ctx context.Context,
	request SearchMovieRequest,
) (SearchMovieResponse, error) {
	key := "search:movie:" + request.Query + ":" + request.Year.String()
	if c.cache.get(key) {
		return SearchMovieResponse{}, nil
	}

	response, err := c.Client.SearchMovie(ctx, request)
	if err == nil && len(response.Results) == 0 {
		c.cache.put(key)
	}

	return response, err
}

func (c *negativeCacheClient) SearchTv(ctx context.Context, request SearchTvRequest) (SearchTvResponse, error) {
	key := "search:tv:" + request.Query + ":" + request.FirstAirDateYear.String()
	if c.cache.get(key) {
		return SearchTvResponse{}, nil
	}

	response, err := c.Client.SearchTv(ctx, request)
	if err == nil && len(response.Results) == 0 {
		c.cache.put(key)
	}

	return response, err
}

func (c *negativeCacheClient) FindByID(ctx context.Context, request FindByIDRequest) (FindByIDResponse, error) {
	key := "find:" + request.ExternalSource + ":" + request.ExternalID
	if c.cache.get(key) {
		return FindByIDResponse{}, nil
	}

	response, err := c.Client.FindByID(ctx, request)
	if err == nil && len(response.MovieResults) == 0 && len(response.TvResults) == 0 {
		c.cache.put(key)
	}

	return response, err
}
