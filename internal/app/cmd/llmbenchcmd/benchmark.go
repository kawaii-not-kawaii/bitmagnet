package llmbenchcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"go.uber.org/zap"
)

// maxBenchFiles caps the number of file paths included per prompt, mirroring
// the production limit in action_llm_classify.go so benchmark prompt sizes
// (and therefore latency) match the real classification path.
const maxBenchFiles = 20

type BenchmarkParams struct {
	Classifier lazy.Lazy[classifier.Runner]
	Dao        lazy.Lazy[*dao.Query]
	Providers  map[string]llm.Provider
	Logger     *zap.SugaredLogger
	Config     classifier.Config
}

type BenchmarkResult struct {
	Provider        string           `json:"provider"`
	TorrentCount    int              `json:"torrent_count"`
	TotalDuration   time.Duration    `json:"total_duration"`
	AvgPerTorrent   time.Duration    `json:"avg_per_torrent"`
	Throughput      float64          `json:"throughput_per_second"`
	Successes       int              `json:"successes"`
	Failures        int              `json:"failures"`
	Classifications []ClassifyRecord `json:"classifications"`
}

type ClassifyRecord struct {
	Name        string        `json:"name"`
	ContentType string        `json:"content_type"`
	Title       string        `json:"title"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
}

// BenchmarkOptions controls sampling and execution shape, independent of
// which torrents get picked or how the classification results are reported.
type BenchmarkOptions struct {
	// Concurrency is the number of torrents classified in parallel. 1
	// (the default) preserves the old strictly-sequential behavior, which
	// only ever exercises a single LLM provider slot.
	Concurrency int
	// Random, when true, samples from a random offset into the unknown
	// torrent set instead of always taking the same first N rows. Uses a
	// random OFFSET rather than ORDER BY random() to avoid a full-table
	// sort on what can be a multi-million-row table.
	Random bool
}

func RunBenchmark(ctx context.Context, p BenchmarkParams, count int, opts BenchmarkOptions) (*BenchmarkResult, error) {
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

	query := d.Torrent.WithContext(ctx).Preload(d.Torrent.Files)

	if opts.Random {
		total, countErr := d.Torrent.WithContext(ctx).Count()
		if countErr != nil {
			return nil, fmt.Errorf("count torrents: %w", countErr)
		}

		if total > int64(count) {
			//nolint:gosec // benchmark sampling offset, not security-sensitive
			offset := rand.Intn(int(total - int64(count)))
			query = query.Offset(offset)
		}
	}

	torrentResults, err := query.Limit(count).Find()
	if err != nil {
		return nil, fmt.Errorf("query torrents: %w", err)
	}

	torrents := make([]model.Torrent, 0, len(torrentResults))
	for _, t := range torrentResults {
		torrents = append(torrents, *t)
	}

	if len(torrents) == 0 {
		return nil, fmt.Errorf("no unknown torrents found")
	}

	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	fmt.Fprintf(os.Stderr, "Benchmarking %s on %d torrents (concurrency %d)...\n",
		providerName, len(torrents), concurrency)

	start := time.Now()
	records := make([]ClassifyRecord, len(torrents))
	successes := 0
	failures := 0

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, concurrency)
	)

	for i, t := range torrents {
		wg.Add(1)

		sem <- struct{}{}

		go func(i int, t model.Torrent) {
			defer wg.Done()
			defer func() { <-sem }()

			record := classifyOne(ctx, provider, t)

			mu.Lock()
			defer mu.Unlock()

			records[i] = record
			if record.Error != "" {
				failures++
			} else {
				successes++
			}

			done := successes + failures
			if record.Error != "" {
				fmt.Fprintf(os.Stderr, "  ERR [%d/%d] %s -> %s (%.2fs)\n",
					done, len(torrents), truncate(t.Name, 40), record.Error, record.Duration.Seconds())
			} else {
				fmt.Fprintf(os.Stderr, "  OK  [%d/%d] %s -> %s (%.2fs)\n",
					done, len(torrents), truncate(t.Name, 40), record.ContentType, record.Duration.Seconds())
			}
		}(i, t)
	}

	wg.Wait()

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

// classifyOne runs a single classification, including the same file-path
// truncation the production llm_classify action applies
// (internal/classifier/action_llm_classify.go), so benchmark prompt sizes
// reflect real-world latency rather than understating it.
func classifyOne(ctx context.Context, provider llm.Provider, t model.Torrent) ClassifyRecord {
	input := llm.ClassifyInput{
		Name:         t.Name,
		ContentTypes: "movie, tv_show, music, ebook, comic, audiobook, game, software, xxx",
	}

	if len(t.Files) > 0 {
		files := make([]string, 0, min(len(t.Files), maxBenchFiles))
		for i, f := range t.Files {
			if i >= maxBenchFiles {
				break
			}

			files = append(files, f.Path)
		}

		input.Files = files
	}

	recordStart := time.Now()
	result, err := provider.Classify(ctx, input)
	duration := time.Since(recordStart)

	record := ClassifyRecord{
		Name:     t.Name,
		Duration: duration,
	}

	if err != nil {
		record.Error = err.Error()
	} else {
		record.ContentType = result.ContentType
		record.Title = result.Title
	}

	return record
}

func PrintJSON(r *BenchmarkResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(r)
}

func PrintSummary(r *BenchmarkResult) {
	fmt.Fprintf(os.Stdout, "\n=== Benchmark Results ===\n")
	fmt.Fprintf(os.Stdout, "Provider:        %s\n", r.Provider)
	fmt.Fprintf(os.Stdout, "Torrents:        %d\n", r.TorrentCount)
	fmt.Fprintf(os.Stdout, "Successes:       %d\n", r.Successes)
	fmt.Fprintf(os.Stdout, "Failures:        %d\n", r.Failures)
	fmt.Fprintf(os.Stdout, "Total Duration:  %s\n", r.TotalDuration.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "Avg/Torrent:     %s\n", r.AvgPerTorrent.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "Throughput:      %.2f torrents/sec\n", r.Throughput)
	fmt.Fprintf(os.Stdout, "\nClassification Distribution:\n")

	distribution := make(map[string]int)

	for _, c := range r.Classifications {
		if c.ContentType != "" {
			distribution[c.ContentType]++
		}
	}

	for ct, count := range distribution {
		fmt.Fprintf(os.Stdout, "  %s: %d\n", ct, count)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen-3] + "..."
}
