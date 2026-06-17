package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

// BatchClient wraps a Provider and accumulates single Classify requests
// into batch calls of up to batchSize, flushing on size or timer.
type BatchClient struct {
	provider   llm.Provider
	batchSize  int
	flushAfter time.Duration

	mu      sync.Mutex
	pending []pendingRequest
	timer   *time.Timer

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
	bc.pending = append(bc.pending, req)

	// Start timer on first pending item
	if len(bc.pending) == 1 {
		bc.timer = time.AfterFunc(bc.flushAfter, bc.timedFlush)
	}

	shouldFlush := len(bc.pending) >= bc.batchSize
	bc.mu.Unlock()

	if shouldFlush {
		bc.flush()
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
	bc.mu.Unlock()

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

	ctx, cancel := context.WithTimeout(bc.lifecycleCtx, 60*time.Second)
	defer cancel()

	results, err := bp.BatchClassify(ctx, inputs)

	// Distribute results
	if err == nil && len(results) != len(batch) {
		log.Printf("llm batch: expected %d results, got %d — filling short positions with ErrNoResult", len(batch), len(results))
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
// Call this during application shutdown.
func (bc *BatchClient) Drain() {
	bc.mu.Lock()
	if bc.timer != nil {
		bc.timer.Stop()
		bc.timer = nil
	}
	batch := bc.pending
	bc.pending = nil
	bc.mu.Unlock()

	bc.lifecycleCancel()

	for _, r := range batch {
		r.result <- batchResult{err: fmt.Errorf("llm batch: shutting down")}
	}
}

// BatchClassifyJSON sends multiple torrents in a single chat completion request.
// This is the actual batch implementation that works with OpenAI-compatible endpoints
// by constructing a prompt with multiple torrents and parsing the array response.
type batchChatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
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
			for j := 0; j < maxFiles; j++ {
				if j > 0 {
					msg += " | "
				}
				msg += input.Files[j]
			}
			msg += "\n"
		}
	}
	msg += "\nReturn a JSON object: {\"results\": [{\"content_type\": \"...\", \"title\": \"...\", \"year\": 0}, ...]} with one entry per torrent in order."
	return msg
}

// ParseBatchResponse parses a batch response containing multiple results.
func ParseBatchResponse(content string) ([]*llm.ClassifyResult, error) {
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

	// Try as single result (model didn't batch)
	var single llm.ClassifyResult
	if err := json.Unmarshal([]byte(content), &single); err == nil {
		return []*llm.ClassifyResult{&single}, nil
	}

	return nil, llm.ErrInvalidJSON
}
