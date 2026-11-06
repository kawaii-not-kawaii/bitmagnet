// Package openai provides an LLM provider implementation compatible with the
// OpenAI Chat Completions API (/v1/chat/completions). It works with any
// OpenAI-compatible backend including llama.cpp, vLLM, Ollama, and OpenAI itself.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

const (
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
	// maxBatchRequestTimeout caps one batched HTTP attempt. Measured on the
	// OpenRouter free tier: 100 torrents per request took ~150s, 25 took up to ~180s.
	maxBatchRequestTimeout = 10 * time.Minute
	// retryBackoffSlack covers the backoff sleeps between attempts in BatchDeadline.
	retryBackoffSlack = time.Minute
)

// batchRequestTimeout is the per-attempt HTTP timeout for a request carrying n
// torrents. The configured timeout is per torrent, so it scales with n, capped
// at maxBatchRequestTimeout and never below the configured value.
func batchRequestTimeout(base time.Duration, n int) time.Duration {
	if n <= 1 {
		return base
	}

	scaled := base * time.Duration(n)
	if scaled > maxBatchRequestTimeout || scaled/time.Duration(n) != base {
		scaled = maxBatchRequestTimeout
	}

	return max(base, scaled)
}

// Config holds the configuration for an OpenAI-compatible provider.
type Config struct {
	// Name is the provider identifier used in classifier YAML config.
	Name string
	// BaseURL is the base URL of the OpenAI-compatible API (e.g. "http://localhost:8080").
	BaseURL string
	// Model is the model identifier sent in the API request.
	Model string
	// APIKey is an optional bearer token for authenticated endpoints.
	APIKey string
	// Timeout is the HTTP timeout per torrent: a single request gets this much,
	// a batch of n gets n times as much (see batchRequestTimeout). Default 30s.
	Timeout time.Duration
	// SystemPrompt is the system message template. Uses {{.ContentTypes}} variable.
	SystemPrompt string
	// UserPrompt is the user message template. Uses {{.Name}}, {{.Files}}, {{.ContentTypes}}.
	UserPrompt string
	// MaxContext is the configured per-request context budget. Zero disables trimming.
	MaxContext int
	// MaxTokens is the configured output budget. Zero defaults to 256.
	MaxTokens int
}

func (c *Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultTimeout
	}

	return c.Timeout
}

type client struct {
	config Config
	http   *http.Client
}

// New creates a new OpenAI-compatible provider.
func New(cfg Config) llm.Provider {
	return &client{
		config: cfg,
		// No client-wide timeout: each attempt gets its own deadline sized to
		// the request (see doRequestRaw / batchRequestTimeout).
		http: &http.Client{},
	}
}

func (c *client) Name() string { return c.config.Name }

// BatchDeadline is the longest a BatchClassify call for n torrents may take,
// including every retry. BatchClient uses it to size its flush deadline.
func (c *client) BatchDeadline(n int) time.Duration {
	return (maxRetries+1)*batchRequestTimeout(c.config.timeout(), n) + retryBackoffSlack
}

// chatMessage is a single message in the OpenAI chat format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body for /v1/chat/completions.
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponseMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func (m *chatResponseMessage) content() string {
	if m.Content != "" {
		return m.Content
	}

	return m.ReasoningContent
}

type chatResponseChoice struct {
	Message      chatResponseMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type chatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponseError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// chatResponse is the response body from /v1/chat/completions.
type chatResponse struct {
	Choices []chatResponseChoice `json:"choices"`
	Usage   chatResponseUsage    `json:"usage"`
	Error   *chatResponseError   `json:"error,omitempty"`
}

func (c *client) Classify(ctx context.Context, input llm.ClassifyInput) (*llm.ClassifyResult, error) {
	reqBytes, err := c.buildRequest(input)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}

	raw, err := c.doRequestRaw(ctx, reqBytes, c.config.timeout())
	if err != nil {
		return nil, err
	}

	var result llm.ClassifyResult

	parseErr := json.Unmarshal([]byte(raw.content), &result)
	result.PromptTokens = raw.usage.PromptTokens
	result.CompletionTokens = raw.usage.CompletionTokens

	if parseErr != nil {
		return &result, raw.wrapTruncated(fmt.Errorf("%w: %w", llm.ErrInvalidJSON, parseErr))
	}

	if result.ContentType == "" {
		return &result, llm.ErrNoResult
	}

	return &result, nil
}

// BatchClassify implements llm.BatchProvider. It sends multiple torrents
// in a single chat completion request without response_format constraint
// (to allow JSON array output).
func (c *client) BatchClassify(ctx context.Context, inputs []llm.ClassifyInput) ([]*llm.ClassifyResult, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	if len(inputs) == 1 {
		r, err := c.Classify(ctx, inputs[0])
		if err != nil {
			return nil, err
		}

		return []*llm.ClassifyResult{r}, nil
	}

	// Build batch prompt — merge content types from all inputs to avoid using only inputs[0].
	seen := make(map[string]struct{})

	var mergedTypes []string

	for _, inp := range inputs {
		for _, ct := range strings.Split(inp.ContentTypes, ", ") {
			ct = strings.TrimSpace(ct)
			if ct != "" {
				if _, ok := seen[ct]; !ok {
					seen[ct] = struct{}{}

					mergedTypes = append(mergedTypes, ct)
				}
			}
		}
	}

	mergedInput := llm.ClassifyInput{ContentTypes: strings.Join(mergedTypes, ", ")}

	userContent := BatchClassifyJSONString(inputs)
	batchSuffix := "\n\nYou are classifying multiple torrents at once." +
		" Return a JSON object with a \"results\" array" +
		" containing one classification per torrent, in the same order."
	systemContent := c.buildSystemMessage(mergedInput) + batchSuffix

	messages := []chatMessage{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: userContent},
	}

	req := chatRequest{
		Model:          c.config.Model,
		Messages:       messages,
		Temperature:    0.1,
		MaxTokens:      effectiveMaxTokens(c.config.MaxTokens) * len(inputs),
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: build batch request: %w", err)
	}

	raw, err := c.doRequestRaw(ctx, reqBytes, batchRequestTimeout(c.config.timeout(), len(inputs)))
	if err != nil {
		return nil, err
	}

	results, err := ParseBatchResponse(raw.content)
	if err != nil {
		return nil, raw.wrapTruncated(err)
	}

	return results, nil
}

func (c *client) buildRequest(input llm.ClassifyInput) ([]byte, error) {
	systemContent := c.buildSystemMessage(input)
	messages := []chatMessage{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: c.buildUserMessage(input, systemContent)},
	}

	req := chatRequest{
		Model:          c.config.Model,
		Messages:       messages,
		Temperature:    0.1,
		MaxTokens:      effectiveMaxTokens(c.config.MaxTokens),
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	return json.Marshal(req)
}

const defaultSystemPromptFmt = "You are a BitTorrent content classifier." +
	" Given a torrent name and optional file list, determine the content type and extract metadata." +
	"\n\nAvailable content types: %s" +
	"\n\nReturn valid JSON with fields: content_type, title, year, season, episode," +
	" language, video_resolution, video_source, video_codec, release_group, tags." +
	"\n\nRules:" +
	"\n- Use filename structure and file list to determine content type" +
	"\n- Look for S01E01 patterns for tv_show" +
	"\n- Look for years (1900-2099) to identify movies" +
	"\n- Music releases typically have .mp3/.flac files" +
	"\n- Return ONLY valid JSON"

func (c *client) buildSystemMessage(input llm.ClassifyInput) string {
	if c.config.SystemPrompt != "" {
		return strings.ReplaceAll(c.config.SystemPrompt, "{{.ContentTypes}}", input.ContentTypes)
	}

	return fmt.Sprintf(defaultSystemPromptFmt, input.ContentTypes)
}

func (c *client) buildUserMessage(input llm.ClassifyInput, systemContent string) string {
	included := min(len(input.Files), 20)
	omitted := len(input.Files) - included

	if c.config.MaxContext <= 0 {
		marker := ""
		if omitted > 0 {
			marker = fmt.Sprintf("... and %d more files", omitted)
		}

		return renderUserMessage(input, included, marker)
	}

	inputBudget := c.config.MaxContext - effectiveMaxTokens(c.config.MaxTokens)

	for {
		marker := ""
		if omitted > 0 {
			marker = fmt.Sprintf("(+%d more files)", omitted)
		}

		message := renderUserMessage(input, included, marker)
		if estimateTokens(systemContent)+estimateTokens(message) <= inputBudget || included == 0 {
			return message
		}

		included--
		omitted++
	}
}

func renderUserMessage(input llm.ClassifyInput, included int, marker string) string {
	var builder strings.Builder

	_, _ = builder.WriteString("Name: ")
	_, _ = builder.WriteString(input.Name)
	_ = builder.WriteByte('\n')

	for _, file := range input.Files[:included] {
		_, _ = builder.WriteString("File: ")
		_, _ = builder.WriteString(file)
		_ = builder.WriteByte('\n')
	}

	if marker != "" {
		_, _ = builder.WriteString(marker)
		_ = builder.WriteByte('\n')
	}

	return builder.String()
}

func estimateTokens(value string) int {
	// ponytail: chars/4 estimate; wire /v1/tokenize if precision ever matters.
	return (len(value) + 3) / 4
}

func effectiveMaxTokens(configured int) int {
	if configured > 0 {
		return configured
	}

	return 256
}

// stripCodeFence removes a surrounding markdown code fence, which many models emit
// around JSON even when response_format=json_object is requested — not every server
// enforces that parameter (lemonade/llama.cpp does not), so the fence reaches us and
// json.Unmarshal fails on the leading backtick. Content without a fence is returned
// unchanged.
func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return content
	}

	// drop the opening fence line, which may carry a language tag (```json)
	_, after, found := strings.Cut(trimmed, "\n")
	if !found {
		return content
	}

	if end := strings.LastIndex(after, "```"); end >= 0 {
		after = after[:end]
	}

	if stripped := strings.TrimSpace(after); stripped != "" {
		return stripped
	}

	return content
}

// rawResponse is the first choice of a successful chat completion.
type rawResponse struct {
	content      string
	usage        chatResponseUsage
	finishReason string
}

// wrapTruncated marks a parse failure as ErrOutputTruncated when the model
// stopped at its token budget: the JSON is cut off, not malformed.
func (r rawResponse) wrapTruncated(err error) error {
	if r.finishReason == finishReasonLength {
		return fmt.Errorf("%w: %w", llm.ErrOutputTruncated, err)
	}

	return err
}

const finishReasonLength = "length"

// doRequestRaw sends the request and returns the raw content of the first
// choice. timeout bounds each HTTP attempt separately (retries get a fresh one).
func (c *client) doRequestRaw(ctx context.Context, reqBody []byte, timeout time.Duration) (rawResponse, error) {
	baseURL := strings.TrimSuffix(strings.TrimRight(c.config.BaseURL, "/"), "/v1")
	url := baseURL + "/v1/chat/completions"

	var (
		lastErr       error
		retryDelay    time.Duration
		hasRetryDelay bool
	)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(100*math.Pow(2, float64(attempt-1))) * time.Millisecond

			if hasRetryDelay {
				backoff = retryDelay
				hasRetryDelay = false
			}

			select {
			case <-ctx.Done():
				return rawResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := c.attempt(ctx, url, reqBody, timeout)
		if err != nil {
			if errors.Is(err, errCreateRequest) {
				return rawResponse{}, err
			}

			lastErr = err

			continue
		}

		if resp.status < http.StatusOK || resp.status >= http.StatusMultipleChoices {
			detail := fmt.Sprintf("HTTP %d: %s", resp.status, strings.TrimSpace(string(resp.body)))
			if resp.status == http.StatusTooManyRequests {
				lastErr = fmt.Errorf("openai: %w: %s", llm.ErrRateLimited, detail)
				retryDelay, hasRetryDelay = parseRetryAfter(resp.retryAfter)

				continue
			}

			lastErr = fmt.Errorf("openai: %w: %s", llm.ErrBadStatus, detail)
			if resp.status >= 400 && resp.status < 500 {
				return rawResponse{}, lastErr
			}

			continue
		}

		var chatResp chatResponse
		if err := json.Unmarshal(resp.body, &chatResp); err != nil {
			lastErr = fmt.Errorf("openai: parse response: %w", err)
			continue
		}

		if chatResp.Error != nil {
			if chatResp.Error.Message != "" || chatResp.Error.Type != "" {
				// At least one field is set — identifiable error; retrying won't help.
				msg := chatResp.Error.Message
				if msg == "" {
					msg = "(no message)"
				}

				lastErr = fmt.Errorf("openai: API error: %s (type=%s)", msg, chatResp.Error.Type)

				return rawResponse{}, lastErr
			}
			// Both fields empty — ambiguous transient condition; retry.
			lastErr = fmt.Errorf("openai: API error: empty error object")

			continue
		}

		if len(chatResp.Choices) == 0 {
			return rawResponse{}, llm.ErrNoResult
		}

		choice := chatResp.Choices[0]
		raw := rawResponse{
			content:      stripCodeFence(choice.Message.content()),
			usage:        chatResp.Usage,
			finishReason: choice.FinishReason,
		}

		if raw.content == "" {
			return rawResponse{}, raw.wrapTruncated(llm.ErrNoResult)
		}

		return raw, nil
	}

	return rawResponse{}, fmt.Errorf("openai: %w (after %d retries)", lastErr, maxRetries)
}

var errCreateRequest = errors.New("openai: create request")

// attemptResponse is the part of an HTTP response doRequestRaw needs, captured
// before the body is closed.
type attemptResponse struct {
	status     int
	retryAfter string
	body       []byte
}

// attempt performs one HTTP round trip under its own timeout and returns the
// fully read body, so the per-attempt deadline also covers reading it.
func (c *client) attempt(
	ctx context.Context, url string, reqBody []byte, timeout time.Duration,
) (attemptResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return attemptResponse{}, fmt.Errorf("%w: %w", errCreateRequest, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if c.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return attemptResponse{}, fmt.Errorf("openai: request failed: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()

	if err != nil {
		return attemptResponse{}, fmt.Errorf("openai: read response: %w", err)
	}

	return attemptResponse{status: resp.StatusCode, retryAfter: resp.Header.Get("Retry-After"), body: body}, nil
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)

	if value == "" {
		return 0, false
	}

	if delay, err := time.ParseDuration(value + "s"); err == nil && delay >= 0 {
		return delay, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	return max(time.Until(retryAt), 0), true
}
