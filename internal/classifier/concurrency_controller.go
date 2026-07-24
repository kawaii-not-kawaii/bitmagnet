package classifier

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"go.uber.org/fx"
)

const (
	autoscaleInterval       = 10 * time.Second
	minimumAutoscaleSamples = 5
)

// ConcurrencyController gates classifier work and adjusts its effective limit.
type ConcurrencyController struct {
	mu sync.Mutex

	ceiling   int
	effective int
	active    int
	autoScale bool
	notify    chan struct{}

	windowRequests   int
	windowErrors     int
	windowRateLimits int
	windowLatencies  []time.Duration
	previousP95      time.Duration
	hasPreviousP95   bool

	recorder *llmobs.Recorder
	stop     chan struct{}
	done     chan struct{}
}

// NewConcurrencyController creates the shared classifier concurrency controller.
func NewConcurrencyController(
	config Config,
	recorder *llmobs.Recorder,
	lifecycle fx.Lifecycle,
) *ConcurrencyController {
	controller := newConcurrencyController(config, recorder)

	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go controller.run()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(controller.stop)

			select {
			case <-controller.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	return controller
}

func newConcurrencyController(config Config, recorder *llmobs.Recorder) *ConcurrencyController {
	ceiling := max(1, config.Concurrency)

	effective := ceiling
	if config.AutoScale {
		effective = 1
	}

	controller := &ConcurrencyController{
		ceiling:   ceiling,
		effective: effective,
		autoScale: config.AutoScale,
		notify:    make(chan struct{}),
		recorder:  recorder,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	controller.updateStatsLocked()

	return controller
}

// Acquire waits until the effective limit admits another classifier run.
func (c *ConcurrencyController) Acquire(ctx context.Context) error {
	for {
		c.mu.Lock()
		if c.active < c.effective {
			c.active++
			c.mu.Unlock()

			return nil
		}

		notify := c.notify
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

// Release completes one admitted classifier run.
func (c *ConcurrencyController) Release() {
	c.mu.Lock()
	if c.active > 0 {
		c.active--
	}

	c.notifyLocked()
	c.mu.Unlock()
}

// SetEffective updates the admission limit, clamped to the configured ceiling.
func (c *ConcurrencyController) SetEffective(limit int) {
	c.mu.Lock()
	c.setEffectiveLocked(limit)
	c.notifyLocked()
	c.updateStatsLocked()
	c.mu.Unlock()
}

// Configure applies a new ceiling and autoscaling kill-switch state.
func (c *ConcurrencyController) Configure(ceiling int, autoScale bool) {
	c.mu.Lock()
	ceiling = max(1, ceiling)

	switch {
	case !autoScale:
		c.ceiling = ceiling
		c.autoScale = false
		c.effective = ceiling
		c.clearAutoscaleStateLocked()
	case !c.autoScale:
		c.ceiling = ceiling
		c.autoScale = true
		c.effective = 1
		c.clearAutoscaleStateLocked()
	default:
		c.ceiling = ceiling
		c.effective = min(c.effective, ceiling)
	}

	c.notifyLocked()
	c.updateStatsLocked()
	c.mu.Unlock()
}

// Configured returns the current configured concurrency ceiling.
func (c *ConcurrencyController) Configured() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ceiling
}

// Effective returns the current admission limit.
func (c *ConcurrencyController) Effective() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.effective
}

// Observe adds one completed LLM request to the current autoscaling window.
func (c *ConcurrencyController) Observe(latency time.Duration, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.autoScale {
		return
	}

	c.windowRequests++

	c.windowLatencies = append(c.windowLatencies, latency)
	if err != nil {
		c.windowErrors++
	}

	if errors.Is(err, llm.ErrRateLimited) {
		c.windowRateLimits++
	}
}

func (c *ConcurrencyController) run() {
	ticker := time.NewTicker(autoscaleInterval)
	defer ticker.Stop()
	defer close(c.done)

	for {
		select {
		case <-ticker.C:
			c.evaluate()
		case <-c.stop:
			return
		}
	}
}

func (c *ConcurrencyController) evaluate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.autoScale {
		c.clearAutoscaleStateLocked()

		return
	}

	if c.windowRequests < minimumAutoscaleSamples {
		c.clearWindowLocked()

		return
	}

	slices.Sort(c.windowLatencies)
	p95 := nearestRankDuration(c.windowLatencies, 95)
	decrease := c.windowRateLimits > 0 ||
		c.windowErrors*10 >= c.windowRequests ||
		c.hasPreviousP95 && float64(p95) >= 1.5*float64(c.previousP95)
	increase := c.windowRateLimits == 0 &&
		c.windowErrors*20 < c.windowRequests &&
		(!c.hasPreviousP95 || float64(p95) <= 1.2*float64(c.previousP95))

	switch {
	case decrease:
		c.setEffectiveLocked(max(1, c.effective/2))
	case increase:
		c.setEffectiveLocked(min(c.ceiling, c.effective+1))
	}

	c.previousP95 = p95
	c.hasPreviousP95 = true
	c.clearWindowLocked()
	c.notifyLocked()
	c.updateStatsLocked()
}

func (c *ConcurrencyController) setEffectiveLocked(limit int) {
	c.effective = max(1, min(limit, c.ceiling))
}

func (c *ConcurrencyController) notifyLocked() {
	close(c.notify)
	c.notify = make(chan struct{})
}

func (c *ConcurrencyController) clearWindowLocked() {
	c.windowRequests = 0
	c.windowErrors = 0
	c.windowRateLimits = 0
	c.windowLatencies = nil
}

func (c *ConcurrencyController) clearAutoscaleStateLocked() {
	c.clearWindowLocked()
	c.previousP95 = 0
	c.hasPreviousP95 = false
}

func (c *ConcurrencyController) updateStatsLocked() {
	c.recorder.SetConcurrency(c.ceiling, c.effective)
}

func nearestRankDuration(sorted []time.Duration, percentile int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	rank := (len(sorted)*percentile + 99) / 100

	return sorted[rank-1]
}
