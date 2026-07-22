package llmbench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

const (
	MaxSampleSize = 500
	maxFiles      = 20
)

type Loader func(context.Context, int, bool) ([]model.Torrent, error)

type Options struct {
	Concurrency int
	Random      bool
	Progress    io.Writer
}

type Result struct {
	Provider        string           `json:"provider"`
	TorrentCount    int              `json:"torrent_count"`
	TotalDuration   time.Duration    `json:"total_duration"`
	AvgPerTorrent   time.Duration    `json:"avg_per_torrent"`
	Throughput      float64          `json:"throughput_per_second"`
	Matched         int              `json:"matched"`
	Unmatched       int              `json:"unmatched"`
	Errored         int              `json:"errored"`
	Successes       int              `json:"successes"`
	Failures        int              `json:"failures"`
	Classifications []ClassifyRecord `json:"classifications"`
}

type ClassifyRecord struct {
	Name        string         `json:"name"`
	ContentType string         `json:"content_type"`
	Title       string         `json:"title"`
	Duration    time.Duration  `json:"duration"`
	Outcome     llmobs.Outcome `json:"outcome"`
	Error       string         `json:"error,omitempty"`
}

func LoadFromDAO(d *dao.Query) Loader {
	return func(ctx context.Context, count int, random bool) ([]model.Torrent, error) {
		query := d.Torrent.WithContext(ctx).Preload(d.Torrent.Files)

		if random {
			total, err := d.Torrent.WithContext(ctx).Count()
			if err != nil {
				return nil, fmt.Errorf("count torrents: %w", err)
			}

			if total > int64(count) {
				//nolint:gosec // benchmark sampling offset, not security-sensitive
				query = query.Offset(rand.IntN(int(total - int64(count))))
			}
		}

		rows, err := query.Limit(count).Find()
		if err != nil {
			return nil, fmt.Errorf("query torrents: %w", err)
		}

		torrents := make([]model.Torrent, 0, MaxSampleSize)

		for _, torrent := range rows {
			torrents = append(torrents, *torrent)
		}

		return torrents, nil
	}
}

func Run(
	ctx context.Context,
	providerName string,
	provider llm.Provider,
	requested int,
	load Loader,
	opts Options,
) (*Result, error) {
	if requested < 1 {
		return nil, fmt.Errorf("sample size must be positive")
	}

	count := requested
	if count > MaxSampleSize {
		count = MaxSampleSize
	}

	loaded, err := load(ctx, count, opts.Random)
	if err != nil {
		return nil, err
	}

	torrents := make([]model.Torrent, 0, MaxSampleSize)

	for i, torrent := range loaded {
		if i == count {
			break
		}

		torrents = append(torrents, torrent)
	}

	if len(torrents) == 0 {
		return nil, fmt.Errorf("no torrents found")
	}

	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	if opts.Progress != nil {
		fmt.Fprintf(
			opts.Progress,
			"Benchmarking %s on %d torrents (concurrency %d)...\n",
			providerName,
			len(torrents),
			concurrency,
		)
	}

	started := time.Now()
	records := make(chan ClassifyRecord, MaxSampleSize)
	semaphore := make(chan struct{}, concurrency)

	var workers sync.WaitGroup

	for _, torrent := range torrents {
		workers.Add(1)

		go func(torrent model.Torrent) {
			defer workers.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			records <- classifyOne(ctx, provider, torrent)
		}(torrent)
	}

	go func() {
		workers.Wait()
		close(records)
	}()

	result := &Result{
		Provider:        providerName,
		Classifications: make([]ClassifyRecord, 0, MaxSampleSize),
	}

	var latency time.Duration

	for record := range records {
		result.Classifications = append(result.Classifications, record)
		latency += record.Duration

		switch record.Outcome {
		case llmobs.OutcomeMatched:
			result.Matched++
		case llmobs.OutcomeUnmatched:
			result.Unmatched++
		case llmobs.OutcomeError:
			result.Errored++
		}
	}

	result.TorrentCount = len(result.Classifications)
	result.TotalDuration = time.Since(started)
	result.AvgPerTorrent = latency / time.Duration(result.TorrentCount)
	result.Throughput = float64(result.TorrentCount) / result.TotalDuration.Seconds()
	result.Successes = result.Matched
	result.Failures = result.Unmatched + result.Errored

	return result, nil
}

func classifyOne(ctx context.Context, provider llm.Provider, torrent model.Torrent) ClassifyRecord {
	input := llm.ClassifyInput{
		Name:         torrent.Name,
		ContentTypes: "movie, tv_show, music, ebook, comic, audiobook, game, software, xxx",
	}

	files := make([]string, 0, maxFiles)

	for i, file := range torrent.Files {
		if i == maxFiles {
			break
		}

		files = append(files, file.Path)
	}

	input.Files = files

	started := time.Now()
	classification, err := provider.Classify(ctx, input)
	record := ClassifyRecord{Name: torrent.Name, Duration: time.Since(started)}

	if errors.Is(err, llm.ErrNoResult) || err == nil && classification == nil {
		record.Outcome = llmobs.OutcomeUnmatched

		return record
	}

	if err != nil {
		record.Outcome = llmobs.OutcomeError
		record.Error = err.Error()

		return record
	}

	record.Outcome = llmobs.OutcomeMatched
	record.ContentType = classification.ContentType
	record.Title = classification.Title

	return record
}
