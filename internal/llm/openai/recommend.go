package openai

import "time"

// Repository defaults used when the live configuration carries no positive
// value for a field. They mirror the classifier LLM config defaults.
const (
	defaultRecommendMaxTokens   = 256
	defaultRecommendMaxContext  = 16000
	defaultRecommendTimeout     = 30 * time.Second
	defaultRecommendConcurrency = 10
)

// CurrentConfig is the live classifier LLM configuration a recommendation
// starts from. Non-positive fields fall back to the repository defaults.
type CurrentConfig struct {
	MaxTokens   int
	MaxContext  int
	Timeout     time.Duration
	Concurrency int
}

// RecommendedConfig is a complete, operator-acceptable classifier LLM
// configuration derived from a successful capacity probe. Every field is
// populated; the UI applies all five or none.
type RecommendedConfig struct {
	BatchSize      int
	MaxTokens      int
	MaxContext     int
	TimeoutSeconds int
	Concurrency    int
}

// resolve returns the current configuration with every non-positive field
// replaced by its repository default (rule 1 of the recommendation design).
func (c CurrentConfig) resolve() CurrentConfig {
	if c.MaxTokens <= 0 {
		c.MaxTokens = defaultRecommendMaxTokens
	}

	if c.MaxContext <= 0 {
		c.MaxContext = defaultRecommendMaxContext
	}

	if c.Timeout <= 0 {
		c.Timeout = defaultRecommendTimeout
	}

	if c.Concurrency <= 0 {
		c.Concurrency = defaultRecommendConcurrency
	}

	return c
}

// Recommend derives a complete configuration recommendation from a capacity
// probe, the live configuration, and the measured connection latency.
//
// It is pure: no I/O, no mutation of capacity or the provider registry. Callers
// must only invoke it for a probe taken during a SUCCESSFUL connection test —
// a failed test must present no recommendation at all.
//
// TODO(pr-2): apply the detected-capacity caps — token/context clamping against
// the probed window, source-based concurrency, and latency-derived timeout.
func Recommend(_ Capacity, current CurrentConfig, _ time.Duration) RecommendedConfig {
	resolved := current.resolve()

	return RecommendedConfig{
		BatchSize:      1,
		MaxTokens:      resolved.MaxTokens,
		MaxContext:     resolved.MaxContext,
		TimeoutSeconds: int(resolved.Timeout.Seconds()),
		Concurrency:    resolved.Concurrency,
	}
}
