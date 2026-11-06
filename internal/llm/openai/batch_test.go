package openai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

// TestParseBatchResponse_SingleObject_RejectedForBatch covers the mis-attribution
// hazard fixed in ParseBatchResponse: when a batched request (N>1 inputs) gets a
// single un-batched result object back from the model, the parser must NOT silently
// turn it into a length-1 slice — flush() would then blind-assign it to input 0.
// The parser cannot know the original input count, so the safe behavior is to
// reject the single-object form with ErrInvalidJSON, which flush() routes to every
// pending caller so they fall back rather than receive wrong data.
//
// We simulate the N>1 condition by calling ParseBatchResponse with content that
// looks like a single classification object.
func TestParseBatchResponse_SingleObject_RejectedForBatch(t *testing.T) {
	t.Parallel()

	// A single classification object, not wrapped in {"results":[...]} or an array.
	singleObject := `{"content_type":"movie","title":"Some Movie","year":2021}`

	results, err := ParseBatchResponse(singleObject)
	if !errors.Is(err, llm.ErrInvalidJSON) {
		t.Fatalf(
			"expected ErrInvalidJSON for single-object batch response, got err=%v results=%v",
			err,
			results,
		)
	}

	if results != nil {
		t.Fatalf("expected nil results on error, got %v", results)
	}
}

// TestParseBatchResponse_SingleObject_NotAcceptedEvenForOneInput documents that
// ParseBatchResponse rejects a single bare JSON object even when the caller's
// intent is N==1. This is intentional: the function has no way to verify that a
// returned object actually corresponds to the caller's single input, and the only
// production caller (BatchClassify) already routes N==1 through Classify
// (client.go) — so it never reaches ParseBatchResponse in that case. The contract
// here is "give me an array form"; if you have a single object, use Classify.
func TestParseBatchResponse_SingleObject_NotAcceptedEvenForOneInput(t *testing.T) {
	t.Parallel()

	singleObject := `{"content_type":"tv_show","title":"One Show"}`

	results, err := ParseBatchResponse(singleObject)
	if !errors.Is(err, llm.ErrInvalidJSON) {
		t.Fatalf("expected ErrInvalidJSON, got err=%v results=%v", err, results)
	}

	if results != nil {
		t.Fatalf("expected nil results on error, got %v", results)
	}
}

// TestParseBatchResponse_ResultsArray_OK locks in the supported batched form.
func TestParseBatchResponse_ResultsArray_OK(t *testing.T) {
	t.Parallel()

	content := `{"results":[
		{"content_type":"movie","title":"A"},
		{"content_type":"tv_show","title":"B"}
	]}`

	results, err := ParseBatchResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].ContentType != testContentTypeMovie || results[1].ContentType != testContentTypeTVShow {
		t.Errorf("positional mismatch: got %q, %q", results[0].ContentType, results[1].ContentType)
	}
}

func TestParseBatchResponse_FencedResultsArray_OK(t *testing.T) {
	t.Parallel()

	content := "```json\n" +
		`{"results":[{"content_type":"movie","title":"A"},{"content_type":"tv_show","title":"B"}]}` +
		"\n```"

	results, err := ParseBatchResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 ||
		results[0].ContentType != testContentTypeMovie ||
		results[1].ContentType != testContentTypeTVShow {
		t.Fatalf("fenced results = %#v", results)
	}
}

// TestParseBatchResponse_BareArray_OK locks in the bare-array form.
func TestParseBatchResponse_BareArray_OK(t *testing.T) {
	t.Parallel()

	content := `[
		{"content_type":"music","title":"X"},
		{"content_type":"movie","title":"Y"}
	]`

	results, err := ParseBatchResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].ContentType != "music" || results[1].ContentType != testContentTypeMovie {
		t.Errorf("positional mismatch: got %q, %q", results[0].ContentType, results[1].ContentType)
	}
}

// TestParseBatchResponse_EmptyResultsArray covers the empty-array edge case.
// An empty array is valid JSON but carries no classifications — we treat it as
// no result so flush() fills every position with ErrNoResult.
func TestParseBatchResponse_EmptyResultsArray(t *testing.T) {
	t.Parallel()

	results, err := ParseBatchResponse(`[]`)
	if err != nil {
		t.Fatalf("unexpected error for empty array: %v", err)
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

type deadlineRecordingProvider struct {
	deadlines chan time.Duration
	perBatch  time.Duration
}

func (*deadlineRecordingProvider) Name() string { return "rec" }

func (*deadlineRecordingProvider) Classify(context.Context, llm.ClassifyInput) (*llm.ClassifyResult, error) {
	return nil, errors.New("unexpected single classify")
}

func (p *deadlineRecordingProvider) BatchClassify(
	ctx context.Context, inputs []llm.ClassifyInput,
) ([]*llm.ClassifyResult, error) {
	deadline, _ := ctx.Deadline()
	p.deadlines <- time.Until(deadline)

	out := make([]*llm.ClassifyResult, len(inputs))
	for i := range out {
		out[i] = &llm.ClassifyResult{ContentType: "movie"}
	}

	return out, nil
}

func (p *deadlineRecordingProvider) BatchDeadline(int) time.Duration { return p.perBatch }

// A large batch on a slow remote API can legitimately take minutes; the flush
// deadline must come from the provider (which knows its per-request timeout and
// retry policy), not the fixed single-request constant.
func TestBatchClient_FlushDeadlineFromProvider(t *testing.T) {
	t.Parallel()

	rec := &deadlineRecordingProvider{deadlines: make(chan time.Duration, 1), perBatch: 30 * time.Minute}
	bc := NewBatchClient(rec, 2, time.Hour)

	done := make(chan struct{})

	for range 2 {
		go func() {
			_, _ = bc.Classify(context.Background(), llm.ClassifyInput{Name: "x"})
			done <- struct{}{}
		}()
	}

	<-done
	<-done

	if got := <-rec.deadlines; got <= flushTimeout || got > 30*time.Minute {
		t.Fatalf("flush deadline = %v, want provider's 30m (> fallback %v)", got, flushTimeout)
	}
}
