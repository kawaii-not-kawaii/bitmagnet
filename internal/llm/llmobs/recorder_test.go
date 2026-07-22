package llmobs

import (
	"math"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRecorder_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	const (
		writers          = 8
		readers          = 4
		recordsPerWriter = 200
		readsPerReader   = 200
	)

	r := New()
	start := make(chan struct{})

	var workers sync.WaitGroup

	for range writers {
		workers.Add(1)

		go func() {
			defer workers.Done()
			<-start

			for range recordsPerWriter {
				r.Record(Event{Provider: "provider", Outcome: OutcomeMatched})
			}
		}()
	}

	for range readers {
		workers.Add(1)

		go func() {
			defer workers.Done()
			<-start

			for range readsPerReader {
				_ = r.Events(50)
				_ = r.Stats(time.Hour)
			}
		}()
	}

	close(start)
	workers.Wait()

	stats := r.Stats(time.Hour)
	want := int64(writers * recordsPerWriter)

	if stats.Attempted != want {
		t.Errorf("attempted = %d, want %d", stats.Attempted, want)
	}

	if stats.PerProvider[0].Attempted != want {
		t.Errorf("provider attempted = %d, want %d", stats.PerProvider[0].Attempted, want)
	}

	if got := len(r.Events(0)); got != eventCapacity {
		t.Errorf("buffered events = %d, want %d", got, eventCapacity)
	}
}

func TestRecorder_Wraparound(t *testing.T) {
	t.Parallel()

	r := New()
	base := time.Now().Add(-time.Hour)

	for i := range eventCapacity + 1 {
		r.Record(Event{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			InfoHash:  strconv.Itoa(i),
			Outcome:   OutcomeMatched,
		})
	}

	events := r.Events(0)
	if len(events) != eventCapacity {
		t.Fatalf("events = %d, want %d", len(events), eventCapacity)
	}

	if events[0].InfoHash != strconv.Itoa(eventCapacity) {
		t.Errorf("newest info hash = %q, want %q", events[0].InfoHash, strconv.Itoa(eventCapacity))
	}

	if events[len(events)-1].InfoHash != "1" {
		t.Errorf("oldest info hash = %q, want 1", events[len(events)-1].InfoHash)
	}
}

func TestRecorder_EventsLimitsAndCopies(t *testing.T) {
	t.Parallel()

	r := New()
	base := time.Now().Add(-time.Minute)
	languages := []string{"en"}

	for i := 1; i <= 3; i++ {
		event := Event{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			InfoHash:  strconv.Itoa(i),
			Outcome:   OutcomeMatched,
		}

		if i == 3 {
			event.Languages = languages
		}

		r.Record(event)
	}

	languages[0] = "fr"

	tests := []struct {
		name  string
		limit int
		want  []string
	}{
		{name: "positive", limit: 2, want: []string{"3", "2"}},
		{name: "zero", limit: 0, want: []string{"3", "2", "1"}},
		{name: "negative", limit: -1, want: []string{"3", "2", "1"}},
		{name: "oversized", limit: 10, want: []string{"3", "2", "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events := r.Events(tt.limit)
			got := make([]string, len(events))

			for i := range events {
				got[i] = events[i].InfoHash
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("info hashes = %v, want %v", got, tt.want)
			}
		})
	}

	events := r.Events(1)
	if events[0].Languages[0] != "en" {
		t.Fatalf("stored languages = %v, want [en]", events[0].Languages)
	}

	events[0].Languages[0] = "de"

	if got := r.Events(1)[0].Languages[0]; got != "en" {
		t.Errorf("stored language after snapshot mutation = %q, want en", got)
	}
}

func TestRecorder_Stats(t *testing.T) {
	t.Parallel()

	const window = 10 * time.Minute

	r := New()
	base := time.Now().Add(-5 * time.Minute)
	events := []Event{
		{Timestamp: base, Provider: "alpha", Outcome: OutcomeMatched, Duration: time.Second},
		{Timestamp: base.Add(time.Second), Provider: "beta", Outcome: OutcomeError, Duration: 2 * time.Second},
		{
			Timestamp: base.Add(2 * time.Second),
			Provider:  "alpha",
			Outcome:   OutcomeUnmatched,
			Duration:  3 * time.Second,
		},
		{
			Timestamp: base.Add(3 * time.Second),
			Provider:  "alpha",
			Outcome:   OutcomeMatched,
			Duration:  4 * time.Second,
		},
		{Timestamp: base.Add(4 * time.Second), Outcome: OutcomeSkipped, Duration: 100 * time.Second},
	}

	for _, event := range events {
		r.Record(event)
	}

	stats := r.Stats(window)
	if stats.Attempted != 5 || stats.Matched != 2 || stats.Unmatched != 1 || stats.Errored != 1 ||
		stats.Skipped != 1 {
		t.Errorf("lifetime counts = %#v", stats)
	}

	if stats.WindowAttempted != len(events) {
		t.Errorf("window attempted = %d, want %d", stats.WindowAttempted, len(events))
	}

	if stats.LatencyP50 != 3*time.Second {
		t.Errorf("p50 = %v, want 3s", stats.LatencyP50)
	}

	if stats.LatencyP95 != 100*time.Second {
		t.Errorf("p95 = %v, want 100s", stats.LatencyP95)
	}

	if math.Abs(stats.ThroughputPerMinute-0.5) > 1e-9 {
		t.Errorf("throughput = %v, want 0.5", stats.ThroughputPerMinute)
	}

	if !stats.OldestBuffered.IsZero() {
		t.Errorf("oldest buffered = %v, want zero", stats.OldestBuffered)
	}

	wantProviders := []ProviderStats{
		{Provider: "alpha", Attempted: 3, Matched: 2, Unmatched: 1},
		{Provider: "beta", Attempted: 1, Errored: 1},
	}

	if !reflect.DeepEqual(stats.PerProvider, wantProviders) {
		t.Errorf("per provider = %#v, want %#v", stats.PerProvider, wantProviders)
	}
}

func TestRecorder_TokenUsage(t *testing.T) {
	t.Parallel()

	r := New()
	r.Record(Event{PromptTokens: 100, CompletionTokens: 20})
	r.Record(Event{PromptTokens: 50, CompletionTokens: 10})
	r.Record(Event{})

	stats := r.Stats(time.Minute)
	if stats.PromptTokens != 150 || stats.CompletionTokens != 30 {
		t.Errorf(
			"token totals = %d prompt/%d completion, want 150/30",
			stats.PromptTokens,
			stats.CompletionTokens,
		)
	}

	event := r.Events(1)[0]
	if event.PromptTokens != 0 || event.CompletionTokens != 0 {
		t.Errorf("absent usage = %d/%d, want 0/0", event.PromptTokens, event.CompletionTokens)
	}
}

func TestRecorder_StatsTruncation(t *testing.T) {
	t.Parallel()

	const window = time.Hour

	t.Run("wrapped inside window", func(t *testing.T) {
		t.Parallel()

		r := New()
		base := time.Now().Add(-10 * time.Minute)

		for i := range eventCapacity + 1 {
			r.Record(Event{
				Timestamp: base.Add(time.Duration(i) * time.Millisecond),
				Outcome:   OutcomeMatched,
			})
		}

		stats := r.Stats(window)
		wantOldest := base.Add(time.Millisecond)

		if !stats.OldestBuffered.Equal(wantOldest) {
			t.Errorf("oldest buffered = %v, want %v", stats.OldestBuffered, wantOldest)
		}

		if stats.WindowAttempted != eventCapacity {
			t.Errorf("window attempted = %d, want %d", stats.WindowAttempted, eventCapacity)
		}

		effectiveMinutes := stats.WindowStart.Add(window).Sub(wantOldest).Minutes()
		wantThroughput := float64(eventCapacity) / effectiveMinutes

		if math.Abs(stats.ThroughputPerMinute-wantThroughput) > 1e-9 {
			t.Errorf("throughput = %v, want %v", stats.ThroughputPerMinute, wantThroughput)
		}
	})

	t.Run("not full", func(t *testing.T) {
		t.Parallel()

		r := New()
		base := time.Now().Add(-10 * time.Minute)

		for i := range eventCapacity - 1 {
			r.Record(Event{Timestamp: base.Add(time.Duration(i) * time.Millisecond)})
		}

		if got := r.Stats(window).OldestBuffered; !got.IsZero() {
			t.Errorf("oldest buffered = %v, want zero", got)
		}
	})

	t.Run("wrapped before window", func(t *testing.T) {
		t.Parallel()

		r := New()
		base := time.Now().Add(-2 * time.Hour)

		for i := range eventCapacity + 1 {
			r.Record(Event{Timestamp: base.Add(time.Duration(i) * time.Second)})
		}

		if got := r.Stats(window).OldestBuffered; !got.IsZero() {
			t.Errorf("oldest buffered = %v, want zero", got)
		}
	})
}

func TestRecorder_NilSafe(t *testing.T) {
	t.Parallel()

	var r *Recorder

	r.Record(Event{})
	done := r.Begin()
	done()

	if got := r.Events(10); got != nil {
		t.Errorf("events = %v, want nil", got)
	}

	if got := r.Stats(time.Minute); !reflect.DeepEqual(got, Stats{}) {
		t.Errorf("stats = %#v, want zero", got)
	}

	if got := r.Collectors(); got != nil {
		t.Errorf("collectors = %v, want nil", got)
	}
}

func TestRecorder_Begin(t *testing.T) {
	t.Parallel()

	r := New()
	doneFirst := r.Begin()
	doneSecond := r.Begin()

	if got := r.Stats(time.Minute).InFlight; got != 2 {
		t.Errorf("in flight = %d, want 2", got)
	}

	doneSecond()

	if got := r.Stats(time.Minute).InFlight; got != 1 {
		t.Errorf("in flight = %d, want 1", got)
	}

	doneFirst()

	if got := r.Stats(time.Minute).InFlight; got != 0 {
		t.Errorf("in flight = %d, want 0", got)
	}
}

func TestRecorder_Prometheus(t *testing.T) {
	t.Parallel()

	r := New()
	r.Record(Event{Provider: "alpha", Outcome: OutcomeMatched, Duration: time.Second})
	r.Record(Event{Provider: "alpha", Outcome: OutcomeError, Duration: 2 * time.Second})
	r.Record(Event{Provider: "beta", Outcome: OutcomeUnmatched, Duration: 3 * time.Second})

	registry := prometheus.NewPedanticRegistry()

	for _, collector := range r.Collectors() {
		if err := registry.Register(collector); err != nil {
			t.Fatalf("register collector: %v", err)
		}
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	counters := make(map[string]float64)
	histogramCounts := make(map[string]uint64)
	histogramSums := make(map[string]float64)

	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))

			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			switch family.GetName() {
			case "bitmagnet_llm_classifications_total":
				counters[labels["provider"]+"/"+labels["outcome"]] = metric.GetCounter().GetValue()
			case "bitmagnet_llm_classification_duration_seconds":
				histogramCounts[labels["provider"]] = metric.GetHistogram().GetSampleCount()
				histogramSums[labels["provider"]] = metric.GetHistogram().GetSampleSum()
			}
		}
	}

	wantCounters := map[string]float64{
		"alpha/matched":  1,
		"alpha/error":    1,
		"beta/unmatched": 1,
	}

	if !reflect.DeepEqual(counters, wantCounters) {
		t.Errorf("counters = %v, want %v", counters, wantCounters)
	}

	wantHistogramCounts := map[string]uint64{"alpha": 2, "beta": 1}
	if !reflect.DeepEqual(histogramCounts, wantHistogramCounts) {
		t.Errorf("histogram counts = %v, want %v", histogramCounts, wantHistogramCounts)
	}

	wantHistogramSums := map[string]float64{"alpha": 3, "beta": 3}
	if !reflect.DeepEqual(histogramSums, wantHistogramSums) {
		t.Errorf("histogram sums = %v, want %v", histogramSums, wantHistogramSums)
	}

	if got := len(r.Collectors()); got != 2 {
		t.Errorf("collectors = %d, want 2", got)
	}
}
