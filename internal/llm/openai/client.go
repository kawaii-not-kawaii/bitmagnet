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
	Model       string         `json:"model"`
	Messages    []chatMessage  `json:"messages"`
	Temperature float64        `json:"temperature"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// chatResponse is the response body from /v1/chat/completions.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *client) Classify(ctx context.Context, input llm.ClassifyInput) (*llm.ClassifyResult, error) {
	reqBytes, err := c.buildRequest(input)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}

	result, err := c.doRequest(ctx, reqBytes)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *client) buildRequest(input llm.ClassifyInput) ([]byte, error) {
	userContent := c.buildUserMessage(input, false)
	messages := []chatMessage{
		{Role: "system", Content: c.buildSystemMessage(input)},
		{Role: "user", Content: userContent},
	}

	req := chatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: 0.1,
		MaxTokens:   c.estimateMaxTokens(input),
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	return json.Marshal(req)
}

func (c *client) buildSystemMessage(input llm.ClassifyInput) string {
	if c.config.SystemPrompt != "" {
		s := c.config.SystemPrompt
		s = strings.ReplaceAll(s, "{{.ContentTypes}}", input.ContentTypes)
		return s
	}
	// Default system prompt.
	return fmt.Sprintf(`You are a BitTorrent content classifier. Given a torrent name and optional file list, determine the content type and extract metadata.

Available content types: %s

Return valid JSON with fields: content_type, title, year, season, episode, language, video_resolution, video_source, video_codec, release_group, tags.

Rules:
- Use filename structure and file list to determine content type
- Look for S01E01 patterns for tv_show
- Look for years (1900-2099) to identify movies
- Music releases typically have .mp3/.flac files
- Return ONLY valid JSON`, input.ContentTypes)
}

func (c *client) buildUserMessage(input llm.ClassifyInput, batchMode bool) string {
	var b strings.Builder
	b.WriteString("Name: ")
	b.WriteString(input.Name)
	b.WriteByte('\n')

	for i, f := range input.Files {
		b.WriteString("File: ")
		b.WriteString(f)
		b.WriteByte('\n')
		if i >= 20 {
			b.WriteString("... and ")
			b.WriteString(fmt.Sprintf("%d more files", len(input.Files)-i-1))
			b.WriteByte('\n')
			break
		}
	}

	return b.String()
}

func (c *client) estimateMaxTokens(input llm.ClassifyInput) int {
	// Rough estimate: 256 tokens for output is typically enough for a single classification result.
	_ = input
	return 256
}

func (c *client) doRequest(ctx context.Context, reqBody []byte) (*llm.ClassifyResult, error) {
	url := strings.TrimRight(c.config.BaseURL, "/") + "/v1/chat/completions"

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Backoff: 100ms, 200ms, 400ms
			backoff := time.Duration(100*math.Pow(2, float64(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("openai: create request: %w", err)
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

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("openai: read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			// Don't retry 4xx errors.
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			lastErr = fmt.Errorf("openai: parse response: %w", err)
			continue
		}

		// Check for API-level error (e.g. model overloaded).
		if chatResp.Error != nil && chatResp.Error.Message != "" {
			lastErr = fmt.Errorf("openai: API error: %s (%s)", chatResp.Error.Message, chatResp.Error.Type)
			continue
		}

		if len(chatResp.Choices) == 0 {
			return nil, llm.ErrNoResult
		}

		content := chatResp.Choices[0].Message.Content
		if content == "" {
			return nil, llm.ErrNoResult
		}

		var result llm.ClassifyResult
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return nil, fmt.Errorf("%w: %s", llm.ErrInvalidJSON, err)
		}

		if result.ContentType == "" {
			return nil, llm.ErrNoResult
		}

		return &result, nil
	}

	return nil, fmt.Errorf("openai: %w (after %d retries)", lastErr, maxRetries)
}
