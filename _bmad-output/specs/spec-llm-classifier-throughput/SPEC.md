---
id: SPEC-llm-classifier-throughput
companions: [investigation.md]
sources: [../../brainstorming/brainstorming-session-2026-06-17-174649.md]
---

> **Canonical contract.** This SPEC and the files in `companions:` are the complete, preservation-validated contract for what to build, test, and validate. Source documents listed in frontmatter are for traceability only — consult them only if you need narrative rationale or prose color this contract intentionally omits.

# LLM Classifier Throughput: Saturate Slots, Cut Per-Item Cost

## Why

A pain to solve. The bitmagnet LLM classifier runs at ρ ≈ 1 on a generation-bound AMD Lemonade/llama.cpp box (gemma-4-26B-A4B, ~22 tok/s generation). Live measurement on 2026-06-17 showed average job latency of 137s (max 286s) while instantaneous backlog was ~2 — the signature of a critically-loaded queue whose capacity barely matches the steady ~1/sec DHT arrival, so any LLM slowdown silently grows a multi-minute backlog. The operator wants to reclaim throughput headroom and reduce per-item cost. A live first-principles investigation rejected the originally-intended fix (an adaptive token-budget batcher): on a 4-slot continuous-batching server, large client-side prompt batches *serialize* generations into one slot and starve the other three, reducing throughput. The real levers are cutting generation work and keeping all four slots fed. See `investigation.md` for the measured topology and the rejected-batcher rationale.

## Capabilities

- id: CAP-1
  intent: Operator can attribute end-to-end job latency into LLM-call time vs. queue-wait/scheduling time, and observe per-slot utilization, before any tuning lever is applied.
  success: A metric or log breakdown exists that splits `created_at → ran_at` latency into LLM-call duration vs. non-LLM wait, and slot busy-state is observable (via `/slots` polling or enabled llama.cpp `--metrics`); demonstrable on the live instance for a sample of recent jobs.

- id: CAP-2
  intent: System minimizes generation work per classification, since generation (~22 tok/s) is the throughput bottleneck.
  success: Measured average output tokens per single classification drops materially (target ≥40%) and `llm_classify_duration_seconds` p50 falls, with no rise in empty/invalid classifications vs. the pre-change baseline.

- id: CAP-3
  intent: System keeps prompts lean and the system prompt byte-stable so llama.cpp prefix caching reuses the preamble across consecutive same-slot calls.
  success: A real classify call's `timings.cache_n` shows prefix reuse on consecutive requests, confirming the system preamble is not re-evaluated per call; file-list inclusion stays tightly bounded.

- id: CAP-4
  intent: Operator can decide, from data, whether raising the server slot count (`--parallel` 4→6/8) increases throughput or hits a compute-bound ceiling.
  success: A recorded A/B decision backed by aggregate tok/s and p95-latency measurements — either slots raised with a measured throughput gain, or documented as compute-bound and left at 4.

## Constraints

- Instrument first: no tuning lever (CAP-2/3/4) is merged before CAP-1's latency attribution exists — the 137s latency may be partly non-LLM, and tuning the wrong stage wastes effort.
- Do NOT raise `max_context` (currently 16000). Bigger prompts increase prompt-eval cost and reduce throughput on a generation-bound box.
- Keep client concurrency ≥ server slot count (currently 8 ≥ 4); never drop in-flight requests below the slot count, or slots idle.
- Output-token reduction must not regress correctness: the empty-result and invalid-tag-name failure rate (already nonzero today) must not increase.
- Every request must fit within one slot's context budget and complete within the 30s per-request timeout.
- The system prompt must remain byte-identical across requests (no per-request variance beyond the stable `ContentTypes` interpolation) to preserve prefix-cache hits.
- Server-side changes (`--parallel`) are Lemonade launch arguments outside the bitmagnet repository — coordinate with the inference host; do not assume bitmagnet config controls them.

## Non-goals

- The adaptive token-budget batcher (dynamic batch size derived from backlog depth and per-slot context budget) — explicitly deferred until hardware changes (a real GPU) or a genuine reprocess-everything burst justifies it.
- Raising `max_context` or the client `batch_size` above 1.
- Fixing the `process_torrent_batch` duplicate-key bug or the LLM "invalid tag name" output failures — both are real and observed, but tracked separately from this throughput work.
- Switching the model, quantization, or inference hardware.

## Success signal

The classifier sustains the ~1/sec DHT arrival with visible headroom: p50 `created_at → ran_at` job latency drops well below today's 137s and stays stable through arrival bursts, achieved by cutting generation cost (CAP-2) and confirming slot utilization (CAP-1/CAP-4) — with the adaptive batcher never built. Demonstrable by comparing `llm_classify_duration_seconds` and job-latency distributions before and after.

## Assumptions

- The ~22 tok/s generation rate measured once (cold, small request) is the steady-state bottleneck under concurrent load. CAP-1 exists partly to confirm this.
- `ContentTypes` interpolated into the system prompt is constant across calls, so the preamble is a stable cacheable prefix. Needs verification (CAP-3).
- At least part of the 137s latency is LLM-attributable rather than pure scheduling/`run_after` delay — the premise CAP-1 tests.

## Open Questions

- Is the box compute-bound or slot-bound? This decides CAP-4's outcome.
- What fraction of the 137s latency is LLM call vs. queue wait vs. `run_after` scheduling?
- Does the `max_context` (16k) config gate anything in the request-building path today? A grep shows it plumbed into config but not referenced in `buildRequest`; it may be dead or used only on a batch path that is off live.
