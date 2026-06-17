---
stepsCompleted: [1]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/prd.md
  - _bmad-output/planning-artifacts/architecture.md
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/addendum.md
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/.decision-log.md
---

# bitmagnet - LLM Classifier Action - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for the LLM Classifier Action feature, decomposing the requirements from the PRD and Architecture decisions into implementable stories.

## Requirements Inventory

### Functional Requirements

FR1: LLM Classifier Action — New action type `llm_classify` that sends torrent name + file list to a configured LLM provider and receives structured classification output (content_type, title, year, attributes).

FR2: Provider Abstraction — Interface `LLMProvider` with `Classify(ctx, input) -> result`. Users configure providers globally under `classifier.llm.providers` with `base_url` + `model` + `api_key`. Provider-agnostic — supports any OpenAI-compatible endpoint.

FR3: OpenAI-Compatible HTTP Client — Default provider implementation calling `/v1/chat/completions` with `response_format: json_object` for structured output. Works with llama.cpp, vLLM, Ollama, OpenAI, etc.

FR4: TMDB Fallback — After LLM classification, attempt TMDB content enrichment using the LLM-resolved title + year. If TMDB succeeds, attach the full content record. If TMDB fails, keep the LLM classification attributes.

FR5: Batching, Concurrency, and Context Budget — LLM batches N torrents per request (configurable `batch_size`). Multiple concurrent workers (`worker_count`). `max_context_tokens` cap with file list truncation prevents exceeding model context windows. `max_tokens` caps output. Calls are stateless.

FR6: Queue Integration — New `llm_classify` queue handler registered alongside existing `classify` handler. Unmatched torrents re-queued automatically. Fingerprint dedup prevents redundant work. Queue retry backoff handles failures.

FR7: Prompt Templates — Configurable per-provider template using Go `text/template`. Variables: `{{.Name}}`, `{{.Files}}`, `{{.ContentTypes}}`. Optional per-action override.

### Non-Functional Requirements

NFR1: Zero External Data Transfer — All LLM inference must run on local hardware or user-configured endpoints. No data leaves the user's control network unless explicitly configured.

NFR2: Non-Blocking by Default — LLM classification runs in a separate background queue handler. Main classifier pipeline is never blocked by LLM latency.

NFR3: Latency Budget — Single inference < 10s. Batched N items < 30s. Real-world benchmark: 2.05 torrents/sec sustained with 4 workers on Gemma 4 26B.

NFR4: Graceful Degradation — LLM failure logs at warn level, returns ErrUnmatched, no pipeline crash. TMDB failure keeps LLM attributes. No data loss.

### Additional Requirements (Architecture)

- 4 new packages: `internal/llm/` (interface + types), `internal/llm/openai/` (HTTP client), `internal/classifier/action_llm_classify.go` (action), `processor/llm` (handler wiring)
- No schema changes, no new tables, no model changes
- Batching at handler level, not action level (per Winston, party mode)
- Config: providers defined globally under `classifier.llm`, referenced by name in workflows
- `response_format: json_object` mandatory in provider config (per Murat R1)
- Per-item batch partial failure — failed items don't block successful ones
- Metrics: `llm_classify_duration_seconds` histogram with `provider` + `status` labels
- Queue dedup prevents re-classify on model/prompt change (deferred manual reprocess path to v0.2)

### UX Design Requirements

None — this feature has no user interface changes. Classification runs in the background.

### FR Coverage Map

_(To be filled after epic/story creation)_

## Epic List

_(To be filled)_

## Epic 1: LLM Provider Core

Goal: Implement the LLM provider abstraction, OpenAI-compatible HTTP client, and configuration wiring so the system can communicate with any OpenAI-compatible LLM endpoint.

### Story 1.1: Define LLMProvider Interface and Types

As a developer,
I want a provider interface with classification types,
So that the system can work with any LLM backend through a common contract.

**Acceptance Criteria:**

**Given** a new `internal/llm/` package
**When** the `LLMProvider` interface is defined with `Classify(ctx, ClassifyInput) (*ClassifyResult, error)` 
**Then** the interface is provider-agnostic with no dependencies on specific backends

**Given** the `ClassifyInput` struct contains `Name string` and `Files []string`
**When** files are empty
**Then** the provider receives only the torrent name

**Given** `ClassifyResult` contains `ContentType`, `Title`, `Year`, `Season`, `Episode`, language/video attributes, and `Tags`
**When** the response is validated
**Then** all enum fields are checked against the existing domain enums (ContentType, VideoResolution, etc.)

### Story 1.2: Implement OpenAI-Compatible HTTP Client

As a user,
I want an HTTP client that works with any OpenAI-compatible endpoint,
So that I can use llama.cpp, OpenAI, vLLM, Ollama, or any compatible backend.

**Acceptance Criteria:**

**Given** a provider config with `base_url`, `model`, and optional `api_key`
**When** `Classify` is called
**Then** the client sends a POST to `{base_url}/v1/chat/completions` with the configured model

**Given** the system prompt and user message are built from the configured prompt template
**When** the request is sent
**Then** `response_format: { type: "json_object" }` is included in the request body

**Given** `max_context_tokens` is configured
**When** the constructed prompt exceeds the limit
**Then** file lists are truncated proportionally to fit within the budget

**Given** the endpoint returns a valid JSON response
**When** `Classify` receives the response
**Then** it parses and validates the result against expected enum types

**Given** the endpoint is unreachable or returns an error
**When** `Classify` fails
**Then** it returns a wrapped error that the caller can check

### Story 1.3: Wire Provider Config into Classifier YAML

As a user,
I want to configure LLM providers in the classifier YAML config,
So that I can define my LLM endpoint without code changes.

**Acceptance Criteria:**

**Given** the classifier YAML config
**When** the user adds `classifier.llm.providers` section
**Then** each provider has `name`, `base_url`, `model`, and optional `api_key`

**Given** a provider config with per-provider prompt template
**When** the classifier compiles actions
**Then** the template is parsed with Go `text/template` and available for use

**Given** priority/ordering of providers
**When** the action references provider by name
**Then** the named provider is looked up from the global config

## Epic 2: LLM Classifier Action & Queue Worker

Goal: Wire the LLM provider into the existing classifier action framework and create the background queue handler that processes unmatched torrents.

### Story 2.1: Create llm_classify Classifier Action

As a developer,
I want a new classifier action that calls the LLM for unmatched torrents,
So that unknown torrents get a second chance at classification.

**Acceptance Criteria:**

**Given** the existing classifier action framework
**When** `llm_classify` action is defined with `name()`, `compileAction()`, and `run()` methods
**Then** it follows the same pattern as `attach_tmdb_content_by_search`

**Given** the action is registered in `features.go` via `actions(llmClassifyAction{})`
**When** the compiler processes a workflow referencing `llm_classify`
**Then** it compiles successfully and the action is available

**Given** a torrent with no `content_type` set
**When** the action runs
**Then** it constructs the prompt from torrent name + files, calls the configured provider, and sets classification attributes on success

**Given** the provider returns an error
**When** the action completes
**Then** it returns `ErrUnmatched` with a warn-level log

### Story 2.2: Create LLM Queue Handler

As a user,
I want a background worker that continuously processes unmatched torrents,
So that unknowns get classified automatically over time.

**Acceptance Criteria:**

**Given** the existing queue handler framework
**When** a new `llm_classify` handler is registered
**Then** it follows the same pattern as the existing `classify` handler

**Given** the handler configuration with `batch_size`, `worker_count`, and `interval`
**When** the handler polls for work
**Then** it loads up to `batch_size` unmatched torrents per worker goroutine

**Given** a successful LLM classification
**When** the handler receives the result
**Then** it runs TMDB fallback with the LLM-resolved title + year and persists the combined result

**Given** a partially failed batch (some items fail validation)
**When** the handler processes the batch
**Then** successful items are persisted independently, failed items are logged and re-queued

**Given** the handler's `interval` elapses
**When** no work is available
**Then** the handler idles until the next poll cycle

### Story 2.3: Wire Handler into Application Composition

As a developer,
I want the LLM worker wired into the application's dependency injection,
So that it starts and stops with the rest of the system.

**Acceptance Criteria:**

**Given** the fx dependency injection framework
**When** a new `llmfx` module is created
**Then** it provides the LLM provider and handler as fx dependencies

**Given** the handler is registered via the worker registry
**When** the application starts with `--all` or appropriate keys
**Then** the LLM worker starts and begins polling

**Given** the application is shutting down
**When** the fx lifecycle stops
**Then** the LLM worker stops gracefully with in-flight requests completing

## Epic 3: Prompt Engineering & Observability

Goal: Create the default prompt template, file list truncation strategy, and monitoring metrics.

### Story 3.1: Default Prompt Template

As a user,
I want a sensible default prompt template for classification,
So that the feature works out of the box without custom prompt engineering.

**Acceptance Criteria:**

**Given** the default prompt template
**When** constructed
**Then** it includes: system role definition, available content types list, classification rules, and few-shot examples

**Given** the template uses `{{.Name}}` and `{{.Files}}` variables
**When** the torrent has no files
**Then** the files section is omitted from the prompt

**Given** the user provides a custom template in their config
**When** the action runs
**Then** the custom template is used instead of the default

### Story 3.2: File List Truncation Strategy

As a developer,
I want file lists truncated intelligently to fit within the context budget,
So that the most informative files are kept when the prompt would exceed `max_context_tokens`.

**Acceptance Criteria:**

**Given** the `max_context_tokens` budget
**When** file lists exceed the budget
**Then** files are kept in priority order: largest video file first, then audio, then data files; sample/thumbnails dropped first

**Given** a torrent with many small files
**When** the budget is tight
**Then** at minimum the filename and extension are preserved for each kept file

### Story 3.3: Observability Metrics

As an operator,
I want visibility into LLM classification performance,
So that I can monitor throughput, latency, and error rates.

**Acceptance Criteria:**

**Given** the system has Prometheus metrics
**When** a classification completes
**Then** `llm_classify_duration_seconds` histogram is recorded with `provider` and `status` (success/error) labels

**Given** a successful classification
**When** the result is persisted
**Then** a structured log entry records the content_type assigned, provider used, and token counts

**Given** an LLM classification fails
**When** the error is logged
**Then** it includes the provider name, error type, and elapsed time at warn level

### Story 3.4: Benchmark Script (Dev Utility)

As a developer,
I want a CLI tool to test LLM classification on sample torrents,
So that I can evaluate different providers and prompt templates before deploying.

**Acceptance Criteria:**

**Given** a CLI command `go run . reprocess --llm-classify-test --count 20`
**When** executed
**Then** it loads N random unknowns from the database, runs them through the configured LLM provider, and reports throughput, accuracy sampling, and token usage

## Epic 4: Integration & Deployment

Goal: Configure the Gemma 4 LLM server, create docker-compose integration, and document the full setup.

### Story 4.1: Systemd Service Update

As an operator,
I want the Gemma 4 26B service configured with parallel slots,
So that the LLM worker can achieve optimal throughput.

**Acceptance Criteria:**

**Given** the existing `gemma-26b-mtp.service`
**When** the service starts
**Then** it runs with `-np 4` for 4 parallel slots, each with 64K context

**Given** the benchmark configuration
**When** tested with 4 workers x batch of 1
**Then** throughput reaches at least 2.0 torrents/sec

## Epics Summary

| Epic | Stories | Priority | Dependencies |
|------|---------|----------|-------------|
| 1: Provider Core | 1.1, 1.2, 1.3 | 1 | None |
| 2: Classifier Action & Worker | 2.1, 2.2, 2.3 | 2 | Epic 1 |
| 3: Prompt Engineering & Observability | 3.1, 3.2, 3.3, 3.4 | 3 | Epic 2 |
| 4: Integration & Deployment | 4.1 | 4 | None (parallel track) |
