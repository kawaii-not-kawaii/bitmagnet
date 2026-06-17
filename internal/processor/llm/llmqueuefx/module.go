package llmqueue

import (
	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"go.uber.org/fx"
)

type fxParams struct {
	fx.In
	Classifier lazy.Lazy[classifier.Runner]
	Dao        lazy.Lazy[*dao.Query]
}

type fxResult struct {
	fx.Out
	Handler lazy.Lazy[Handler] `group:"queue_handlers"`
}

type Handler interface {
	Name() string
}

func NewFx(p fxParams) fxResult {
	return fxResult{
		Handler: lazy.New(func() (Handler, error) {
			_, err := p.Classifier.Get()
			if err != nil {
				return nil, err
			}
			_, err = p.Dao.Get()
			if err != nil {
				return nil, err
			}
			return &llmHandler{name: "llm_classify"}, nil
		}),
	}
}

type llmHandler struct {
	name string
}

func (h *llmHandler) Name() string { return h.name }
