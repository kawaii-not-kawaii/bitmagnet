# Investigation: Live Measurements, Code Map, Rejected-Batcher Rationale

Load-bearing reference for SPEC-llm-classifier-throughput. Measured on the live
instance `root@tower` on 2026-06-17.

## Inference server (AMD Lemonade / llama.cpp @ 100.125.213.44:8082)

- Model: `gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf` — MoE, ~4B active of 25.2B params, Q4_K_XL, 14.2 GB, multimodal.
- `n_ctx_train` = 262,144 (the "264k" the operator recalled — that is the *training* context, not the served budget).
- Served context: `total_slots` = 4, per-slot `n_ctx` = 65,536 → ~256k total KV split into 4 × 64k slots.
- Speculative decoding ON (MTP draft model; draft accepted 3/3 in test).
- Prompt-prefix caching active (`timings.cache_n` observed) — a stable system-prompt prefix is amortized across sequential same-slot requests.
- `endpoint_metrics` = false (llama.cpp Prometheus endpoint not enabled on the server).
- Measured once (cold small request): prompt-eval ~150 tok/s; generation ~22 tok/s.

## Client config (`/config/config.yml` in `binhex-bitmagnet`)

```
classifier.llm:
  provider_name: gemma4
  provider_base_url: http://100.125.213.44:8082
  provider_model: gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf
  batch_size: 1        # batching effectively OFF live
  max_context: 16000   # caps each request; leaves ~48k/slot unused
  max_tokens: 256
  timeout: 30s
  interval: 5s
```
- Env: `CLASSIFIER_CONCURRENCY=8`.

## Workload (Postgres `bitmagnet.queue_jobs`, `torrents`)

- Arrival: steady ~3,400 torrents/hr (~1/sec) from the DHT crawler, flat across 24h. NOT bursty-deep.
- Backlog: ~2 pending at sample time; classifier keeps up at steady state.
- Job latency `created_at → ran_at` (processed, last 2h): avg 136.8s, max 285.7s. With near-zero instantaneous backlog this indicates ρ ≈ 1 — the queue builds into multi-minute waves and drains.
- Failures observed (separate from this spec): `process_torrent_batch` 4× "duplicate key value violates unique constraint" (batch write-path dedup bug); `process_torrent` repeated "invalid tag name" (e.g. `Japanese`, `final-fantasy-vii-…` — LLM output violates tag-format rules); "missing N info hashes" (data/availability).

## Capacity model

`capacity = slots / (prompt_eval_s + gen_s)` per item.
- `gen_s = output_tokens / 22` — generation dominates and is the slow stage.
- `prompt_eval_s = novel_prompt_tokens / 150` — with prefix caching, only name+files are novel; the system preamble is reused.
- Implication: cutting output tokens (CAP-2) is the highest-leverage lever; bigger prompts/batches are counterproductive.

## Code map (bitmagnet repo)

- `internal/llm/registry.go` — `Config`/`ProviderConfig`: `BatchSize`, `MaxContext` (yaml `max_context_tokens`), `MaxTokens`, `Interval`, `Timeout`.
- `internal/classifier/classifierllm/module.go` — wires config → client + BatchClient; `batchSize`, `flushAfter = Interval`.
- `internal/llm/openai/client.go`:
  - `buildRequest` (single): `Temperature: 0.1`, `ResponseFormat: json_object`, `MaxTokens: estimateMaxTokens(input)`.
  - `estimateMaxTokens` — **hardcodes 256 and ignores `config.MaxTokens`** (relevant to CAP-2).
  - `BatchClassify` (batch path): `MaxTokens: 256 * len(inputs)`, no `response_format`.
  - `buildSystemMessage` / `defaultSystemPromptFmt` — system preamble; stable except `{{.ContentTypes}}` interpolation (CAP-3 prefix-stability hinges on this).
  - `buildUserMessage` — **file list hardcoded-truncated at 20** with "... and N more files".
  - `MaxContext` is plumbed into config but not referenced in `buildRequest` (see Open Question on whether it gates anything today).
- `internal/llm/metrics.go` — existing Prometheus metrics: `llm_classify_duration_seconds` (histogram, buckets → 21s), `llm_classify_tokens_total`, `llm_classify_errors_total`, `llm_classify_batch_size`. Foundation for CAP-1 instrumentation.

## Why the adaptive batcher was rejected (first-principles)

1. Two "batchings" were conflated: CLIENT prompt-batching (N torrents per prompt) vs. SERVER continuous-batching (llama.cpp runs up to 4 requests in parallel across slots, driven by concurrency).
2. The bottleneck is generation (~22 tok/s), not request count. Prompt-batching does not reduce total output tokens, so it does not speed a generation-bound workload.
3. Prompt-batching serializes N generations into ONE slot; N separate requests spread across 4 slots generate in PARALLEL. Large client batches therefore *reduce* throughput by starving slots.
4. Prefix caching already amortizes the system-prompt preamble across sequential calls, shrinking the request-overhead saving batching was meant to deliver.
5. Per-request hard ceiling is ~64k/slot, but the client caps at 16k — so the constraint was never the model context the original idea targeted.
