package logging

import (
	"fmt"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"go.uber.org/zap"
)

func NewLiveApplier(level zap.AtomicLevel) configapply.LiveApplier {
	return configapply.LiveApplier{
		Key: "log",
		Apply: func(value any) (func(), error) {
			cfg, ok := value.(Config)
			if !ok {
				return nil, fmt.Errorf(
					"configapply: section %q: expected %T, got %T", "log", cfg, value)
			}

			level.SetLevel(levelToZapLevel(cfg.Level))

			return nil, nil
		},
	}
}
