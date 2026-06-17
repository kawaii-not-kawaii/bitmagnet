---
title: LLM Classifier Action for bitmagnet
status: final
created: 2026-06-15
updated: 2026-06-15
version: 1.0
---

# LLM Classifier Action for bitmagnet

## Vision

Eliminate the "unknown" classification dead-end in bitmagnet's torrent pipeline by adding a local LLM fallback that can match even poorly-named or obscure torrents that the existing YAML+CEL classifier cannot resolve. The result is a dramatically higher match rate with zero additional API cost and no data leaving the local network.

## Problem Statement

bitmagnet's classifier is YAML + CEL expression-based. It works well for standard scene releases (well-structured names with known groups, years, resolution markers) but produces unmatched results for:

- Obscure or indie content with unconventional naming
- Non-English releases with unusual formatting
- Ambiguous names that could match multiple content types
- Content where the parser extracts conflicting attributes

When the classifier returns `ErrUnmatched`, the torrent sits in the database with no `content_type`, hidden from search results and unusable by Servarr integrations. Currently there is no automated recovery path — only manual reprocess with ad-hoc rule additions.

## Success Metrics

- Reduce unmatched rate by at least 50% (measured as ratio of torrents with null `content_type` vs total processed)
- Zero additional operational cost (local LLM only)
- No user-visible latency increase in normal pipeline (LLM runs asynchronously when needed)
- Correctness rate of at least 90% on content_type attribution vs human judgment for a sampled test set

## User Journeys

### UJ-1: Unknown torrent gets classified

A torrent named `Some.Obscure.Film.2024.1080p.WEBRip.x265` enters the pipeline. The normal classifier fails to match — no `content_type` set, no `find_match` result. The LLM action fires: sends the torrent name (and optionally file list) to the local LLM. The LLM returns `{"content_type": "movie", "title": "Some Obscure Film", "year": 2024, ...}`. The system then searches TMDB with the resolved title + year, attaches the content record. The torrent appears in search results.

### UJ-2: LLM identifies content, TMDB misses

A torrent of an ultra-obscure direct-to-video release names correctly but TMDB has no record. The LLM correctly classifies it as `movie` with the right title and year. The `attach_tmdb_content_by_search` step finds nothing. The torrent is still searchable by title, content_type, year — just without the full content enrichment.

### UJ-3: Background LLM worker continuously processes unknowns

A dedicated background worker registers alongside the normal `classify` handler. As torrents flow through the DHT crawler -> persist -> queue pipeline, the normal classifier processes them. Those that land on `unknown` (`ErrUnmatched`) are automatically re-queued to the `llm_classify` handler. The LLM worker consumes at its own pace — batch-calling the model, running TMDB fallback, persisting results — while the normal pipeline keeps flowing. No user action needed; unknowns get absorbed into search results on the LLM worker's next cycle.

## Functional Requirements

### FR-1: LLM Classifier Action

- New classifier action type `llm_classify` available in `classifier.core.yml` workflows
- Accepts a list of LLM provider configs (name, model, base_url, api_key)
- Sends torrent name + file list as context to LLM
- Receives structured classification response (content_type, title, year, language, video attributes)
- Validates LLM response against existing enum types
- Sets classification attributes on the result if valid
- If LLM returns no valid content_type, returns `ErrUnmatched` as normal

### FR-2: Provider Abstraction

- Interface `LLMProvider` with a single `Classify` method
- Default implementation: OpenAI-compatible HTTP client
- Configurable per provider: `base_url`, `model`, `api_key`, `timeout`, `max_retries`
- Provider selection via YAML config: which provider(s) to try and in what order
- At least one provider must succeed for the action to produce a result

### FR-3: OpenAI-Compatible Client

- Implements `LLMProvider` interface
- Compatible with llama.cpp's `/v1/chat/completions` endpoint (OpenAI-compatible mode)
- Supports any provider advertising OpenAI API compatibility (vLLM, Ollama, etc.)
- Configurable prompt template with system prompt + user message
- JSON mode for structured output

### FR-4: TMDB Fallback

- After LLM classifies content, runs `attach_tmdb_content_by_search` with the LLM-resolved title + year
- If TMDB returns content, attaches it to the result
- If TMDB fails, keeps the LLM classification attributes without content enrichment

### FR-5: Batching, Concurrency, and Context Budget

- LLM action batches N torrent names in a single LLM call (configurable `batch_size`, default 5)
- Multiple concurrent LLM workers (goroutines) consume from the `llm_classify` queue — `worker_count` config controls parallelism (default 4)
- llama.cpp serves with `--parallel N` slots, matching the worker count
- Total model context (256K) is partitioned across slots via `--parallel` — each slot gets ~64K when N=4
- Per-request `max_context_tokens` cap prevents exceeding the per-slot budget (default 16K, well within 64K slot)
- File lists truncated per-torrent to fit within the context budget — most informative files kept first (largest non-sample video, then audio, then data)
- Per-request `max_tokens` cap limits output (256 — classification results are ~50-100 tok)
- Each LLM call is stateless — no conversation accumulation between requests, every call is a fresh prompt
- llama.cpp's continuous batching (`--cont-batching`, default on) handles dynamic slot allocation automatically

### FR-6: Queue Integration

- New queue handler `llm_classify` registered alongside the existing `classify` handler
- Handler receives torrents that the normal classifier action left unmatched
- Pipeline: load unmatched torrents -> batch -> call LLM -> TMDB fallback -> persist
- Queue dedup via existing job fingerprint mechanism prevents redundant classification
- Multiple concurrent worker goroutines consume from the same queue (configurable `worker_count`)
- No additional cache layer needed — the job queue handles dedup and retry natively

### FR-7: Prompt Template

- Configurable YAML template for the LLM prompt
- Default template covers: system prompt with role + examples, available content types, user message with torrent name + files
- Template variables: `{{.Name}}`, `{{.Files}}`, `{{.ContentTypes}}`
- Supports custom templates per workflow or globally
## Non-Functional Requirements

### NFR-1: Zero External Data Transfer

All LLM inference runs on local hardware (Gemma 4 26B via llama.cpp). No data leaves the local network under any configuration. API keys are only for local endpoints.

### NFR-2: Non-Blocking by Default

The LLM action should not block the main classifier pipeline. If the action is configured but the LLM is unreachable, the classifier falls through to unmatched without error cascading.

### NFR-3: Latency Budget

LLM classification per torrent should complete within 10 seconds for a single inference. Batching N items should not exceed 30 seconds. The 26B Gemma 4 QAT at ~100 tok/s on the target hardware meets this comfortably.

### NFR-4: Graceful Degradation

If all configured providers fail (connection error, timeout, invalid response), the action returns `ErrUnmatched` — no data loss, no pipeline crash. Errors are logged at warning level, not error.

## Configuration Schema (Proposed)

```yaml
classifier:
  llm:
    providers:
      - name: gemma4
        model: gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf
        base_url: http://100.125.213.44:8082/v1
    prompt_template: |
      System: You are a torrent classifier...
    batch_size: 5
    max_context_tokens: 16000   # per-request prompt limit
    max_tokens: 256             # per-request output limit
    worker_count: 4             # concurrent workers, match llama.cpp --parallel N
    interval: 5s                # LLM worker poll interval
    timeout: 30s                # per-request timeout
```

## Open Questions — Resolved

All open questions from brainstorming were resolved during review:
1. **Content type restrictions:** LLM classifies everything — no blacklist. That's the point of the feature.
2. **Caching:** Not needed — the LLM worker is a persistent background processor. The job queue's existing fingerprint dedup prevents redundant work. No pre-warm needed.
3. **File lists:** Yes — name + full file list sent to LLM for maximum signal.

1. Should the LLM action have its own flag/limit on which content types to accept (e.g. never classify as `xxx`)?
2. Should cache be pre-warmable via a CLI command for bulk existing unknowns?
3. Should the LLM receive file lists or just torrent names? File lists give more signal but cost tokens.

## Out of Scope (v0.1)

- WASM plugin integration (deferred to broader plugin system)
- Multi-model ensembling / confidence scoring
- Active learning loop (feeding user corrections back to fine-tune)
- LLM-powered tag suggestions (separate feature)
