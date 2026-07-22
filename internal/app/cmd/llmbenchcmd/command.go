package llmbenchcmd

import (
	"os"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
)

type Params struct {
	fx.In
	Dao       lazy.Lazy[*dao.Query]
	Providers map[string]llm.Provider
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
				&cli.IntFlag{
					Name:  "concurrency",
					Value: 1,
					Usage: "Number of torrents to classify in parallel",
				},
				&cli.BoolFlag{
					Name:  "random",
					Usage: "Sample from a random offset instead of always the same first N torrents",
				},
				&cli.BoolFlag{
					Name:  "json",
					Usage: "Output results as JSON",
				},
			},
			Action: func(ctx *cli.Context) error {
				count := ctx.Int("count")
				result, err := RunBenchmark(ctx.Context, BenchmarkParams{
					Dao:       p.Dao,
					Providers: p.Providers,
				}, count, BenchmarkOptions{
					Concurrency: ctx.Int("concurrency"),
					Random:      ctx.Bool("random"),
					Progress:    os.Stderr,
				})
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
