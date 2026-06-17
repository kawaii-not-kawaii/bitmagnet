---
baseline_commit: HEAD
status: ready-for-dev
story_key: 1-2-openai-client
epic: 1
story: 2
---

# Story 1.2: Implement OpenAI-Compatible HTTP Client

## Story

As a user,
I want an HTTP client that works with any OpenAI-compatible endpoint,
So that I can use llama.cpp, OpenAI, vLLM, Ollama, or any compatible backend.

## Acceptance Criteria

## Tasks/Subtasks

- [x] 1. Create `internal/llm/openai/` package
- [x] 2. Define `Config` struct (base_url, model, api_key, timeout, max_retries)
- [x] 3. Implement `Provider` with `Classify()` and `Name()`
- [x] 4. Implement prompt building with template + file truncation
- [x] 5. Implement response parsing and enum validation
- [x] 6. Write unit tests with httptest.Server
- [x] 7. File list limit (max 20 files per prompt to stay within context)

## Dev Agent Record

### Completion Notes

Created `internal/llm/openai/client.go` with:
- `Config` struct: BaseURL, Model, APIKey, Timeout, SystemPrompt, UserPrompt
- `New(Config)` constructor returns `llm.Provider`
- `Classify()` builds chat request with system + user messages, sends POST with retry
- Default system prompt with content type list, JSON response instruction
- File list truncated to 20 entries maximum
- Exponential backoff retry on server errors (100ms, 200ms, 400ms), no retry on 4xx
- `response_format: json_object` for structured output
- Bearer token auth via APIKey
- Created `internal/llm/openai/client_test.go` with 15 tests: success, empty content, invalid JSON, HTTP error, server retry, timeout, auth header, file inclusion, file limit, system prompt default/custom, config timeout
- All 15 tests pass, `go vet` clean

## File List

- `internal/llm/openai/client.go` (new)
- `internal/llm/openai/client_test.go` (new)

## Change Log

- Story 1.2: OpenAI-compatible HTTP client implemented.
