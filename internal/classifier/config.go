package classifier

import "time"

// Config holds the runtime configuration for the classifier subsystem.
type Config struct {
	Workflow    string
	Keywords    map[string][]string
	Extensions  map[string][]string
	Flags       map[string]any
	DeleteXxx   bool
	Concurrency int
	Verbose     bool
	Llm         LlmConfig
}

// LlmConfig holds the configuration for LLM-based classification.
type LlmConfig struct {
	Providers  map[string]LlmProviderConfig
	BatchSize  int
	MaxContext int
	MaxTokens  int
	Interval   time.Duration
	Timeout    time.Duration
}

// LlmProviderConfig holds a single LLM provider's configuration.
// Providers are referenced by name in workflow actions via llm_classify.
type LlmProviderConfig struct {
	BaseURL      string        `yaml:"base_url" json:"base_url"`
	Model        string        `yaml:"model"    json:"model"`
	APIKey       string        `yaml:"api_key"  json:"api_key,omitempty"`
	Timeout      time.Duration `yaml:"timeout"  json:"timeout,omitempty"`
	SystemPrompt string        `yaml:"system_prompt" json:"system_prompt,omitempty"`
}

func NewDefaultConfig() Config {
	return Config{
		Workflow:    "default",
		Concurrency: 10,
		Llm: LlmConfig{
			BatchSize:  5,
			MaxContext: 16000,
			MaxTokens:  256,
			Interval:   5 * time.Second,
			Timeout:    30 * time.Second,
		},
	}
}
