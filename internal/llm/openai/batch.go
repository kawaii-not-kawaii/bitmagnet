package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

// flushTimeout is the maximum wall-clock time a single batch HTTP call may take,
// sized to cover the worst-case retry chain: (maxRetries+1) × per-request HTTP timeout.
const flushTimeout = (maxRetries + 1) * defaultTimeout

// BatchClient wraps a Provider and accumulates single Classify requests
// into batch calls of up to batchSize, flushing on size or timer.
type BatchClient struct {
	provider   llm.Provider
	batchSize  int
	flushAfter time.Duration

	mu      sync.Mutex
	pending []pendingRequest
	timer   *time.Timer
	drained bool // set by Drain(); checked in Classify to reject post-shutdown enqueues

	wg sync.WaitGroup // tracks in-flight flush() calls for graceful shutdown

	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
}

type pendingRequest struct {
	input  llm.ClassifyInput
	result chan<- batchResult
}

type batchResult struct {
	result *llm.ClassifyResult
	err    error
}

// NewBatchClient wraps a provider with automatic batching.
// batchSize=1 disables batching (passthrough).
func NewBatchClient(provider llm.Provider, batchSize int, flushAfter time.Duration) llm.Provider {
	if batchSize <= 1 {
		return provider // no batching needed
	}

	ctx, cancel := context.WithCancel(context.Background())
	bc := &BatchClient{
		provider:        provider,
		batchSize:       batchSize,
		flushAfter:      flushAfter,
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}

	return bc
}

func (bc *BatchClient) Name() string { return bc.provider.Name() }

func (bc *BatchClient) Classify(ctx context.Context, input llm.ClassifyInput) (*llm.ClassifyResult, error) {
	// If provider doesn't support batch, fall through to single request
	if _, ok := bc.provider.(llm.BatchProvider); !ok {
		return bc.provider.Classify(ctx, input)
	}

	resultCh := make(chan batchResult, 1)
	req := pendingRequest{input: input, result: resultCh}

	bc.mu.Lock()
	if bc.drained {
		bc.mu.Unlock()
		return nil, fmt.Errorf("llm batch: shutting down")
	}

	bc.pending = append(bc.pending, req)

	// Start timer on first pending item
	if len(bc.pending) == 1 {
		bc.timer = time.AfterFunc(bc.flushAfter, bc.timedFlush)
	}

	shouldFlush := len(bc.pending) >= bc.batchSize
	bc.mu.Unlock()

	if shouldFlush {
		bc.flush() //nolint:contextcheck // flush uses its own lifecycle context to serve multiple callers
	}

	select {
	case r := <-resultCh:
		return r.result, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (bc *BatchClient) timedFlush() {
	bc.flush()
}

func (bc *BatchClient) flush() {
	bc.mu.Lock()
	if len(bc.pending) == 0 {
		bc.mu.Unlock()
		return
	}

	if bc.timer != nil {
		bc.timer.Stop()
		bc.timer = nil
	}

	batch := bc.pending
	bc.pending = nil
	// Add to WaitGroup while holding the lock so Drain()'s wg.Wait() cannot
	// race past this point before the counter is incremented.
	bc.wg.Add(1)
	bc.mu.Unlock()
	defer bc.wg.Done()

	// Extract inputs
	inputs := make([]llm.ClassifyInput, len(batch))
	for i, r := range batch {
		inputs[i] = r.input
	}

	// Call batch classify
	bp, ok := bc.provider.(llm.BatchProvider)
	if !ok {
		for _, r := range batch {
			r.result <- batchResult{err: fmt.Errorf("llm: provider does not implement BatchProvider")}
		}

		return
	}

	ctx, cancel := context.WithTimeout(bc.lifecycleCtx, flushTimeout)
	defer cancel()

	results, err := bp.BatchClassify(ctx, inputs)

	// Distribute results
	if err == nil && len(results) != len(batch) {
		log.Printf(
			"llm batch: expected %d results, got %d — filling short positions with ErrNoResult",
			len(batch),
			len(results),
		)
	}

	for i, r := range batch {
		if err != nil {
			r.result <- batchResult{err: err}
			continue
		}

		if i < len(results) && results[i] != nil {
			r.result <- batchResult{result: results[i]}
		} else {
			r.result <- batchResult{err: llm.ErrNoResult}
		}
	}
}

// Drain stops accepting new requests, routes any pending callers to error, and
// cancels the lifecycle context so in-flight HTTP requests are aborted.
// It blocks until all in-flight flush goroutines have returned.
// Call this during application shutdown before the HTTP stack tears down.
func (bc *BatchClient) Drain() {
	bc.mu.Lock()

	bc.drained = true
	if bc.timer != nil {
		bc.timer.Stop()
		bc.timer = nil
	}

	batch := bc.pending
	bc.pending = nil
	bc.mu.Unlock()

	// Cancel in-flight HTTP requests, then wait for flush goroutines to exit.
	bc.lifecycleCancel()

	for _, r := range batch {
		r.result <- batchResult{err: fmt.Errorf("llm batch: shutting down")}
	}

	bc.wg.Wait()
}

// BatchClassifyJSONString builds a user message containing multiple torrents.
func BatchClassifyJSONString(inputs []llm.ClassifyInput) string {
	var msg string
	for i, input := range inputs {
		msg += fmt.Sprintf("[%d] Name: %s\n", i+1, input.Name)

		if len(input.Files) > 0 {
			maxFiles := 5
			if len(input.Files) < maxFiles {
				maxFiles = len(input.Files)
			}

			msg += "    Files: "

			for j := range maxFiles {
				if j > 0 {
					msg += " | "
				}

				msg += input.Files[j]
			}

			msg += "\n"
		}
	}

	msg += "\nReturn a JSON object: {\"results\":" +
		" [{\"content_type\": \"...\", \"title\": \"...\", \"year\": 0}, ...]}" +
		" with one entry per torrent in order."

	return msg
}

// ParseBatchResponse parses a batch response containing multiple results.
// It accepts either a JSON object with a "results" array or a bare JSON array.
//
// It deliberately does NOT accept a single bare JSON object as a one-element
// batch. The only production caller, BatchClassify, short-circuits len(inputs)==1
// through Classify (client.go), so this function is only reached when len(inputs)>=2.
// In that case a single returned object cannot be positionally attributed to any
// specific input — assigning it to index 0 would silently mis-classify whichever
// torrent happened to be first in the batch. Rejecting the single-object form
// returns ErrInvalidJSON, which flush() routes to every pending caller so they
// fall back to the upstream ErrUnmatched path instead of receiving wrong data.
func ParseBatchResponse(content string) ([]*llm.ClassifyResult, error) {
	content = stripMarkdownCodeFence(content)
	// Try to parse as JSON object with "results" array
	var wrapper struct {
		Results []*llm.ClassifyResult `json:"results"`
	}

	if err := json.Unmarshal([]byte(content), &wrapper); err == nil && len(wrapper.Results) > 0 {
		return wrapper.Results, nil
	}

	// Try as bare JSON array
	var arr []*llm.ClassifyResult
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		return arr, nil
	}

	// Do NOT accept a single bare object: see function doc comment.
	return nil, llm.ErrInvalidJSON
}

func stripMarkdownCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") || !strings.HasSuffix(content, "```") {
		return content
	}

	lineEnd := strings.IndexByte(content, '\n')
	if lineEnd < 0 {
		return content
	}

	return strings.TrimSpace(strings.TrimSuffix(content[lineEnd+1:], "```"))
}
