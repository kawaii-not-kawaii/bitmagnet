package resolvers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/metrics/queuemetrics"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/processor"
)

func TestLlmQuery_Events(t *testing.T) {
	t.Parallel()

	recorder := llmobs.New()
	recorder.Record(llmobs.Event{
		Timestamp:   time.Unix(1, 0),
		InfoHash:    "old-hash",
		TorrentName: "Old torrent",
		Outcome:     llmobs.OutcomeUnmatched,
	})
	recorder.Record(llmobs.Event{
		Timestamp:   time.Unix(2, 0),
		InfoHash:    "new-hash",
		TorrentName: "New torrent",
		Provider:    "gemma",
		Duration:    1250 * time.Millisecond,
		Outcome:     llmobs.OutcomeMatched,
		ContentType: "movie",
		Title:       "New",
		Year:        2026,
		Languages:   []string{"en"},
	})

	limit := 1
	events, err := (&llmQueryResolver{&Resolver{LlmRecorder: recorder}}).Events(
		context.Background(),
		&gen.LlmQuery{},
		&limit,
	)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Events returned %d items, want 1", len(events))
	}

	event := events[0]
	if event.InfoHash != "new-hash" || event.TorrentName != "New torrent" {
		t.Fatalf("newest event = %#v", event)
	}
	if event.Provider != "gemma" || event.DurationMs != 1250 ||
		event.Outcome != gen.LlmClassificationOutcomeMatched {
		t.Errorf("event classification fields = %#v", event)
	}
	if event.ContentType != "movie" || event.Title != "New" || event.Year != 2026 {
		t.Errorf("event parsed fields = %#v", event)
	}
	if len(event.Languages) != 1 || event.Languages[0] != "en" {
		t.Errorf("event languages = %v, want [en]", event.Languages)
	}

	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Events payload: %v", err)
	}

	for _, credentialField := range []string{"apiKey", "api_key", "password", "token"} {
		if strings.Contains(string(payload), credentialField) {
			t.Errorf("Events payload contains credential field %q: %s", credentialField, payload)
		}
	}
}

func TestLlmQuery_Stats(t *testing.T) {
	t.Parallel()

	recorder := llmobs.New()
	now := time.Now()
	recorder.Record(llmobs.Event{
		Timestamp: now.Add(-time.Minute),
		Provider:  "gemma",
		Duration:  100 * time.Millisecond,
		Outcome:   llmobs.OutcomeMatched,
	})
	recorder.Record(llmobs.Event{
		Timestamp: now.Add(-30 * time.Second),
		Provider:  "gemma",
		Duration:  300 * time.Millisecond,
		Outcome:   llmobs.OutcomeError,
	})

	finish := recorder.Begin()
	defer finish()

	queueMetrics := &llmQueueMetricsClient{
		buckets: []queuemetrics.Bucket{{Count: 7}},
	}
	windowMinutes := 15
	stats, err := (&llmQueryResolver{&Resolver{
		LlmRecorder:        recorder,
		QueueMetricsClient: queueMetrics,
		ClassifierConfig:   classifier.Config{Concurrency: 8},
	}}).Stats(context.Background(), &gen.LlmQuery{}, &windowMinutes)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.Attempted != 2 || stats.Matched != 1 || stats.Errored != 1 {
		t.Errorf(
			"lifetime counts = attempted %d, matched %d, errored %d",
			stats.Attempted,
			stats.Matched,
			stats.Errored,
		)
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("successRate = %v, want 0.5", stats.SuccessRate)
	}

	if stats.WindowAttempted != 2 || stats.LatencyP50Ms != 100 || stats.LatencyP95Ms != 300 {
		t.Errorf(
			"window stats = attempted %d, p50 %dms, p95 %dms",
			stats.WindowAttempted,
			stats.LatencyP50Ms,
			stats.LatencyP95Ms,
		)
	}

	if stats.InFlight != 1 || stats.Concurrency != 8 || stats.QueuePending != 7 {
		t.Errorf(
			"utilization = inFlight %d, concurrency %d, queuePending %d",
			stats.InFlight,
			stats.Concurrency,
			stats.QueuePending,
		)
	}

	if stats.WindowStart.IsZero() || stats.ThroughputPerMinute <= 0 {
		t.Errorf("window fields = start %v, throughput %v", stats.WindowStart, stats.ThroughputPerMinute)
	}
	if len(stats.PerProvider) != 1 || stats.PerProvider[0].Provider != "gemma" ||
		stats.PerProvider[0].Attempted != 2 {
		t.Errorf("perProvider = %#v", stats.PerProvider)
	}

	if len(queueMetrics.request.Statuses) != 1 || queueMetrics.request.Statuses[0] != model.QueueJobStatusPending {
		t.Errorf("queue status filter = %v", queueMetrics.request.Statuses)
	}
	if len(queueMetrics.request.Queues) != 1 || queueMetrics.request.Queues[0] != processor.MessageName {
		t.Errorf("queue filter = %v", queueMetrics.request.Queues)
	}
	if queueMetrics.request.BucketDuration != "minute" {
		t.Errorf("bucket duration = %q, want minute", queueMetrics.request.BucketDuration)
	}
}

type llmQueueMetricsClient struct {
	request queuemetrics.Request
	buckets []queuemetrics.Bucket
}

func (c *llmQueueMetricsClient) Request(
	_ context.Context,
	request queuemetrics.Request,
) ([]queuemetrics.Bucket, error) {
	c.request = request

	return c.buckets, nil
}
