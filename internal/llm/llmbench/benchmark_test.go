package llmbench

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

type testProvider struct {
	calls atomic.Int64
}

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) Classify(_ context.Context, input llm.ClassifyInput) (*llm.ClassifyResult, error) {
	p.calls.Add(1)

	switch input.Name {
	case "matched":
		return &llm.ClassifyResult{ContentType: "movie", Title: "Matched"}, nil
	case "unmatched":
		return nil, llm.ErrNoResult
	default:
		return nil, errors.New("provider failed")
	}
}

func TestRunSelectsAndCapsSamples(t *testing.T) {
	t.Parallel()

	provider := &testProvider{}
	loaded := []model.Torrent{{Name: "matched"}, {Name: "unmatched"}, {Name: "error"}}
	requestedLimit := 0

	result, err := Run(
		context.Background(),
		"test",
		provider,
		MaxSampleSize+100,
		func(_ context.Context, limit int, _ bool) ([]model.Torrent, error) {
			requestedLimit = limit

			return loaded, nil
		},
		Options{Concurrency: 2},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if requestedLimit != MaxSampleSize {
		t.Errorf("loader limit = %d, want %d", requestedLimit, MaxSampleSize)
	}

	if result.TorrentCount != len(loaded) || provider.calls.Load() != int64(len(loaded)) {
		t.Errorf(
			"selected %d torrents with %d calls, want %d",
			result.TorrentCount,
			provider.calls.Load(),
			len(loaded),
		)
	}

	if result.Matched != 1 || result.Unmatched != 1 || result.Errored != 1 {
		t.Errorf(
			"outcomes = %d matched/%d unmatched/%d errored, want 1/1/1",
			result.Matched,
			result.Unmatched,
			result.Errored,
		)
	}
}

func TestRunIsDryRun(t *testing.T) {
	t.Parallel()

	provider := &testProvider{}
	torrents := []model.Torrent{{Name: "matched"}}
	before := append([]model.Torrent(nil), torrents...)

	_, err := Run(
		context.Background(),
		"test",
		provider,
		1,
		func(context.Context, int, bool) ([]model.Torrent, error) { return torrents, nil },
		Options{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !reflect.DeepEqual(torrents, before) {
		t.Fatalf("benchmark mutated selected torrents: got %#v, want %#v", torrents, before)
	}
}

func TestRunRejectsNonPositiveSampleSize(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), "test", &testProvider{}, 0, nil, Options{})
	if err == nil {
		t.Fatal("expected non-positive sample size error")
	}
}
