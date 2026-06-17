---
baseline_commit: HEAD
status: ready-for-dev
story_key: 1-1-llm-provider-interface
epic: 1
story: 1
---

# Story 1.1: Define LLMProvider Interface and Types

## Story

As a developer,
I want a provider interface with classification types,
So that the system can work with any LLM backend through a common contract.

## Acceptance Criteria

**AC1:** `LLMProvider` interface is defined in `internal/llm/` with `Classify(ctx, ClassifyInput) (*ClassifyResult, error)` — provider-agnostic with no backend-specific dependencies.

**AC2:** `ClassifyInput` contains `Name string` and `Files []string`. When files are empty the provider receives only the torrent name.

## Tasks/Subtasks

- [x] 1. Create `internal/llm/` package with `provider.go`
- [x] 2. Define `ClassifyInput` struct
- [x] 3. Define `ClassifyResult` struct with domain-aligned types
- [x] 4. Define `Provider` interface
- [x] 5. Define optional `BatchProvider` interface

## Dev Agent Record

### Completion Notes

Created `internal/llm/provider.go` with the full provider abstraction:
- `ClassifyInput` struct with Name, Files (omitempty), ContentTypes fields. JSON-tagged for serialization.
- `ClassifyResult` struct with all classification fields (ContentType, Title, Year, Season, Episode, Language, VideoResolution, VideoSource, VideoCodec, ReleaseGroup, Tags). All optional fields use `omitempty`.
- `Provider` interface with `Name()` and `Classify(ctx, ClassifyInput) -> *ClassifyResult, error`.
- `BatchProvider` interface extending `Provider` with `BatchClassify(ctx, []ClassifyInput) -> []*ClassifyResult, error`.
- Error sentinels: `ErrNoResult`, `ErrInvalidJSON`.
- Created `internal/llm/provider_test.go` with unit tests for: value semantics, empty files, provider interface contract, error handling, batch interface, batch fallback, JSON round-trip, zero values in JSON, provider name, error sentinels.
- Pure stdlib, zero external dependencies, no model package imports (provider-agnostic by design).
- `go vet` passes cleanly.

## File List

- `internal/llm/provider.go` (new)
- `internal/llm/provider_test.go` (new)

## Change Log

- Story 1.1: LLM provider interface and types implemented.
