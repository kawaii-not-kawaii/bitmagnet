package classifier

import "time"

// Config holds the runtime configuration for the classifier subsystem.
type Config struct {
	Workflow    string
	Keywords    map[string][]string
	Extensions  map[string][]string
	Flags       map[string]any
	DeleteXxx   bool
	Concurrency int `validate:"gt=0"`
	// AutoScale maps to `auto_scale` via the resolver's strcase.ToSnake on the
	// field name (like every other field here) — no mapstructure/yaml tag needed.
	AutoScale bool
	Verbose   bool
	Llm       LlmConfig
}

// LlmConfig holds the configuration for LLM-based classification.
// Flattened to single-provider to avoid config resolver issues with map[string]struct.
type LlmConfig struct {
	Enabled         bool
	ProviderName    string
	ProviderBaseURL string
	ProviderModel   string
	ProviderAPIKey  string
	BatchSize       int
	MaxContext      int
	MaxTokens       int
	Interval        time.Duration
	Timeout         time.Duration
}

func NewDefaultConfig() Config {
	return Config{
		Workflow:    "default",
		Concurrency: 10,
		Llm: LlmConfig{
			Enabled:    true,
			BatchSize:  1,
			MaxContext: 16000,
			MaxTokens:  256,
			Interval:   5 * time.Second,
			Timeout:    30 * time.Second,
		},
	}
}

// ConcurrencyLimits returns the admission ceiling and autoscaling switch the
// ConcurrencyController should run with.
//
// LLM batching only groups classifier runs that are waiting on the LLM at the
// same time, so a batch can never exceed the number of runs admitted at once.
// When batching is on, the ceiling is raised to at least one full batch and
// the latency-driven autoscaler is paused: it starts at 1, and a batch's
// fill wait plus its long request would read as a latency spike and keep
// halving admission, so batches would never fill.
func (c Config) ConcurrencyLimits() (ceiling int, autoScale bool) {
	if c.Llm.Enabled && c.Llm.BatchSize > 1 {
		return max(c.Concurrency, c.Llm.BatchSize), false
	}

	return c.Concurrency, c.AutoScale
}
