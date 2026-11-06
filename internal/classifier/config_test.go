package classifier

import (
	"testing"
	"time"

	configpkg "github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configresolver"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigConcurrencyDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	config := NewDefaultConfig()
	assert.False(t, config.AutoScale)
	require.NoError(t, validator.New().Struct(config))

	config.Concurrency = 0
	require.Error(t, validator.New().Struct(config))
}

func TestConfigLlmBatchSizeDefaultAndExplicitOverride(t *testing.T) {
	t.Parallel()

	validate := validator.New()
	defaultConfig := NewDefaultConfig()

	assert.Equal(t, 1, defaultConfig.Llm.BatchSize)

	result, err := configpkg.New(configpkg.Params{
		Specs: []configpkg.Spec{{
			Key:          "classifier",
			DefaultValue: defaultConfig,
		}},
		Resolvers: []configresolver.Resolver{
			configresolver.NewMap(map[string]interface{}{
				"classifier": map[string]interface{}{
					"llm": map[string]interface{}{
						"batch_size": 10,
					},
				},
			}, validate),
		},
		Validate: validate,
	})
	require.NoError(t, err)

	resolved, ok := result.Resolved.NodeMap["classifier"].Value.(Config)
	require.True(t, ok)
	assert.Equal(t, 10, resolved.Llm.BatchSize)
}

// Batching only groups classifier runs that wait on the LLM at the same time,
// so a batch can never be larger than the number of runs admitted at once.
// With batching on, admission must allow at least one full batch, and the
// latency-driven autoscaler (which starts at 1) is paused.
func TestConfigConcurrencyLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		concurrency   int
		autoScale     bool
		llmEnabled    bool
		batchSize     int
		wantCeiling   int
		wantAutoScale bool
	}{
		{"no batching", 10, true, true, 1, 10, true},
		{"batching raises ceiling and pauses autoscale", 10, true, true, 100, 100, false},
		{"concurrency already above batch", 200, false, true, 100, 200, false},
		{"llm disabled ignores batch size", 10, true, false, 100, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := NewDefaultConfig()
			cfg.Concurrency, cfg.AutoScale = tt.concurrency, tt.autoScale
			cfg.Llm.Enabled, cfg.Llm.BatchSize = tt.llmEnabled, tt.batchSize

			ceiling, autoScale := cfg.ConcurrencyLimits()
			assert.Equal(t, tt.wantCeiling, ceiling)
			assert.Equal(t, tt.wantAutoScale, autoScale)
		})
	}
}

func TestConfigArmigerDefaultsAndSnakeCaseOverride(t *testing.T) {
	t.Parallel()

	validate := validator.New()
	defaultConfig := NewDefaultConfig()

	assert.False(t, defaultConfig.Armiger.Enabled)
	assert.Empty(t, defaultConfig.Armiger.URL)
	assert.Equal(t, 2*time.Second, defaultConfig.Armiger.Timeout)

	result, err := configpkg.New(configpkg.Params{
		Specs: []configpkg.Spec{{
			Key:          "classifier",
			DefaultValue: defaultConfig,
		}},
		Resolvers: []configresolver.Resolver{
			configresolver.NewMap(map[string]interface{}{
				"classifier": map[string]interface{}{
					"armiger": map[string]interface{}{
						"enabled": true,
						"url":     "http://armiger:8080",
						"timeout": "500ms",
					},
				},
			}, validate),
		},
		Validate: validate,
	})
	require.NoError(t, err)

	resolved, ok := result.Resolved.NodeMap["classifier"].Value.(Config)
	require.True(t, ok)
	assert.Equal(t, ArmigerConfig{
		Enabled: true,
		URL:     "http://armiger:8080",
		Timeout: 500 * time.Millisecond,
	}, resolved.Armiger)
}
