package torznab

import (
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/slice"
)

type Config struct {
	BaseURL  string
	Profiles []Profile
}

const configBaseURLUndefined = "__undefined__"

func (c Config) MergeDefaults() Config {
	c.Profiles = slice.Map(c.Profiles, func(profile Profile) Profile {
		return profile.MergeDefaults()
	})

	return c
}

func NewDefaultConfig() Config {
	return Config{
		BaseURL: configBaseURLUndefined,
	}
}

func (c Config) GetProfile(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if strings.EqualFold(p.ID, name) {
			// An empty BaseURL counts as unset, not as "set to empty". The
			// sentinel only distinguishes unset from set when Config was built
			// via NewDefaultConfig; a zero-valued Config (as in tests, or any
			// caller constructing the struct directly) has BaseURL == "", which
			// would otherwise pass this check and yield a Valid-but-empty
			// NullString. Downstream, PermaLink gates on .Valid, so that would
			// produce permalinks with no host.
			if c.BaseURL != configBaseURLUndefined && c.BaseURL != "" && !p.BaseURL.Valid {
				p.BaseURL = model.NewNullString(c.BaseURL)
			}

			return p, true
		}
	}

	return Profile{}, false
}
