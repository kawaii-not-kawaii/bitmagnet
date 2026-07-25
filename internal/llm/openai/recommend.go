package openai

import (
	"math"
	"time"
)

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
func Recommend(capacity Capacity, current CurrentConfig, latency time.Duration) RecommendedConfig {
	resolved := current.resolve()
	maxTokens := resolved.MaxTokens

	if capacity.MaxCompletionTokens != nil && *capacity.MaxCompletionTokens > 0 {
		maxTokens = min(maxTokens, *capacity.MaxCompletionTokens)
	}

	maxContext := resolved.MaxContext

	if capacity.ContextPerRequest != nil && *capacity.ContextPerRequest >= 2 {
		window := *capacity.ContextPerRequest
		maxTokens = min(maxTokens, window-1)
		maxContext = min(maxContext, window-maxTokens)
	}

	concurrency := resolved.Concurrency

	switch {
	case capacity.Slots != nil && *capacity.Slots > 0:
		concurrency = *capacity.Slots
	case capacity.Source == "models" || capacity.Source == "models.dev":
		concurrency = 16
	}

	timeoutSeconds := int(math.Ceil(4 * latency.Seconds()))
	timeoutSeconds = max(int(defaultRecommendTimeout.Seconds()), min(timeoutSeconds, 300))

	return RecommendedConfig{
		BatchSize:      1,
		MaxTokens:      maxTokens,
		MaxContext:     maxContext,
		TimeoutSeconds: timeoutSeconds,
		Concurrency:    concurrency,
	}
}
