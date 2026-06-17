// Package llm provides a provider-agnostic interface for LLM-based torrent classification.
// It defines the contract that any LLM backend must satisfy, along with input/output types.
// Providers are configured globally under classifier.llm.providers in the classifier YAML config.
package llm

import (
	"context"
	"errors"
)

// Common errors returned by providers.
var (
	ErrNoResult    = errors.New("llm: no classification result returned")
	ErrInvalidJSON = errors.New("llm: provider returned invalid JSON")
)

// ClassifyInput contains the torrent information sent to the LLM for classification.
type ClassifyInput struct {
	// Name is the torrent name as stored in the database.
	Name string `json:"name"`
	// Files is the list of file paths within the torrent, ordered by size descending.
	// May be empty if file info is not available or was truncated.
	Files []string `json:"files,omitempty"`

	// ContentTypes is a comma-separated list of valid content types the LLM may choose from.
	// Populated from the configured domain enums at classification time.
	ContentTypes string `json:"content_types,omitempty"`
}

// ClassifyResult is the structured output returned by an LLM provider.
// String fields that map to domain enums (ContentType, VideoResolution, etc.)
// are validated against their respective enum types by the caller.
type ClassifyResult struct {
	// ContentType is the classification result: movie, tv_show, music, etc.
	// Must match one of the known values in model.ContentType.
	ContentType string `json:"content_type"`
	// Title is the human-readable content title resolved by the LLM.
	Title string `json:"title"`
	// Year is the release year, if determinable.
	Year int `json:"year,omitempty"`
	// Season number, applicable for tv_show classifications.
	Season int `json:"season,omitempty"`
	// Episode number, applicable for tv_show classifications.
	Episode int `json:"episode,omitempty"`
	// Language code (ISO 639-1) if determinable from the torrent.
	Language string `json:"language,omitempty"`
	// VideoResolution: V360p, V480p, V720p, V1080p, etc.
	VideoResolution string `json:"video_resolution,omitempty"`
	// VideoSource: CAM, TELESYNC, WEB, BluRay, etc.
	VideoSource string `json:"video_source,omitempty"`
	// VideoCodec: h264, x264, x265, XviD, etc.
	VideoCodec string `json:"video_codec,omitempty"`
	// ReleaseGroup is the scene or P2P group name.
	ReleaseGroup string `json:"release_group,omitempty"`
	// Tags are freeform labels derived from the torrent, e.g. ["multilingual", "remux"].
	Tags []string `json:"tags,omitempty"`
}

// Provider classifies a single torrent by name and optional file list.
type Provider interface {
	// Name returns a human-readable identifier for this provider instance,
	// typically matching the name configured in classifier.llm.providers.
	Name() string
	// Classify sends a single torrent to the LLM and returns the structured result.
	// If the provider is unreachable or returns an invalid response, an error is returned.
	// The caller should fall through to ErrUnmatched rather than propagating the error.
	Classify(ctx context.Context, input ClassifyInput) (*ClassifyResult, error)
}

// BatchProvider is an optional interface that providers may implement to support
// classifying multiple torrents in a single LLM call, which is more efficient
// for high-throughput backends.
type BatchProvider interface {
	Provider
	// BatchClassify sends multiple torrents in a single request and returns results
	// in the same order as the inputs. The number of results must equal the number of inputs.
	// If any individual item fails, its position in the returned slice should be nil
	// and the corresponding error returned separately. The caller is responsible for
	// partially-accepted batch handling.
	BatchClassify(ctx context.Context, inputs []ClassifyInput) ([]*ClassifyResult, error)
}

// Drainer is an optional interface for providers that hold background resources
// (timers, goroutines, connection pools) that must be flushed before shutdown.
// Call Drain() during OnStop before the HTTP stack tears down.
type Drainer interface {
	Drain()
}
