package llm

import (
	"testing"
	"time"
)

func TestStatsSnapshotAggregatesCurrentWindow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	stats := NewStats()
	stats.startedAt = now.Add(-time.Hour)
	stats.Record(Observation{
		Provider:         "test",
		At:               now.Add(-time.Minute),
		Duration:         time.Second,
		PromptTokens:     10,
		CompletionTokens: 5,
		Classifications:  2,
		ContentTypes:     []string{"movie", "movie"},
	})
	stats.Record(Observation{
		Provider:         "test",
		At:               now,
		Duration:         3 * time.Second,
		PromptTokens:     20,
		CompletionTokens: 8,
		Classifications:  1,
		ContentTypes:     []string{"tv_show"},
		Failed:           true,
	})
	stats.Record(Observation{
		Provider:        "test",
		At:              now.Add(-25 * time.Hour),
		Duration:        30 * time.Second,
		Classifications: 99,
	})

	got := stats.Snapshot(now)
	if got.Matched != 3 || got.PromptTokens != 30 || got.CompletionTokens != 13 {
		t.Fatalf("unexpected totals: %+v", got)
	}

	if got.AverageLatency != 2 || got.P95Latency != 3 || got.ErrorRate != 50 {
		t.Fatalf("unexpected request metrics: %+v", got)
	}

	if got.Distribution["movie"] != 2 || got.Distribution["tv_show"] != 1 {
		t.Fatalf("unexpected distribution: %+v", got.Distribution)
	}
}
