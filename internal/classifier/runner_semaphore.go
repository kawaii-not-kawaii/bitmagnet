package classifier

import (
	"context"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

type runnerSemaphore struct {
	runner     Runner
	controller *ConcurrencyController
}

func (r runnerSemaphore) Run(
	ctx context.Context,
	workflow string,
	flags Flags,
	t model.Torrent,
) (classification.Result, error) {
	if err := r.controller.Acquire(ctx); err != nil {
		return classification.Result{}, err
	}

	defer r.controller.Release()

	return r.runner.Run(ctx, workflow, flags, t)
}
