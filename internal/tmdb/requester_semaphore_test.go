package tmdb

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/semaphore"
)

func TestMaxConcurrentRequestsDefaultsToTwo(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int64(2), maxConcurrentRequests(Config{}))
	assert.Equal(t, int64(2), maxConcurrentRequests(Config{MaxConcurrentRequests: -1}))
	assert.Equal(t, 2, NewDefaultConfig().MaxConcurrentRequests)
}

func TestMaxConcurrentRequestsUsesConfiguredValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int64(16), maxConcurrentRequests(Config{MaxConcurrentRequests: 16}))
}

// blockingRequester holds every request until release is closed and records the
// peak number of requests in flight.
type blockingRequester struct {
	inFlight atomic.Int64
	peak     atomic.Int64
	release  chan struct{}
}

func (b *blockingRequester) Request(context.Context, string, map[string]string, any) (*resty.Response, error) {
	n := b.inFlight.Add(1)

	for {
		p := b.peak.Load()
		if n <= p || b.peak.CompareAndSwap(p, n) {
			break
		}
	}

	<-b.release
	b.inFlight.Add(-1)

	return &resty.Response{}, nil
}

func TestRequesterSemaphoreAllowsConfiguredConcurrency(t *testing.T) {
	t.Parallel()

	inner := &blockingRequester{release: make(chan struct{})}
	r := requesterSemaphore{
		requester: inner,
		semaphore: semaphore.NewWeighted(maxConcurrentRequests(Config{MaxConcurrentRequests: 5})),
	}

	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, _ = r.Request(context.Background(), "/search/tv", nil, nil)
		}()
	}

	assert.Eventually(t, func() bool { return inner.inFlight.Load() == 5 }, time.Second, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond) // give extra goroutines a chance to (wrongly) get in
	close(inner.release)
	wg.Wait()

	assert.Equal(t, int64(5), inner.peak.Load())
}
