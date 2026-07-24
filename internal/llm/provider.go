// Package llm provides a provider-agnostic interface for LLM-based torrent classification.
// It defines the contract that any LLM backend must satisfy, along with input/output types.
// Providers are configured globally under classifier.llm.providers in the classifier YAML config.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
)

// Common errors returned by providers.
var (
	ErrNoResult    = errors.New("llm: no classification result returned")
	ErrInvalidJSON = errors.New("llm: provider returned invalid JSON")
	ErrRateLimited = errors.New("llm: provider rate limited")
	// ErrBadStatus wraps a non-success HTTP status that is not a rate-limit
	// (429). Providers SHOULD wrap non-2xx responses with it so callers can
	// distinguish "the URL/endpoint is wrong" from other failures.
	ErrBadStatus = errors.New("llm: provider returned a non-success HTTP status")
)

// ErrorCategory is a stable, low-cardinality classification of a provider error,
// used for observability (Prometheus labels, stats breakdowns) so operators can
// self-diagnose without reading Go source. Derived purely from typed errors via
// errors.Is/As so the categories stay correct as error messages change.
type ErrorCategory string

const (
	CategoryRateLimited  ErrorCategory = "rate-limited"
	CategoryConnection   ErrorCategory = "connection"
	CategoryBadStatus    ErrorCategory = "bad-status"
	CategoryInvalidJSON  ErrorCategory = "invalid-json"
	CategoryEmptyContent ErrorCategory = "empty-content"
	CategoryOther        ErrorCategory = "other"
)

// Categorize maps a provider error to a stable ErrorCategory. It returns the
// empty string for a nil error. Sentinel checks take precedence over the
// net.Error fallback (an HTTP 429/non-2xx is never a transport error), and any
// unrecognized error collapses to CategoryOther.
func Categorize(err error) ErrorCategory {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrRateLimited):
		return CategoryRateLimited
	case errors.Is(err, ErrBadStatus):
		return CategoryBadStatus
	case errors.Is(err, ErrInvalidJSON):
		return CategoryInvalidJSON
	case errors.Is(err, ErrNoResult):
		return CategoryEmptyContent
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return CategoryConnection
	}

	return CategoryOther
}

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
	// Language codes (ISO 639-1) if determinable from the torrent. Single code
	// for a mono-language release, multiple for multi-language packs (e.g.
	// ["rus","spa"]). Populated from the LLM response; the custom UnmarshalJSON
	// accepts either a single JSON string or a JSON array of strings. Validation
	// against the canonical language list happens downstream in applyLLMResult.
	Language []string `json:"language,omitempty"`
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
	// PromptTokens and CompletionTokens are provider-reported usage metadata.
	// They are not part of the model's JSON classification payload.
	PromptTokens     int `json:"-"`
	CompletionTokens int `json:"-"`
}

// flexInt is an int that tolerates the real-world type variations LLMs emit
// for numeric ClassifyResult fields (Year, Season, Episode). It accepts:
//   - a JSON number (int or float):       26 / 2024.0 / 1.5 -> 26 / 2024 / 1
//   - a JSON string of a number:          "26" / "1.5"     -> 26 / 1
//   - a JSON array of the above:          [1,2,3]          -> 1 (first element)
//   - JSON null, "", non-numeric:                          -> 0 (treated as unset)
//
// Float handling: LLMs frequently emit "year": 2024.0 for integer fields —
// it is the same class of type sloppiness this type exists to absorb, so
// floats are accepted and truncated toward zero, matching Go's int(float64)
// conversion. Integral floats (2024.0 -> 2024) and non-integral floats
// (2024.7 -> 2024, -1.5 -> -1) are treated identically: drawing a semantic
// line between "trailing .0" and "trailing .5" would be arbitrary and is
// not visible to the model. The downstream classifier's `> 0` guards
// (action_llm_classify.go:111-121) filter implausible results (zeros and
// negatives), so silent truncation cannot corrupt stored data — at worst
// it yields a value the caller already knows how to reject. NaN, ±Inf, and
// values outside int range collapse to 0 to avoid wraparound.
//
// Array rationale: a multi-episode pack returns "episode": [1,2,3]. The
// downstream classifier can only store a single (season, episode) tuple
// (model.Episodes is map[int]map[int]struct{}), so multi-episode info is
// lost regardless. Taking the first element preserves a useful value — the
// starting episode of the pack — rather than dropping the field. The same
// rule applies uniformly to Year and Season for consistency; an array for
// Year is nonsensical but taking the first is harmless.
type flexInt int

// parseFlexNumber parses a numeric string into flexInt, accepting both
// integer and float representations. Floats truncate toward zero. Returns
// ok=false if s is not a valid number, or if it is NaN/±Inf/out of int range.
func parseFlexNumber(s string) (flexInt, bool) {
	if i, err := strconv.Atoi(s); err == nil {
		return flexInt(i), true
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}

	if math.IsNaN(f) || math.IsInf(f, 0) || f > float64(math.MaxInt) || f < float64(math.MinInt) {
		return 0, false
	}

	return flexInt(int(f)), true
}

func (f *flexInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)

	if bytes.Equal(data, []byte("null")) {
		*f = 0
		return nil
	}

	// Array — recurse on the first element. Empty array -> unset.
	if len(data) > 0 && data[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return fmt.Errorf("flexInt: invalid array: %w", err)
		}

		if len(arr) == 0 {
			*f = 0
			return nil
		}

		return f.UnmarshalJSON(arr[0])
	}

	// String — trim and parse. Empty or non-numeric -> unset (not an error):
	// the model often emits "" when it cannot determine a value, and a
	// descriptive string like "pilot" for an episode is wrong but not
	// malformed JSON. The downstream `> 0` guards filter the resulting zero.
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("flexInt: invalid string: %w", err)
		}

		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}

		v, ok := parseFlexNumber(s)
		if !ok {
			*f = 0
			return nil
		}

		*f = v

		return nil
	}

	// Bare number — int or float. Anything else is genuinely malformed and
	// must error (the JSON decoder normally guarantees this branch only sees
	// number syntax, so reaching it with a non-number means the caller poked
	// raw bytes into flexInt.UnmarshalJSON directly).
	v, ok := parseFlexNumber(string(data))
	if !ok {
		return fmt.Errorf("flexInt: cannot unmarshal %s into int", data)
	}

	*f = v

	return nil
}

// flexStrings is a []string that tolerates the real-world shape variations LLMs
// emit for the Language field: a single JSON string ("eng"), a JSON array of
// strings (["rus","spa"]), JSON null, or an empty array. The model legitimately
// returns either shape — a single string for mono-language releases and an
// array for multi-language packs — so both are first-class.
//
// A non-string element inside an array is skipped rather than erroring: the
// model occasionally emits ["rus", 5, "spa"] and discarding the 5 preserves the
// valid entries. A top-level non-string scalar (e.g. a JSON number or object
// where Language is expected) IS an error — that is genuinely malformed output,
// not the shape-flex this type exists to tolerate. Validation against the
// canonical language list is deferred to the caller (applyLLMResult).
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)

	if bytes.Equal(data, []byte("null")) {
		*f = nil
		return nil
	}

	// Array — keep string elements, skip non-strings and empty strings.
	if len(data) > 0 && data[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return fmt.Errorf("flexStrings: invalid array: %w", err)
		}

		out := make([]string, 0, len(arr))

		for _, el := range arr {
			var s string
			if err := json.Unmarshal(el, &s); err != nil {
				continue
			}

			if s != "" {
				out = append(out, s)
			}
		}

		if len(out) == 0 {
			*f = nil
			return nil
		}

		*f = out

		return nil
	}

	// Single string -> one-element slice. A non-string scalar here is
	// genuinely malformed and must error.
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("flexStrings: cannot unmarshal %s into string: %w", data, err)
	}

	if s == "" {
		*f = nil
		return nil
	}

	*f = []string{s}

	return nil
}

// UnmarshalJSON decodes a ClassifyResult from an LLM response, tolerating the
// type variations documented on flexInt (Year/Season/Episode) and flexStrings
// (Language). All other fields use the standard JSON decoder: a genuinely
// malformed response (broken JSON syntax, wrong type for a non-numeric field,
// an object where a scalar is expected) still returns an error.
//
// The alias type strips this method so we can defer to the default decoder
// for the non-overridden fields, while the outer struct shadows Year/Season/
// Episode/Language with flex-typed fields at a shallower depth — Go's JSON
// decoder selects the shallower field, so only the flex unmarshalers run for
// them.
func (r *ClassifyResult) UnmarshalJSON(data []byte) error {
	type alias ClassifyResult

	aux := struct {
		alias
		Year     flexInt     `json:"year"`
		Season   flexInt     `json:"season"`
		Episode  flexInt     `json:"episode"`
		Language flexStrings `json:"language"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*r = ClassifyResult(aux.alias)
	r.Year = int(aux.Year)
	r.Season = int(aux.Season)
	r.Episode = int(aux.Episode)
	r.Language = []string(aux.Language)

	return nil
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
