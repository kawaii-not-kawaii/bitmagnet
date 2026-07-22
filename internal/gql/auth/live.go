package auth

import (
	"fmt"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
)

func NewLiveApplier(authenticator *Authenticator) configapply.LiveApplier {
	return configapply.LiveApplier{
		Key: "auth",
		Apply: func(value any) (func(), error) {
			cfg, ok := value.(Config)
			if !ok {
				return nil, fmt.Errorf(
					"configapply: section %q: expected %T, got %T",
					"auth",
					cfg,
					value,
				)
			}
			return nil, authenticator.applyConfig(cfg)
		},
	}
}
