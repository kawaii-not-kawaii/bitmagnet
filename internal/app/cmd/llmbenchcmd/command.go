package llmbenchcmd

import (
	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Classifier lazy.Lazy[classifier.Runner]
	Dao        lazy.Lazy[*dao.Query]
	Providers  map[string]llm.Provider
	Logger     *zap.SugaredLogger
	Config     classifier.Config
}

type Result struct {
	fx.Out
	Command *cli.Command `group:"commands"`
}

func New(p Params) Result {
	return Result{
		Command: &cli.Command{
			Name:  "llm-bench",
			Usage: "Benchmark LLM classification on unknown torrents",
			Flags: []cli.Flag{
				&cli.IntFlag{
					Name:  "count",
					Value: 20,
					Usage: "Number of unknown torrents to classify",
				},
				&cli.BoolFlag{
					Name:  "json",
					Usage: "Output results as JSON",
				},
			},
			Action: func(ctx *cli.Context) error {
				count := ctx.Int("count")
				result, err := RunBenchmark(ctx.Context, BenchmarkParams{
					Classifier: p.Classifier,
					Dao:        p.Dao,
					Providers:  p.Providers,
					Logger:     p.Logger,
					Config:     p.Config,
				}, count)
				if err != nil {
					return err
				}
				if ctx.Bool("json") {
					return PrintJSON(result)
				}
				PrintSummary(result)
				return nil
			},
		},
	}
}
