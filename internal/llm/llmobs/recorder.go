package llmobs

import (
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	eventCapacity      = 500
	defaultStatsWindow = 15 * time.Minute
)

// Recorder stores recent classification events and process-lifetime counters.
type Recorder struct {
	mu sync.RWMutex

	events [eventCapacity]Event
	next   int
	count  int

	attempted        int64
	matched          int64
	unmatched        int64
	errored          int64
	skipped          int64
	promptTokens     int64
	completionTokens int64
	inFlight         int64

	perProvider map[string]ProviderStats
	metrics     recorderMetrics
}

// New creates an empty Recorder with a 500-event ring buffer.
func New() *Recorder {
	return &Recorder{
		perProvider: make(map[string]ProviderStats),
		metrics:     newRecorderMetrics(),
	}
}

// Record stores an event, updates lifetime counters, and observes its metrics.
// A zero timestamp is replaced with the current time.
func (r *Recorder) Record(e Event) {
	if r == nil {
		return
	}

	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	e.Languages = slices.Clone(e.Languages)

	r.mu.Lock()
	r.events[r.next] = e
	r.next = (r.next + 1) % eventCapacity

	if r.count < eventCapacity {
		r.count++
	}

	r.attempted++
	r.promptTokens += int64(e.PromptTokens)
	r.completionTokens += int64(e.CompletionTokens)

	switch e.Outcome {
	case OutcomeMatched:
		r.matched++
	case OutcomeUnmatched:
		r.unmatched++
	case OutcomeError:
		r.errored++
	case OutcomeSkipped:
		r.skipped++
	}

	if e.Provider != "" {
		if r.perProvider == nil {
			r.perProvider = make(map[string]ProviderStats)
		}

		provider := r.perProvider[e.Provider]
		provider.Provider = e.Provider
		provider.Attempted++

		switch e.Outcome {
		case OutcomeMatched:
			provider.Matched++
		case OutcomeUnmatched:
			provider.Unmatched++
		case OutcomeError:
			provider.Errored++
		}

		r.perProvider[e.Provider] = provider
	}

	r.mu.Unlock()

	r.metrics.record(e)
}

// Begin increments the in-flight count and returns its matching decrement.
// Callers must invoke the returned function exactly once, typically with defer.
func (r *Recorder) Begin() func() {
	if r == nil {
		return func() {}
	}

	r.mu.Lock()
	r.inFlight++
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		r.inFlight--
		r.mu.Unlock()
	}
}

// Events returns a newest-first snapshot. Non-positive and oversized limits
// return every buffered event.
func (r *Recorder) Events(limit int) []Event {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > r.count {
		limit = r.count
	}

	// Allocate at the constant ring capacity rather than the caller-provided
	// limit: the limit is already bounded by r.count <= eventCapacity, but a
	// constant-sized allocation makes that invariant obvious to static
	// analysis (CodeQL go/uncontrolled-allocation-size).
	events := make([]Event, 0, eventCapacity)

	for i := range limit {
		index := (r.next - 1 - i + eventCapacity) % eventCapacity
		event := r.events[index]
		event.Languages = slices.Clone(event.Languages)
		events = append(events, event)
	}

	return events
}

// Stats returns lifetime counters and figures for the requested recent window.
func (r *Recorder) Stats(window time.Duration) Stats {
	if r == nil {
		return Stats{}
	}

	if window <= 0 {
		window = defaultStatsWindow
	}

	now := time.Now()
	windowStart := now.Add(-window)

	r.mu.RLock()
	stats := Stats{
		Attempted:        r.attempted,
		Matched:          r.matched,
		Unmatched:        r.unmatched,
		Errored:          r.errored,
		Skipped:          r.skipped,
		PromptTokens:     r.promptTokens,
		CompletionTokens: r.completionTokens,
		InFlight:         r.inFlight,
		WindowStart:      windowStart,
		PerProvider:      make([]ProviderStats, 0, len(r.perProvider)),
	}

	for _, provider := range r.perProvider {
		stats.PerProvider = append(stats.PerProvider, provider)
	}

	buffered := make([]Event, r.count)
	oldestIndex := 0

	if r.count == eventCapacity {
		oldestIndex = r.next
	}

	for i := range r.count {
		buffered[i] = r.events[(oldestIndex+i)%eventCapacity]
	}

	r.mu.RUnlock()

	slices.SortFunc(stats.PerProvider, func(a, b ProviderStats) int {
		return strings.Compare(a.Provider, b.Provider)
	})

	effectiveStart := windowStart

	if len(buffered) == eventCapacity && buffered[0].Timestamp.After(windowStart) {
		stats.OldestBuffered = buffered[0].Timestamp
		effectiveStart = stats.OldestBuffered
	}

	durations := make([]time.Duration, 0, len(buffered))

	for _, event := range buffered {
		if event.Timestamp.Before(windowStart) {
			continue
		}

		stats.WindowAttempted++

		durations = append(durations, event.Duration)
	}

	slices.Sort(durations)
	stats.LatencyP50 = nearestRank(durations, 50)
	stats.LatencyP95 = nearestRank(durations, 95)

	effectiveWindow := now.Sub(effectiveStart)
	if effectiveWindow > 0 {
		stats.ThroughputPerMinute = float64(stats.WindowAttempted) / effectiveWindow.Minutes()
	}

	return stats
}

func nearestRank(sorted []time.Duration, percentile int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	rank := (len(sorted)*percentile + 99) / 100

	return sorted[rank-1]
}
