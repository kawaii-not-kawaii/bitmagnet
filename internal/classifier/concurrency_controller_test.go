package classifier

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyControllerAdmissionAndRelease(t *testing.T) {
	t.Parallel()

	controller := newConcurrencyController(Config{Concurrency: 1}, llmobs.New())
	require.NoError(t, controller.Acquire(context.Background()))

	started := make(chan struct{})
	acquired := make(chan error, 1)

	go func() {
		close(started)
		acquired <- controller.Acquire(context.Background())
	}()
	<-started

	select {
	case err := <-acquired:
		t.Fatalf("second acquire returned before release: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	controller.Release()
	require.NoError(t, <-acquired)
	controller.Release()
}

func TestConcurrencyControllerAcquireCancellation(t *testing.T) {
	t.Parallel()

	controller := newConcurrencyController(Config{Concurrency: 1}, llmobs.New())
	require.NoError(t, controller.Acquire(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, controller.Acquire(ctx), context.Canceled)
	controller.Release()
}

func TestConcurrencyControllerEffectiveClamp(t *testing.T) {
	t.Parallel()

	controller := newConcurrencyController(Config{Concurrency: 4}, llmobs.New())
	controller.SetEffective(10)
	assert.Equal(t, 4, controller.Effective())

	controller.SetEffective(0)
	assert.Equal(t, 1, controller.Effective())
}

func TestConcurrencyControllerDownscaleBelowActive(t *testing.T) {
	t.Parallel()

	controller := newConcurrencyController(Config{Concurrency: 3}, llmobs.New())
	for range 3 {
		require.NoError(t, controller.Acquire(context.Background()))
	}

	controller.SetEffective(1)

	controller.Release()
	controller.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, controller.Acquire(ctx), context.DeadlineExceeded)

	controller.Release()
	require.NoError(t, controller.Acquire(context.Background()))
	controller.Release()
}

func TestConcurrencyControllerEvaluate(t *testing.T) {
	t.Parallel()

	const baseline = 100 * time.Millisecond
	tests := []struct {
		name            string
		ceiling         int
		effective       int
		previousP95     time.Duration
		hasPreviousP95  bool
		requests        int
		latency         time.Duration
		errorCount      int
		rateLimitCount  int
		wantEffective   int
		wantPreviousP95 time.Duration
		wantBaseline    bool
	}{
		{
			name:            "sparse window holds and preserves baseline",
			ceiling:         8,
			effective:       4,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        4,
			latency:         2 * baseline,
			wantEffective:   4,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "healthy first window increases",
			ceiling:         8,
			effective:       1,
			requests:        5,
			latency:         baseline,
			wantEffective:   2,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "latency at 120 percent increases",
			ceiling:         8,
			effective:       3,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        5,
			latency:         120 * time.Millisecond,
			wantEffective:   4,
			wantPreviousP95: 120 * time.Millisecond,
			wantBaseline:    true,
		},
		{
			name:            "neutral latency holds",
			ceiling:         8,
			effective:       3,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        5,
			latency:         121 * time.Millisecond,
			wantEffective:   3,
			wantPreviousP95: 121 * time.Millisecond,
			wantBaseline:    true,
		},
		{
			name:            "latency at 150 percent decreases",
			ceiling:         8,
			effective:       7,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        5,
			latency:         150 * time.Millisecond,
			wantEffective:   3,
			wantPreviousP95: 150 * time.Millisecond,
			wantBaseline:    true,
		},
		{
			name:            "rate limit decreases before healthy signals",
			ceiling:         8,
			effective:       7,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        20,
			latency:         baseline,
			errorCount:      1,
			rateLimitCount:  1,
			wantEffective:   3,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "error rate at 10 percent decreases",
			ceiling:         8,
			effective:       6,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        10,
			latency:         baseline,
			errorCount:      1,
			wantEffective:   3,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "error rate below 5 percent increases",
			ceiling:         8,
			effective:       3,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        21,
			latency:         baseline,
			errorCount:      1,
			wantEffective:   4,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "error rate at 5 percent holds",
			ceiling:         8,
			effective:       3,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        20,
			latency:         baseline,
			errorCount:      1,
			wantEffective:   3,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
		{
			name:            "decrease floors at one",
			ceiling:         8,
			effective:       1,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        5,
			latency:         150 * time.Millisecond,
			wantEffective:   1,
			wantPreviousP95: 150 * time.Millisecond,
			wantBaseline:    true,
		},
		{
			name:            "increase caps at ceiling",
			ceiling:         4,
			effective:       4,
			previousP95:     baseline,
			hasPreviousP95:  true,
			requests:        5,
			latency:         baseline,
			wantEffective:   4,
			wantPreviousP95: baseline,
			wantBaseline:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			controller := newConcurrencyController(Config{
				Concurrency: tt.ceiling,
				AutoScale:   true,
			}, llmobs.New())
			controller.SetEffective(tt.effective)
			controller.previousP95 = tt.previousP95
			controller.hasPreviousP95 = tt.hasPreviousP95

			for i := range tt.requests {
				var observationErr error

				switch {
				case i < tt.rateLimitCount:
					observationErr = fmt.Errorf("provider: %w", llm.ErrRateLimited)
				case i < tt.errorCount:
					observationErr = errors.New("provider error")
				}

				controller.Observe(tt.latency, observationErr)
			}

			controller.evaluate()

			assert.Equal(t, tt.wantEffective, controller.Effective())
			assert.Equal(t, tt.wantPreviousP95, controller.previousP95)
			assert.Equal(t, tt.wantBaseline, controller.hasPreviousP95)
			assert.Zero(t, controller.windowRequests)
		})
	}
}

func TestConcurrencyControllerConfigure(t *testing.T) {
	t.Parallel()

	recorder := llmobs.New()
	controller := newConcurrencyController(Config{Concurrency: 8, AutoScale: true}, recorder)
	controller.Observe(time.Second, nil)
	controller.previousP95 = time.Second
	controller.hasPreviousP95 = true

	controller.Configure(10, true)
	assert.Equal(t, 1, controller.Effective(), "ceiling increase must retain effective limit")

	controller.SetEffective(7)
	controller.Configure(4, true)
	assert.Equal(t, 4, controller.Effective(), "ceiling decrease must clamp immediately")

	controller.Configure(6, false)
	assert.Equal(t, 6, controller.Effective())
	assert.Zero(t, controller.windowRequests)
	assert.False(t, controller.hasPreviousP95)

	controller.Observe(time.Second, errors.New("ignored while disabled"))
	controller.evaluate()
	assert.Equal(t, 6, controller.Effective())
	assert.Zero(t, controller.windowRequests)

	controller.Configure(9, true)
	assert.Equal(t, 1, controller.Effective())

	stats := recorder.Stats(0)
	assert.Equal(t, 9, stats.Concurrency)
	assert.Equal(t, 1, stats.EffectiveConcurrency)
}
