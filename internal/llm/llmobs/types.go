// Package llmobs records LLM classification attempts in-process — a bounded
// ring buffer of recent events plus lifetime counters — and serves them to the
// GraphQL observability queries and Prometheus collectors. Recording is
// race-safe, never fails, and never alters a classification outcome.
//
// See openspec/changes/llm-observability/ for the governing design and specs.
package llmobs

import "time"

// Outcome classifies a single LLM classification attempt.
type Outcome string

const (
	// OutcomeMatched: a usable result was parsed and applied.
	OutcomeMatched Outcome = "MATCHED"
	// OutcomeUnmatched: the provider answered but no usable classification
	// resulted.
	OutcomeUnmatched Outcome = "UNMATCHED"
	// OutcomeError: the call or parse failed.
	OutcomeError Outcome = "ERROR"
	// OutcomeSkipped: no providers were available — a distinct operator
	// signal (misconfiguration), not a model limitation.
	OutcomeSkipped Outcome = "SKIPPED"
)

// Event is one recorded classification attempt. Fields are plain scalars so
// this package depends on nothing above the standard library.
type Event struct {
	Timestamp   time.Time
	InfoHash    string
	TorrentName string
	// Provider is empty for OutcomeSkipped.
	Provider         string
	Duration         time.Duration
	PromptTokens     int
	CompletionTokens int
	Outcome          Outcome
	// Parsed fields, populated when Outcome is OutcomeMatched.
	ContentType string
	Title       string
	Year        int
	Season      int
	Episode     int
	Languages   []string
	// Error is the failure message when Outcome is OutcomeError.
	Error string
}

// ProviderStats is the lifetime per-provider breakdown.
type ProviderStats struct {
	Provider  string
	Attempted int64
	Matched   int64
	Unmatched int64
	Errored   int64
}

// Stats is the aggregate payload served to the stats query.
type Stats struct {
	// Lifetime counters since process start.
	Attempted        int64
	Matched          int64
	Unmatched        int64
	Errored          int64
	Skipped          int64
	PromptTokens     int64
	CompletionTokens int64
	// PerProvider is sorted by provider name for deterministic output.
	PerProvider []ProviderStats
	// InFlight is the number of classifications currently executing.
	InFlight int64
	// Concurrency is the configured classifier concurrency ceiling.
	Concurrency int
	// EffectiveConcurrency is the controller's current admission limit.
	EffectiveConcurrency int

	// Windowed figures, computed over buffered events at read time.
	WindowStart time.Time
	// OldestBuffered marks the effective window start when the ring buffer
	// no longer covers the requested window (honest truncation). Zero when
	// the buffer covers the full window.
	OldestBuffered      time.Time
	WindowAttempted     int
	LatencyP50          time.Duration
	LatencyP95          time.Duration
	ThroughputPerMinute float64
}
