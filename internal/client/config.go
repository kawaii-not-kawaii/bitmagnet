package client

import (
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

type SendTo struct {
	ID       string
	Host     string
	Port     string
	Username string
	Password string
}

type Config struct {
	Enabled         bool
	SendTo          []SendTo
	DefaultCategory string
	Categories      map[model.ContentType]string
}

func NewDefaultConfig() Config {
	cfg := Config{
		Enabled:         false,
		DefaultCategory: "prowlarr",
	}
	cat := make(map[model.ContentType]string)
	cat[model.ContentTypeTvShow] = "sonarr"
	cat[model.ContentTypeMovie] = "radarr"
	cfg.Categories = cat

	return cfg
}

func (c Config) GetSendTo(id gen.ClientID) (SendTo, bool) {
	for _, c := range c.SendTo {
		if c.ID == string(id) {
			return c, true
		}
	}

	return SendTo{}, false
}

func (c Config) All() []gen.ClientID {
	all := make([]gen.ClientID, 0)

	for _, s := range c.SendTo {
		for _, valid := range gen.AllClientID {
			if s.ID == valid.String() {
				all = append(all, valid)
			}
		}
	}

	return all
}
