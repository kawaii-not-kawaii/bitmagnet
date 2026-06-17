---
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/prd.md
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/addendum.md
  - _bmad-output/planning-artifacts/prds/prd-bitmagnet-2026-06-15/.decision-log.md
workflowType: 'architecture'
project_name: 'bitmagnet'
user_name: 'Yun'
date: '2026-06-15'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**
- **FR-1 to FR-7** as defined in the PRD cover the full feature surface: LLM classier action, provider abstraction, OpenAI-compatible client, TMDB fallback, batching/concurrency/context budget, queue integration, prompt templates
- **Architectural structure:** 4 packages (internal/llm, internal/llm/openai, internal/classifier/action_llm_classify, processor/llm handler wiring) — not 5-7
- **Batching lifted to handler:** Per Winston's assessment, batching belongs at the queue handler level, not in the classifier action. Actions remain single-torrent by design. The handler loads N unmatched jobs, sends as a batch, fans results back to individual persist calls.
- **Provider config refactored:** Providers defined globally in `classifier.llm.providers`, referenced by name in workflow actions. Avoids inline re-declaration.
- **No new worker lifecycle:** The LLM handler piggybacks on the existing `queue_server` worker via the handler registry — no new worker registration needed.

**Non-Functional Requirements:**
- NFR-1 (zero external data): Config validation asserting base_url resolves to private IP/localhost
- NFR-2 (non-blocking): Separate queue handler prevents main pipeline backpressure
- NFR-3 (latency budget): Mock clock for batch timer; Prometheus histogram for LLM duration
- NFR-4 (graceful degradation): Warn-level logging, not error; torrent stays queryable as unmatched

**Scale & Complexity:**
- Project complexity: medium
- Primary domain: backend (Go)
- Estimated components: 4 new packages + 1 config extension + 1 queue handler registration
- Lines of code: ~600-700 across 5-6 files

### Technical Constraints & Dependencies

- LLM must run on local hardware (Gemma 4 26B QAT on miniscoffee:8082)
- OpenAI-compatible HTTP client matching llama.cpp's /v1/chat/completions
- `response_format: { type: "json_object" }` required for structured output (Murat R1 — make mandatory, not optional)
- Queue dedup (fingerprint) prevents redundant work — no cache layer
- Token estimation for context budget must be accurate (Winston risk: token estimation is a foot-gun)
- No Go toolchain on the development machine — must compile elsewhere or set up Go

### Cross-Cutting Concerns Identified

1. **LLM Output Validation (Murat R1 — 9/10 risk):** The response validator is the critical gate between non-deterministic LLM output and the typed domain model. Needs 20+ table-driven tests, property-based fuzzing, and full TestClassifier integration.
2. **Queue Math / Backpressure (John):** At 100 tok/s with 4 workers, the LLM classify rate may be slower than the crawl ingest rate. Unbounded backlog needs acknowledgment — either rate-limit enqueue or accept the gap as a known operating constraint.
3. **File List Truncation (Murat R2 — 7/10):** Truncation algorithm invisibly drops signal. Must be tested with real torrent structures.
4. **Batch Partial Failure (Murat R3 — 6/10):** Per-item acceptance — failed items must not block successful ones from persisting.
5. **No re-classify on model change:** Queue dedup prevents automatic re-processing when the model or prompt changes. Documented manual reprocess path needed (deferred to v0.2).
6. **Config schema extension:** `classifier.llm.providers` and per-action provider references need JSON schema coverage.
7. **Accuracy measurement:** No feedback loop to measure the 90% correctness target. Acknowledge as hobby-project gap.

### Party Mode Resolution

The following architectural corrections from the roundtable discussion are incorporated into the requirements above:

| Source | Finding | Action |
|--------|---------|--------|
| Winston | Batching at handler, not action | Queue handler owns batch orchestration; actions stay single-torrent |
| Winston | Provider config: global + name-ref | `classifier.llm.providers[]` defined once, referenced by name in workflows |
| Winston | Packages: 4, not 5-7 | llm/, llm/openai/, action_llm_classify.go, handler wiring |
| John | Queue backpressure math | Acknowledge as known constraint; consider rate-limiting enqueue for v0.2 |
| John | Accuracy measurement gap | No feedback loop — document as hobby-project limitation |
| Amelia | File list + implementation map | 5-6 files, ~650 lines, no schema changes |
| Murat | R1: JSON response_format mandatory | Raise from optional to required in provider config |
| Murat | R3: Batch partial failure — per-item | Failed items logged independently, successful ones persisted |
| Murat | R6: No re-classify on model change | Defer to v0.2 with documented manual reprocess path |

## Core Architectural Decisions

All decisions are documented with their rationale. Versions reference the project's existing Go 1.23 / Gin / gqlgen / GORM stack — no new external dependencies introduced.

### Decision Priority Analysis

**Critical Decisions (Block Implementation):**
- Provider abstraction interface (LLMProvider)
- Queue handler integration point
- Prompt template model (per-provider)
- Config schema for classifier.llm.*

**Important Decisions (Shape Architecture):**
- Batching at handler level vs action level
- Context budget enforcement
- File list truncation strategy
- Fallback chain design

**Deferred Decisions (Post-MVP):**
- Multi-provider fallback chain (first provider wins for now)
- E4B / multi-model support (test model didn't work on current llama.cpp build)
- Provider health checking
- Active learning loop

### API & Integration Architecture

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Provider interface | `LLMProvider` with `Classify(ctx, input) -> result` | OpenAI-compatible HTTP client is the default. Users can implement against any backend by providing `base_url` + `model` + `api_key`. Agnostic by design. |
| Config model | Providers defined globally under `classifier.llm.providers`, referenced by name in workflow actions | Keeps workflow YAML clean; providers are infrastructure, not workflow logic |
| Batch strategy | At queue handler level, not classifier action | Action stays single-torrent and composable. Handler accumulates N jobs, sends batch, fans results back. |
| Prompt templates | Per-provider in config, optional override in action | Different models (Gemma 4, LFM, GPT-4o-mini) need different system prompts and context formatting |
| Template engine | Go `text/template` | Stdlib, zero dependencies, already used in the project |

### Concurrency & Resource Model

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Worker model | Background queue handler, separate from inline classifier | LLM latency (1-2s) must not block the fast YAML+CEL pipeline |
| Worker count | `worker_count` config param, user matches to their LLM backend's capacity | Provider-agnostic — a local 26B needs different concurrency than an OpenAI API key |
| Context budget | `max_context_tokens` config, file lists truncated to fit | Prevents exceeding any provider's context window regardless of model |
| Queue dedup | Via existing job fingerprint mechanism | No additional cache infrastructure. Same mechanism as normal classify queue. |
| Retry | Reuse existing queue retry backoff (Sidekiq-style) | Consistent with rest of system. No special-casing for LLM failures. |

### Observability

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Classification duration | `llm_classify_duration_seconds` histogram with `provider` + `status` labels | Tracks per-provider latency and error rates |
| Content type distribution | Log line on successful classification | Lightweight, no new metric needed. Can be aggregated from logs. |
| Errors | Logged at warn level, not error | Graceful degradation by design — LLM failure is expected during config/testing |

### Fallback Chain

First configured provider only for v0.1. If it fails → `ErrUnmatched` → logging at warn level. Multi-provider fallback (try A, if unreachable try B) deferred to v0.2.

This keeps initial implementation simple: one HTTP call, no fallback orchestration, no timeout cascading.

### Real-World Benchmark Reference

Tested against the project's own llama.cpp instance (Gemma 4 26B QAT+MTP on local network):

| Config | Throughput | 5.38M backlog |
|--------|:---------:|:-------------:|
| Sequential | 0.6/sec | 112 days |
| 4 parallel, single requests | 2.05/sec | **30 days** |
| Inflow rate (est. new unknowns) | ~0.3/sec | Trivial after catch-up |

Accuracy on 30 random unknowns: **~95% valid classification** (5/5 correct on manual audit).
