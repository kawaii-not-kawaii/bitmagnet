package llmqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/queue/handler"
	"go.uber.org/fx"
)

const MessageName = "llm_classify"

type MessageParams struct {
	InfoHashes []string `json:"info_hashes,omitempty"`
}

type Params struct {
	fx.In
	Classifier  lazy.Lazy[classifier.Runner]
	Dao         lazy.Lazy[*dao.Query]
	Logger      interface{ Infof(string, ...interface{}) }
}

type Result struct {
	fx.Out
	Handler lazy.Lazy[handler.Handler] `group:"queue_handlers"`
}

func New(p Params) Result {
	return Result{
		Handler: lazy.New(func() (handler.Handler, error) {
			cr, err := p.Classifier.Get()
			if err != nil {
				return handler.Handler{}, err
			}
			d, err := p.Dao.Get()
			if err != nil {
				return handler.Handler{}, err
			}

			return handler.New(
				MessageName,
				func(ctx context.Context, job model.QueueJob) error {
					msg := &MessageParams{}
					if err := json.Unmarshal([]byte(job.Payload), msg); err != nil {
						return err
					}

					return processBatch(ctx, cr, d, msg)
				},
				handler.JobTimeout(60*time.Second),
				handler.Concurrency(1),
			), nil
		}),
	}
}

func processBatch(ctx context.Context, runner classifier.Runner, d *dao.Query, msg *MessageParams) error {
	for _, hash := range msg.InfoHashes {
		infoHash, err := model.NewHash20FromHex(hash)
		if err != nil {
			continue
		}

		torrent, err := d.Torrent.GetByInfoHash(ctx, infoHash)
		if err != nil {
			continue
		}

		result, err := runner.Run(ctx, "default", nil, *torrent)
		if err != nil {
			if classification.IsRuntimeError(err) {
				continue
			}
			continue
		}

		// Persist the classification result
		content := model.TorrentContent{
			InfoHash: infoHash,
		}
		if result.ContentType.Valid {
			content.ContentType = model.NewNullContentType(result.ContentType.ContentType)
		}
		if result.BaseTitle.Valid {
			// Map to torrent content title
		}

		// TODO: implement proper persistence matching the existing processor pattern
		_ = content
	}

	return nil
}
