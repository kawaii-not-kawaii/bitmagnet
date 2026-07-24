package classifier

import "time"

// Config holds the runtime configuration for the classifier subsystem.
type Config struct {
	Workflow    string
	Keywords    map[string][]string
	Extensions  map[string][]string
	Flags       map[string]any
	DeleteXxx   bool
	Concurrency int  `validate:"gt=0"`
	AutoScale   bool `                yaml:"auto_scale" mapstructure:"auto_scale"`
	Verbose     bool
	Llm         LlmConfig
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
			BatchSize:  5,
			MaxContext: 16000,
			MaxTokens:  256,
			Interval:   5 * time.Second,
			Timeout:    30 * time.Second,
		},
	}
}
