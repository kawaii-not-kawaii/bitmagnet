// Package openai provides an LLM provider implementation compatible with the
// OpenAI Chat Completions API (/v1/chat/completions). It works with any
// OpenAI-compatible backend including llama.cpp, vLLM, Ollama, and OpenAI itself.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
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
)

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
	// Timeout is the per-request HTTP timeout. Default 30s.
	Timeout time.Duration
	// SystemPrompt is the system message template. Uses {{.ContentTypes}} variable.
	SystemPrompt string
	// UserPrompt is the user message template. Uses {{.Name}}, {{.Files}}, {{.ContentTypes}}.
	UserPrompt string
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
		http: &http.Client{
			Timeout: cfg.timeout(),
		},
	}
}

func (c *client) Name() string { return c.config.Name }

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
	Content string `json:"content"`
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

	content, usage, err := c.doRequestRaw(ctx, reqBytes)
	if err != nil {
		return nil, err
	}

	var result llm.ClassifyResult

	parseErr := json.Unmarshal([]byte(content), &result)
	result.PromptTokens = usage.PromptTokens
	result.CompletionTokens = usage.CompletionTokens

	if parseErr != nil {
		return &result, fmt.Errorf("%w: %w", llm.ErrInvalidJSON, parseErr)
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
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: 0.1,
		MaxTokens:   256 * len(inputs),
		// No response_format — allow free JSON array output
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: build batch request: %w", err)
	}

	content, _, err := c.doRequestRaw(ctx, reqBytes)
	if err != nil {
		return nil, err
	}

	return ParseBatchResponse(content)
}

func (c *client) buildRequest(input llm.ClassifyInput) ([]byte, error) {
	userContent := c.buildUserMessage(input)
	messages := []chatMessage{
		{Role: "system", Content: c.buildSystemMessage(input)},
		{Role: "user", Content: userContent},
	}

	req := chatRequest{
		Model:          c.config.Model,
		Messages:       messages,
		Temperature:    0.1,
		MaxTokens:      c.estimateMaxTokens(input),
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

func (*client) buildUserMessage(input llm.ClassifyInput) string {
	var b strings.Builder

	b.WriteString("Name: ")
	b.WriteString(input.Name)
	b.WriteByte('\n')

	for i, f := range input.Files {
		if i >= 20 {
			b.WriteString(fmt.Sprintf("... and %d more files\n", len(input.Files)-20))
			break
		}

		b.WriteString("File: ")
		b.WriteString(f)
		b.WriteByte('\n')
	}

	return b.String()
}

func (*client) estimateMaxTokens(input llm.ClassifyInput) int {
	// Rough estimate: 256 tokens for output is typically enough for a single classification result.
	_ = input
	return 256
}

// doRequestRaw sends the request and returns the raw content string from the first choice.
// Used by BatchClassify which needs the raw content for array parsing.
func (c *client) doRequestRaw(ctx context.Context, reqBody []byte) (string, chatResponseUsage, error) {
	url := strings.TrimRight(c.config.BaseURL, "/") + "/v1/chat/completions"

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(100*math.Pow(2, float64(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return "", chatResponseUsage{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return "", chatResponseUsage{}, fmt.Errorf("openai: create request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")

		if c.config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}

		resp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("openai: request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("openai: read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return "", chatResponseUsage{}, lastErr
			}

			continue
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
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

				return "", chatResponseUsage{}, lastErr
			}
			// Both fields empty — ambiguous transient condition; retry.
			lastErr = fmt.Errorf("openai: API error: empty error object")

			continue
		}

		if len(chatResp.Choices) == 0 {
			return "", chatResponseUsage{}, llm.ErrNoResult
		}

		content := chatResp.Choices[0].Message.Content
		if content == "" {
			return "", chatResponseUsage{}, llm.ErrNoResult
		}

		return content, chatResp.Usage, nil
	}

	return "", chatResponseUsage{}, fmt.Errorf("openai: %w (after %d retries)", lastErr, maxRetries)
}
