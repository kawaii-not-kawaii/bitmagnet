---
stepsCompleted: []
inputDocuments: []
session_topic: ''
session_goals: ''
selected_approach: ''
techniques_used: []
ideas_generated: []
context_file: ''
---

# Brainstorming Session Results

**Facilitator:** {{user_name}}
**Date:** {{date}}

---
session_topic: 'Adaptive/dynamic LLM classifier batch sizing'
session_goals: 'Design dynamic batch sizing from backlog depth + tuned token budget; token estimation per torrent; batch-size derivation; backlog-awareness; guardrails; trade-offs'
stepsCompleted: [1]

## Session Overview

**Topic:** Adaptive/dynamic LLM classifier batch sizing (replace fixed 5-torrents/request with batch size derived from backlog + tuned token budget)

**Goals:**
- Estimate tokens per torrent cheaply before sending
- Derive batch size from token budget + backlog depth
- Backlog-awareness (grow when deep, shrink when shallow)
- Guardrails (token ceiling, accuracy degradation, latency, failure blast-radius)
- Trade-offs (throughput vs accuracy vs cost vs recoverability)

### Session Setup

Brownfield: bitmagnet LLM classifier. Current state — fixed batch of 5 torrents/request via BatchClient (internal/llm/openai/batch.go). Model: Gemma 3 (~128k ctx max; target tuned budget ~32k input tokens).


---
techniques_used: ['First Principles Thinking']
stepsCompleted: [1, 2]

## Technique Selection

**Approach:** User-Selected (lean) — First Principles, then Pre-Mortem
**Rationale:** Concrete technical design; user wanted to skip ceremony. Bottleneck targeted: (a) wall-clock throughput + (b) request overhead/cost.

## Live System Findings (root@tower, 2026-06-17)

**Arrival / backlog:**
- Torrent arrival: steady ~3,400/hr (~1/sec) from DHT crawler, 24h flat. NOT bursty-deep.
- queue_jobs backlog: ~2 pending. Classifier keeps up at steady state. Deep-backlog premise is false at steady state.
- CLASSIFIER_CONCURRENCY=8.

**LLM server (llama.cpp / AMD Lemonade @ 100.125.213.44:8082):**
- Model: gemma-4-26B-A4B-it-qat (MoE, ~4B active of 25B), Q4_K_XL, 14.2GB, multimodal.
- n_ctx_train = 262,144 (the "264k" the user recalled — that's TRAINING context).
- Server loaded context: total_slots=4, per-slot n_ctx=65,536 => ~256k total KV split into 4×64k slots.
- Speculative decoding ON (MTP draft model, draft accepted 3/3 in test).
- Prompt-prefix CACHING active (timings show cache_n) — stable system-prompt prefix amortized across sequential same-slot requests.
- Measured (cold small req): prompt eval ~150 tok/s; generation ~22 tok/s.

**Client config (config.yml) — the artificial limits:**
- batch_size: 1  <-- BATCHING IS EFFECTIVELY OFF on the live box right now.
- max_context: 16000  <-- caps each request at 16k, leaving ~48k/slot unused.
- max_tokens: 256 (output), timeout 30s, interval 5s.

## First-Principles Reframing

1. Two different "batchings" were conflated:
   (i) CLIENT prompt-batching = N torrents in one prompt (what the spec/idea targets).
   (ii) SERVER continuous-batching = llama.cpp runs up to 4 requests in parallel across 4 slots (driven by CONCURRENCY, not prompt size).
2. The throughput bottleneck on this Ryzen-AI box is GENERATION (~22 tok/s), not request count. Prompt-batching does NOT reduce total output tokens, so it does not speed up a generation-bound workload.
3. Prompt-batching SERIALIZES N generations into ONE slot. Spreading N single items across 4 slots generates in PARALLEL. => Large client batches can REDUCE throughput by starving slots. The real throughput lever is keeping all 4 slots saturated (concurrency >= 4).
4. Goal (b) request-overhead: prefix caching already amortizes the system-prompt preamble across sequential same-slot calls, so the marginal saving from prompt-batching is smaller than assumed.
5. Per-request HARD ceiling is ~64k (per slot), but client caps at 16k. Token-budget batch math should target per-slot budget, mindful that total KV (256k) is shared by all 4 slots.


---
techniques_used: ['First Principles Thinking', 'Pre-Mortem']
stepsCompleted: [1, 2, 3]
selected_approach: 'Saturate slots first (defer adaptive batcher)'

## Direction Chosen: Saturate Slots First

Defer the adaptive token-budget batcher. Treat throughput as a slot-utilization +
per-item-cost problem on the generation-bound Ryzen-AI/Lemonade box. Output = tuning +
instrumentation plan, no new batcher logic.

### Evidence: pipeline runs hot (rho ~= 1)
- Job latency created_at->ran_at: avg 136.8s, max 285.7s (last 2h), while instantaneous
  backlog ~2. Classic critically-loaded queue: builds into ~2min waves, drains.
- Capacity model: cap = slots / (prompt_eval_s + gen_s). gen at ~22 tok/s dominates.

### Throughput levers, ranked by leverage (generation-bound, 4 slots)
1. CUT OUTPUT TOKENS (highest): gen at 22 tok/s is the slow stage. Tighten output JSON,
   lower max_tokens (256 is generous), add stop sequence / grammar-constrained decode.
   ~60->25 output tokens saves ~1.6s/item => +25-40% capacity.
2. LEAN + PREFIX-STABLE PROMPTS: keep file-list truncation tight; ensure system prompt is
   BYTE-STABLE so llama.cpp prefix cache hits (then only name+files pay prompt-eval).
3. MORE SERVER SLOTS (--parallel 4->6/8): multiplies throughput IFF NPU has compute
   headroom. Server-side (Lemonade launch args), must A/B test - may be compute-bound.
4. CONCURRENCY >= SLOTS: already 8 >= 4. OK. Do NOT raise max_context (16k) - bigger
   prompts = more prompt-eval = worse throughput on a gen-bound box.

### Instrumentation (endpoint_metrics is FALSE on server)
- Enable llama.cpp --metrics (Prometheus) on Lemonade OR poll /slots state to derive busy%.
- In bitmagnet/db: track queue depth over time, created_at->ran_at p50/p95, items/sec.
- Attribute latency: LLM call time vs queue-wait (the 137s may be partly non-LLM).
- A/B each lever; stop bumping slots when aggregate tok/s stops rising (compute-bound).

### Pre-Mortem: how "saturate slots" fails
1. NPU compute-bound, not slot-bound -> more slots/concurrency = no gain, +latency.
   Mitigation: bump --parallel, watch AGGREGATE tok/s; flat => stop.
2. Prefix cache silently busting (any per-request variance in system prompt) -> prompts
   you think are cheap cost full eval. Mitigation: assert system prompt byte-identical;
   inspect timings.cache_n on a real classify call.
3. Output-token cuts break classification (truncated JSON/missing fields). Mitigation:
   grammar-constrained output; watch invalid-tag failure rate (already nonzero).
4. At rho~1, more concurrency raises throughput AND latency variance; bursts could hit
   the 30s timeout -> failures. (Baseline: failures are NOT timeouts today.)
5. 137s latency may be dominated by non-LLM (scheduling/run_after/DB). If so, slot
   saturation won't fix latency. Mitigation: instrument to attribute before tuning.

### Side-findings (separate bugs, not batching)
- process_torrent_batch: 4x "duplicate key value violates unique constraint" -> the
  BATCHED write path has a dedup/upsert bug. (Batch path is off live but code exists.)
- process_torrent: multiple "invalid tag name" failures (e.g. 'Japanese', 'final-fantasy-vii-')
  -> LLM emits tag names violating bitmagnet tag-format rules. Output-quality / validation gap.
- "missing N info hashes" -> data/availability, unrelated.

