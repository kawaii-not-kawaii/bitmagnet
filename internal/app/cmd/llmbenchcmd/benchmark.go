package llmbenchcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"go.uber.org/zap"
)

type BenchmarkParams struct {
	Classifier lazy.Lazy[classifier.Runner]
	Dao        lazy.Lazy[*dao.Query]
	Providers  map[string]llm.Provider
	Logger     *zap.SugaredLogger
	Config     classifier.Config
}

type BenchmarkResult struct {
	Provider      string        `json:"provider"`
	TorrentCount  int           `json:"torrent_count"`
	TotalDuration time.Duration `json:"total_duration"`
	AvgPerTorrent time.Duration `json:"avg_per_torrent"`
	Throughput    float64       `json:"throughput_per_second"`
	Successes     int           `json:"successes"`
	Failures      int           `json:"failures"`
	Classifications []ClassifyRecord `json:"classifications"`
}

type ClassifyRecord struct {
	Name        string        `json:"name"`
	ContentType string        `json:"content_type"`
	Title       string        `json:"title"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
}

func RunBenchmark(ctx context.Context, p BenchmarkParams, count int) (*BenchmarkResult, error) {
	if len(p.Providers) == 0 {
		return nil, fmt.Errorf("no LLM providers configured")
	}

	// Pick first provider
	var provider llm.Provider
	var providerName string
	for name, prov := range p.Providers {
		provider = prov
		providerName = name
		break
	}

	d, err := p.Dao.Get()
	if err != nil {
		return nil, fmt.Errorf("dao: %w", err)
	}

	// Fetch unknown torrents
	var torrents []model.Torrent
	err = d.Torrent.WithContext(ctx).
		Where("info_hash IN (SELECT info_hash FROM torrent_contents WHERE content_type IS NULL LIMIT ?)", count).
		Limit(count).
		Find(&torrents).Error
	if err != nil {
		return nil, fmt.Errorf("query torrents: %w", err)
	}

	if len(torrents) == 0 {
		return nil, fmt.Errorf("no unknown torrents found")
	}

	fmt.Fprintf(os.Stderr, "Benchmarking %s on %d torrents...\n", providerName, len(torrents))

	start := time.Now()
	records := make([]ClassifyRecord, 0, len(torrents))
	successes := 0
	failures := 0

	for _, t := range torrents {
		input := llm.ClassifyInput{
			Name:         t.Name,
			ContentTypes: "movie, tv_show, music, ebook, comic, audiobook, game, software, xxx",
		}

		recordStart := time.Now()
		result, err := provider.Classify(ctx, input)
		duration := time.Since(recordStart)

		record := ClassifyRecord{
			Name:     t.Name,
			Duration: duration,
		}

		if err != nil {
			failures++
			record.Error = err.Error()
		} else {
			successes++
			record.ContentType = result.ContentType
			record.Title = result.Title
		}

		records = append(records, record)

		fmt.Fprintf(os.Stderr, "  [%d/%d] %s -> %s (%.2fs)\n",
			len(records), len(torrents),
			truncate(t.Name, 50),
			record.ContentType,
			duration.Seconds())
	}

	totalDuration := time.Since(start)

	benchResult := &BenchmarkResult{
		Provider:        providerName,
		TorrentCount:    len(torrents),
		TotalDuration:   totalDuration,
		AvgPerTorrent:   totalDuration / time.Duration(len(torrents)),
		Throughput:      float64(len(torrents)) / totalDuration.Seconds(),
		Successes:       successes,
		Failures:        failures,
		Classifications: records,
	}

	return benchResult, nil
}

func PrintJSON(r *BenchmarkResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func PrintSummary(r *BenchmarkResult) {
	fmt.Printf("\n=== Benchmark Results ===\n")
	fmt.Printf("Provider:        %s\n", r.Provider)
	fmt.Printf("Torrents:        %d\n", r.TorrentCount)
	fmt.Printf("Successes:       %d\n", r.Successes)
	fmt.Printf("Failures:        %d\n", r.Failures)
	fmt.Printf("Total Duration:  %s\n", r.TotalDuration.Round(time.Millisecond))
	fmt.Printf("Avg/Torrent:     %s\n", r.AvgPerTorrent.Round(time.Millisecond))
	fmt.Printf("Throughput:      %.2f torrents/sec\n", r.Throughput)
	fmt.Printf("\nClassification Distribution:\n")

	distribution := make(map[string]int)
	for _, c := range r.Classifications {
		if c.ContentType != "" {
			distribution[c.ContentType]++
		}
	}
	for ct, count := range distribution {
		fmt.Printf("  %s: %d\n", ct, count)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
