package classifier

import (
	"testing"

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
