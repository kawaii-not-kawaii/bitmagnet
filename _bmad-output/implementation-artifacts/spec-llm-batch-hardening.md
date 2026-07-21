---
title: 'LLM Batch Client Hardening'
type: 'bugfix'
created: '2026-06-17'
status: 'done'
baseline_commit: '73d131c24331b1b5aa9a746f27db9455886030ea'
context: []
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `BatchClient` has three robustness gaps — no lifecycle context (flush uses `context.Background()`), an unchecked type assertion that will panic under invariant violation, and no shutdown drain, leaving pending callers blocked on process exit. Additionally, `doRequestRaw` has two silent error-handling bugs: a body-level error with an empty `message` field passes undetected, and body-level API errors (HTTP 200 + JSON error) are retried like transient failures instead of short-circuiting. A dead `batchMode bool` parameter rounds out the cleanup.

**Approach:** Add a lifecycle context and `Drain()` method to `BatchClient`, wire `Drain()` into the fx OnStop hook via duck-typing, fix the type assertion, remove the dead parameter, and tighten both error guards in `doRequestRaw`.

## Boundaries & Constraints

**Always:** No change to public function signatures (`NewBatchClient`, `ParseBatchResponse`). All changes are in `internal/llm/openai/` and `internal/classifier/classifierllm/`. Must compile without errors.

**Ask First:** If `errors` package import would create a dependency cycle in `batch.go`, halt and ask.

**Never:** Do not change the batch flush timeout (60s). Do not add retry logic to `Drain()`. Do not change the `llm.Provider` or `llm.BatchProvider` interfaces.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Shutdown while batch pending | `Drain()` called with 3 pending callers | All 3 receive `batchResult{err: "shutting down"}` immediately | No goroutine leak |
| Shutdown while flush in-flight | `Drain()` called while `BatchClassify` HTTP is running | HTTP request cancelled via `lifecycleCtx`; callers already unblocked (on `resultCh`) | No hang |
| Provider lacks BatchProvider | `BatchClient` wraps non-BatchProvider | All pending items routed to error; no panic | Graceful error per item |
| HTTP 200 + `{"error": {"message": ""}}` | Response body with empty error message | Error detected, `lastErr` set, loop continues retrying | Retried up to maxRetries |
| HTTP 200 + `{"error": {"message": "quota exceeded"}}` | Non-empty message = identifiable permanent error | Short-circuit: return error immediately, no further retries | `return "", lastErr` |

</frozen-after-approval>

## Code Map

- `internal/llm/openai/batch.go` -- `BatchClient`, `flush()`, `NewBatchClient`, `Drain()` (to add)
- `internal/llm/openai/client.go` -- `doRequestRaw`, `buildUserMessage`
- `internal/classifier/classifierllm/module.go` -- fx lifecycle wiring

## Tasks & Acceptance

**Execution:**
- [x] `internal/llm/openai/batch.go` -- Add `lifecycleCtx context.Context` and `lifecycleCancel context.CancelFunc` fields to `BatchClient`. In `NewBatchClient`, initialize with `context.WithCancel(context.Background())` before constructing the struct.
- [x] `internal/llm/openai/batch.go` -- In `flush()`: (a) replace `bc.provider.(llm.BatchProvider)` with comma-ok form; if !ok, route all batch items to `batchResult{err: errors.New("llm: provider does not implement BatchProvider")}` and return. (b) Replace `context.WithTimeout(context.Background(), 60*time.Second)` with `context.WithTimeout(bc.lifecycleCtx, 60*time.Second)` so lifecycle cancellation propagates to in-flight HTTP requests.
- [x] `internal/llm/openai/batch.go` -- Add `Drain()` method to `*BatchClient`: (1) acquire lock, stop timer, drain `bc.pending` to a local slice, set `bc.pending = nil`, release lock. (2) Call `bc.lifecycleCancel()` to abort any in-flight flush. (3) Send `batchResult{err: errors.New("llm batch: shutting down")}` to every pending caller's result channel.
- [x] `internal/llm/openai/client.go` -- Remove `batchMode bool` parameter from `buildUserMessage`. Update the single call site (`c.buildUserMessage(input, false)` → `c.buildUserMessage(input)`).
- [x] `internal/llm/openai/client.go` -- In `doRequestRaw`, change the body-error guard from `chatResp.Error.Message != ""` to `chatResp.Error != nil`. This detects errors even when `message` is empty.
- [x] `internal/llm/openai/client.go` -- In `doRequestRaw`, after detecting a body-level API error: if `chatResp.Error.Message != ""` (identifiable permanent error), short-circuit with `return "", lastErr`; otherwise (empty message = ambiguous/transient) keep `continue` to retry.
- [x] `internal/classifier/classifierllm/module.go` -- Re-add OnStop lifecycle hook. Iterate over `providers` map; for each provider, duck-type assert to `interface{ Drain() }` and call `Drain()` if satisfied. Add back the `"context"` import.

**Acceptance Criteria:**
- Given `Drain()` is called while 3 requests are pending in `bc.pending`, then all 3 callers receive an error immediately and no goroutines are left blocked.
- Given `Drain()` is called while a `BatchClassify` HTTP call is in-flight, then `bc.lifecycleCtx` is cancelled, the HTTP request aborts, and results distributed before `Drain()` are unaffected.
- Given `BatchClient` wraps a provider that does not implement `BatchProvider`, when `flush()` runs, then all pending items receive an error and no panic occurs.
- Given `doRequestRaw` receives HTTP 200 with `{"error": {"message": ""}}`, then `lastErr` is set and the retry loop continues (error detected but treated as transient).
- Given `doRequestRaw` receives HTTP 200 with `{"error": {"message": "quota exceeded"}}`, then the function returns the error immediately without further retries.
- Given `buildUserMessage` is called, then the `batchMode` parameter no longer exists (compilation confirms removal).

## Design Notes

**Drain() ordering matters:** Call `lifecycleCancel()` AFTER draining `bc.pending` to local. This ensures the pending items are captured before the in-flight flush (if any) completes and tries to send to result channels. The cancel races with any in-flight flush's distribution loop, but that loop already safely sends one result per channel (buffered with cap 1).

**Duck-typed drain in module:** `for _, prov := range providers { if d, ok := prov.(interface{ Drain() }); ok { d.Drain() } }` — no import needed; providers map is captured at construction time before the lifecycle hook fires.

**Body-error short-circuit:** The 4xx HTTP short-circuit uses `return nil, lastErr`. The body-error short-circuit mirrors this with `return "", lastErr`. Both are "permanent error, no point retrying."

## Spec Change Log

### Loop 1 (2026-06-17)
**Finding:** Task 6 was over-broad — `return "", lastErr` was applied unconditionally to all body errors including empty-message ones, violating AC4 which specified empty-message errors should retry (only non-empty/identifiable permanent errors should short-circuit).
**Amended:** I/O matrix, task 6 description, and AC4 updated to distinguish: `Message != ""` → short-circuit; `Message == ""` → continue retrying.
**Known-bad state avoided:** Transient server errors returned as HTTP 200 with `{"error":{"message":""}}` would previously short-circuit all retries, silently failing classification.
**KEEP:** `chatResp.Error != nil` guard (task 5) is correct and must survive. All batch.go changes, module.go OnStop hook, and batchMode removal are correct and must not be re-derived.

## Verification

**Commands:**
- `go build ./internal/...` -- expected: no errors
- `go vet ./internal/...` -- expected: no warnings
- `go test ./internal/llm/... ./internal/classifier/...` -- expected: all pass

## Suggested Review Order

**Lifecycle context & Drain (core of this change)**

- New fields wiring shutdown cancellation into every flush; start here for design intent
  [`batch.go:25`](../../internal/llm/openai/batch.go#L25)

- `NewBatchClient` now initialises the lifecycle context before returning
  [`batch.go:42`](../../internal/llm/openai/batch.go#L42)

- `Drain()`: drains pending to error, cancels lifecycle context to abort in-flight HTTP
  [`batch.go:148`](../../internal/llm/openai/batch.go#L148)

**flush() hardening**

- Comma-ok replaces panicking assertion; lifecycleCtx replaces context.Background()
  [`batch.go:115`](../../internal/llm/openai/batch.go#L115)

**Module wiring**

- OnStop hook calls Drain() on any BatchClient provider via duck-type; no API change needed
  [`module.go:85`](../../internal/classifier/classifierllm/module.go#L85)

**HTTP error handling**

- Non-empty message → short-circuit; empty message → descriptive lastErr, retry
  [`client.go:310`](../../internal/llm/openai/client.go#L310)

**Dead code removal**

- `batchMode bool` removed; internal call site updated
  [`client.go:218`](../../internal/llm/openai/client.go#L218)
