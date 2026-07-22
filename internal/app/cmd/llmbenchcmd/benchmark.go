package llmbenchcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmbench"
)

type BenchmarkParams struct {
	Dao       lazy.Lazy[*dao.Query]
	Providers map[string]llm.Provider
}

type (
	BenchmarkResult  = llmbench.Result
	BenchmarkOptions = llmbench.Options
	ClassifyRecord   = llmbench.ClassifyRecord
)

func RunBenchmark(
	ctx context.Context,
	params BenchmarkParams,
	count int,
	opts BenchmarkOptions,
) (*BenchmarkResult, error) {
	if len(params.Providers) == 0 {
		return nil, fmt.Errorf("no LLM providers configured")
	}

	names := make([]string, 0, len(params.Providers))
	for name := range params.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	d, err := params.Dao.Get()
	if err != nil {
		return nil, fmt.Errorf("dao: %w", err)
	}

	providerName := names[0]

	return llmbench.Run(
		ctx,
		providerName,
		params.Providers[providerName],
		count,
		llmbench.LoadFromDAO(d),
		opts,
	)
}

func PrintJSON(result *BenchmarkResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	return encoder.Encode(result)
}

func PrintSummary(result *BenchmarkResult) {
	fmt.Fprintf(os.Stdout, "\n=== Benchmark Results ===\n")
	fmt.Fprintf(os.Stdout, "Provider:        %s\n", result.Provider)
	fmt.Fprintf(os.Stdout, "Torrents:        %d\n", result.TorrentCount)
	fmt.Fprintf(os.Stdout, "Matched:         %d\n", result.Matched)
	fmt.Fprintf(os.Stdout, "Unmatched:       %d\n", result.Unmatched)
	fmt.Fprintf(os.Stdout, "Errored:         %d\n", result.Errored)
	fmt.Fprintf(os.Stdout, "Total Duration:  %s\n", result.TotalDuration.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "Avg/Torrent:     %s\n", result.AvgPerTorrent.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "Throughput:      %.2f torrents/sec\n", result.Throughput)
	fmt.Fprintf(os.Stdout, "\nClassification Distribution:\n")

	distribution := make(map[string]int)
	for _, classification := range result.Classifications {
		if classification.ContentType != "" {
			distribution[classification.ContentType]++
		}
	}

	for contentType, count := range distribution {
		fmt.Fprintf(os.Stdout, "  %s: %d\n", contentType, count)
	}
}
