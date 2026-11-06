// Package armiger is a small client for the armiger-ultra torrent classifier
// service. It isolates the HTTP contract (request fields and the response
// fields we read) so a service version bump touches one place.
package armiger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultTimeout is used when the configured timeout is not positive.
const DefaultTimeout = 2 * time.Second

// maxResponseBytes bounds how much of a response is read; the service's
// response is large (many heads) but far below this.
const maxResponseBytes = 4 << 20

// Categories are the armiger category values we record; anything else is
// mapped to CategoryUnknown.
const CategoryUnknown = "unknown"

var knownCategories = map[string]struct{}{
	"movie":         {},
	"tv_show":       {},
	"xxx":           {},
	"music":         {},
	"comic":         {},
	"game":          {},
	"audiobook":     {},
	"software":      {},
	"ebook":         {},
	"course":        {},
	CategoryUnknown: {},
}

// Config is the runtime configuration of the client.
type Config struct {
	Enabled bool
	URL     string
	Timeout time.Duration
}

// File is a torrent file as sent to armiger.
type File struct {
	Path string `json:"path"`
	Size uint   `json:"size"`
}

type classifyRequest struct {
	Name  string `json:"name"`
	Size  uint   `json:"size,omitempty"`
	Files []File `json:"files,omitempty"`
}

type categoryHead struct {
	Confidence *float64 `json:"confidence"`
}

type responseHeads struct {
	Category *categoryHead `json:"category"`
}

type classifyResponse struct {
	Category string        `json:"category"`
	Heads    responseHeads `json:"heads"`
}

// Client calls an armiger-ultra service. Its configuration can be replaced at
// runtime with Update; it is safe for concurrent use.
type Client struct {
	cfg  atomic.Pointer[Config]
	http *http.Client
}

func NewClient(cfg Config) *Client {
	c := &Client{http: &http.Client{}}
	c.Update(cfg)

	return c
}

// Update replaces the client's configuration.
func (c *Client) Update(cfg Config) {
	c.cfg.Store(&cfg)
}

// Enabled reports whether the client is enabled and has a URL to call.
func (c *Client) Enabled() bool {
	cfg := c.cfg.Load()

	return cfg != nil && cfg.Enabled && strings.TrimSpace(cfg.URL) != ""
}

// Classify asks armiger for the category of a torrent and the confidence of
// that prediction (0-1). Categories armiger returns that we don't recognise
// are reported as CategoryUnknown.
func (c *Client) Classify(
	ctx context.Context,
	name string,
	size uint,
	files []File,
) (string, float64, error) {
	cfg := c.cfg.Load()
	if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.URL) == "" {
		return "", 0, errors.New("armiger is not enabled")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(classifyRequest{Name: name, Size: size, Files: files})
	if err != nil {
		return "", 0, fmt.Errorf("encoding request: %w", err)
	}

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.URL), "/") + "/v1/classify"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return "", 0, fmt.Errorf("reading response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	var parsed classifyResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, fmt.Errorf("decoding response: %w", err)
	}

	if parsed.Heads.Category == nil || parsed.Heads.Category.Confidence == nil {
		return "", 0, errors.New("response has no heads.category.confidence")
	}

	category := parsed.Category
	if _, ok := knownCategories[category]; !ok {
		category = CategoryUnknown
	}

	return category, *parsed.Heads.Category.Confidence, nil
}
