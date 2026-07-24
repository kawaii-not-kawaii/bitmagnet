package classifier

import (
	"testing"

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
