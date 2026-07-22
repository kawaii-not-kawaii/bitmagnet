package resolvers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmbench"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

func (r *Resolver) dashboardQuery(ctx context.Context) (gen.DashboardQuery, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	total, err := r.Dao.Torrent.WithContext(ctx).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count torrents: %w", err)
	}

	today, err := r.Dao.Torrent.WithContext(ctx).Where(r.Dao.Torrent.CreatedAt.Gte(startOfDay)).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count today's torrents: %w", err)
	}

	lastHour, err := r.Dao.Torrent.WithContext(ctx).Where(r.Dao.Torrent.CreatedAt.Gte(now.Add(-time.Hour))).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count last hour: %w", err)
	}

	previousHour, err := r.Dao.Torrent.WithContext(ctx).Where(
		r.Dao.Torrent.CreatedAt.Gte(now.Add(-2*time.Hour)),
		r.Dao.Torrent.CreatedAt.Lt(now.Add(-time.Hour)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count previous hour: %w", err)
	}

	classified, err := r.Dao.TorrentContent.WithContext(ctx).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count classified torrents: %w", err)
	}

	processed, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.Eq(string(model.QueueJobStatusProcessed)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count processed jobs: %w", err)
	}

	pending, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.In(string(model.QueueJobStatusPending), string(model.QueueJobStatusRetry)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count pending jobs: %w", err)
	}

	failed, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.Eq(string(model.QueueJobStatusFailed)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count failed jobs: %w", err)
	}

	classifiedPercent := 0.0
	if total > 0 {
		classifiedPercent = min(100, 100*float64(classified)/float64(total))
	}

	return gen.DashboardQuery{
		Summary: gen.DashboardSummary{
			TotalTorrents:       int(total),
			TorrentsToday:       int(today),
			IndexedLastHour:     int(lastHour),
			IndexedPreviousHour: int(previousHour),
			ClassifiedPercent:   classifiedPercent,
			QueueProcessed:      int(processed),
			QueuePending:        int(pending),
			QueueFailed:         int(failed),
		},
	}, nil
}

func (r *Resolver) testDashboardLlmConnection(ctx context.Context) (gen.DashboardLlmConnectionResult, error) {
	_, provider, err := r.dashboardProvider()
	if err != nil {
		message := err.Error()

		return gen.DashboardLlmConnectionResult{ //nolint:nilerr // connection failure is the payload, not a resolver error
			Error: &message,
		}, nil
	}

	startedAt := time.Now()

	_, err = provider.Classify(ctx, llm.ClassifyInput{
		Name:         "The Matrix 1999 1080p BluRay",
		ContentTypes: strings.Join(model.ContentTypeNames(), ", "),
	})
	if err != nil {
		message := fmt.Errorf("dashboard: test LLM connection: %w", err).Error()

		return gen.DashboardLlmConnectionResult{ //nolint:nilerr // connection failure is the payload, not a resolver error
			Error: &message,
		}, nil
	}

	return gen.DashboardLlmConnectionResult{
		Ok:             true,
		Connected:      true,
		LatencySeconds: time.Since(startedAt).Seconds(),
	}, nil
}

func (r *Resolver) runDashboardLlmBenchmark(ctx context.Context, sampleSize int) (gen.DashboardLlmBenchmark, error) {
	providerName, provider, err := r.dashboardProvider()
	if err != nil {
		return gen.DashboardLlmBenchmark{}, err
	}

	result, err := llmbench.Run(
		ctx,
		providerName,
		provider,
		sampleSize,
		llmbench.LoadFromDAO(r.Dao),
		llmbench.Options{Concurrency: r.LlmRegistry.Config().BatchSize},
	)
	if err != nil {
		return gen.DashboardLlmBenchmark{}, fmt.Errorf("dashboard: run LLM benchmark: %w", err)
	}

	distributionCounts := make(map[string]int)

	for _, classification := range result.Classifications {
		if classification.ContentType != "" {
			distributionCounts[classification.ContentType]++
		}
	}

	distribution := make([]gen.DashboardLlmBenchmarkDistribution, 0, 16)
	for contentType, count := range distributionCounts {
		distribution = append(
			distribution,
			gen.DashboardLlmBenchmarkDistribution{ContentType: contentType, Count: count},
		)
	}

	sort.Slice(distribution, func(i, j int) bool {
		return distribution[i].Count > distribution[j].Count
	})

	return gen.DashboardLlmBenchmark{
		SampleSize:            result.TorrentCount,
		Successes:             result.Successes,
		Failures:              result.Failures,
		Matched:               result.Matched,
		Unmatched:             result.Unmatched,
		Errored:               result.Errored,
		AverageLatencySeconds: result.AvgPerTorrent.Seconds(),
		ThroughputPerSecond:   result.Throughput,
		Distribution:          distribution,
	}, nil
}

func (r *Resolver) dashboardProvider() (string, llm.Provider, error) {
	providers := r.LlmRegistry.All()
	names := make([]string, 0, len(providers))

	for name := range providers {
		names = append(names, name)
	}

	if len(names) == 0 {
		return "", nil, fmt.Errorf("dashboard: LLM engine is disabled or not configured")
	}

	sort.Strings(names)

	return names[0], providers[names[0]], nil
}
