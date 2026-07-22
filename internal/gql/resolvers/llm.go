package resolvers

import (
	"context"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/metrics/queuemetrics"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/processor"
)

func llmEvents(recorder *llmobs.Recorder, limit *int) []gen.LlmClassificationEvent {
	requested := 0
	if limit != nil {
		requested = *limit
	}

	events := recorder.Events(requested)
	result := make([]gen.LlmClassificationEvent, len(events))

	for i, event := range events {
		result[i] = gen.LlmClassificationEvent{
			Timestamp:   event.Timestamp,
			InfoHash:    event.InfoHash,
			TorrentName: event.TorrentName,
			Provider:    event.Provider,
			DurationMs:  int(event.Duration.Milliseconds()),
			Outcome:     gen.LlmClassificationOutcome(event.Outcome),
			ContentType: event.ContentType,
			Title:       event.Title,
			Year:        event.Year,
			Season:      event.Season,
			Episode:     event.Episode,
			Languages:   event.Languages,
			Error:       event.Error,
		}
	}

	return result
}

func llmStats(
	ctx context.Context,
	recorder *llmobs.Recorder,
	queueMetrics queuemetrics.Client,
	classifierConfig classifier.Config,
	windowMinutes *int,
) (gen.LlmStats, error) {
	window := time.Duration(0)
	if windowMinutes != nil {
		window = time.Duration(*windowMinutes) * time.Minute
	}

	stats := recorder.Stats(window)
	buckets, err := queueMetrics.Request(ctx, queuemetrics.Request{
		BucketDuration: "minute",
		Statuses:       []model.QueueJobStatus{model.QueueJobStatusPending},
		Queues:         []string{processor.MessageName},
	})
	if err != nil {
		return gen.LlmStats{}, err
	}

	queuePending := 0
	for _, bucket := range buckets {
		queuePending += int(bucket.Count)
	}

	perProvider := make([]gen.LlmProviderStats, len(stats.PerProvider))
	for i, provider := range stats.PerProvider {
		perProvider[i] = gen.LlmProviderStats{
			Provider:  provider.Provider,
			Attempted: int(provider.Attempted),
			Matched:   int(provider.Matched),
			Unmatched: int(provider.Unmatched),
			Errored:   int(provider.Errored),
		}
	}

	var successRate float64
	if stats.Attempted != 0 {
		successRate = float64(stats.Matched) / float64(stats.Attempted)
	}

	var oldestBuffered *time.Time
	if !stats.OldestBuffered.IsZero() {
		oldestBuffered = &stats.OldestBuffered
	}

	return gen.LlmStats{
		Attempted:           int(stats.Attempted),
		Matched:             int(stats.Matched),
		Unmatched:           int(stats.Unmatched),
		Errored:             int(stats.Errored),
		Skipped:             int(stats.Skipped),
		SuccessRate:         successRate,
		PerProvider:         perProvider,
		InFlight:            int(stats.InFlight),
		Concurrency:         classifierConfig.Concurrency,
		WindowStart:         stats.WindowStart,
		OldestBuffered:      oldestBuffered,
		WindowAttempted:     stats.WindowAttempted,
		LatencyP50Ms:        int(stats.LatencyP50.Milliseconds()),
		LatencyP95Ms:        int(stats.LatencyP95.Milliseconds()),
		ThroughputPerMinute: stats.ThroughputPerMinute,
		QueuePending:        queuePending,
	}, nil
}
