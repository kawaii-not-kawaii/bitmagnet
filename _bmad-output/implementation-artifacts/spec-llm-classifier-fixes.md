---
title: 'LLM Classifier Bug Fixes and Code Quality'
type: 'bugfix'
created: '2026-06-17'
status: 'done'
baseline_commit: '28c2b0270da5ad4e505be431205d487c7916035c'
context: []
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** The LLM classifier has 8 correctness and quality issues: an off-by-one in file truncation, context cancellation not propagating to LLM calls, a redundant `min` function, a misleading no-op lifecycle hook, duplicated HTTP retry logic, hardcoded content type list, silent batch count mismatch, and fragile batch system prompt using only the first input.

**Approach:** Fix each issue in-place with minimal mechanical changes — no new abstractions, no new files, no behavior changes beyond what's described.

## Boundaries & Constraints

**Always:** All changes must compile and the existing test suite must pass. Do not change function signatures visible to other packages (e.g. `NewBatchClient`).

**Ask First:** If removing the `OnStop` lifecycle hook (issue 4) would break any other registered hook or shutdown ordering, HALT and ask before removing.

**Never:** Do not add new dependencies, new config fields, or refactor beyond the specific issues listed. Do not change the batch system prompt tone or structure — only fix the inputs[0] fragility.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| File list = 21 files | torrent with 21 file paths | Exactly 20 files written to LLM prompt, then truncation notice | N/A |
| File list ≤ 20 files | torrent with 5 file paths | All 5 files written, no truncation notice | N/A |
| Classifier context cancelled | context.Done() fires during LLM call | Classify returns ctx.Err() immediately | N/A |
| Batch returns fewer results than inputs | 5 inputs, LLM returns 3 results | items 0-2 resolved, items 3-4 get ErrNoResult; warning logged | No panic |

</frozen-after-approval>

## Code Map

- `internal/llm/openai/client.go` -- HTTP client, `Classify`, `BatchClassify`, `doRequest`, `doRequestRaw`, `buildUserMessage`
- `internal/llm/openai/batch.go` -- `BatchClient`, `flush()`, `BatchClassifyJSONString`
- `internal/classifier/action_llm_classify.go` -- classifier action, `buildContentTypeList`, `applyLLMResult`, `sanitizeTag`, custom `min`
- `internal/classifier/classifierllm/module.go` -- DI wiring, `OnStop` lifecycle hook for flush
- `internal/model/content_type_enum.go` -- `ContentTypeNames()` — use this instead of hardcoded list

## Tasks & Acceptance

**Execution:**
- [x] `internal/llm/openai/client.go` -- Fix off-by-one in `buildUserMessage`: move the `i >= 20` guard BEFORE writing the file entry (currently writes 21 files then adds truncation notice). Correct: if `i >= 20`, write `"... and %d more files\n"` then break, without writing the current file.
- [x] `internal/llm/openai/client.go` -- Eliminate `doRequest`/`doRequestRaw` duplication: rename the existing `doRequestRaw` to keep it as the single HTTP retry implementation; refactor `doRequest` to call `doRequestRaw` and then unmarshal the content string into `*llm.ClassifyResult`.
- [x] `internal/llm/openai/client.go` -- Fix `BatchClassify` system prompt fragility: instead of `c.buildSystemMessage(inputs[0])`, build a merged, deduplicated content-types string from all inputs and pass a synthetic `ClassifyInput{ContentTypes: merged}` to `buildSystemMessage`.
- [x] `internal/llm/openai/batch.go` -- Add warning log on batch count mismatch in `flush()`: after `BatchClassify` returns, if `len(results) != len(batch)`, log via `log.Printf` ("llm batch: expected %d results, got %d — filling short positions with ErrNoResult"). No new dependency needed.
- [x] `internal/classifier/action_llm_classify.go` -- Fix context propagation: change `provider.Classify(context.Background(), input)` to `provider.Classify(ctx, input)` — `executionContext` embeds `context.Context` so `ctx` satisfies the interface. Remove the now-unused `"context"` import.
- [x] `internal/classifier/action_llm_classify.go` -- Replace hardcoded `buildContentTypeList()` body with `strings.Join(model.ContentTypeNames(), ", ")` — `model` is already imported. Delete the inline string slice.
- [x] `internal/classifier/action_llm_classify.go` -- Remove the custom `min(a, b int) int` function (lines 184–189). Go 1.23 provides the built-in; the call site at line 66 works unchanged.
- [x] `internal/classifier/classifierllm/module.go` -- Remove the `OnStop` lifecycle hook entirely. `registry.configPath` is `""` so `Flush()` is always a no-op; the hook only produces a misleading "LLM config flushed to disk" log. If no other code calls `registry.Flush()`, remove that method call from the hook before deleting it.

**Acceptance Criteria:**
- Given a torrent with 21 files, when the LLM user message is built, then exactly 20 file lines appear followed by one `"... and 1 more files"` line (not 21 file lines + truncation).
- Given a cancelled context, when `llm_classify` action runs, then the LLM HTTP call is cancelled and the action returns `ctx.Err()` rather than waiting for the full timeout.
- Given a batch of 5 inputs where the LLM returns 3 results, when `flush()` distributes results, then positions 3 and 4 receive `ErrNoResult` and a warning is logged to stderr.
- Given any classifier run, when `buildContentTypeList()` is called, then its output exactly matches `strings.Join(model.ContentTypeNames(), ", ")`.
- Given the app starts and stops normally, when no LLM config path is configured, then no misleading "flushed to disk" log line is emitted on shutdown.

## Design Notes

**Refactoring doRequest (task 2):** After the refactor, `doRequest` should be ~5 lines:
```go
func (c *client) doRequest(ctx context.Context, reqBody []byte) (*llm.ClassifyResult, error) {
    content, err := c.doRequestRaw(ctx, reqBody)
    if err != nil {
        return nil, err
    }
    var result llm.ClassifyResult
    if err := json.Unmarshal([]byte(content), &result); err != nil {
        return nil, fmt.Errorf("%w: %s", llm.ErrInvalidJSON, err)
    }
    if result.ContentType == "" {
        return nil, llm.ErrNoResult
    }
    return &result, nil
}
```

**Batch system message fix (task 3):** Collect content types from all inputs, deduplicate, join. Since all inputs currently share the same ContentTypes, this is a one-liner in the normal case but is now resilient to future divergence.

## Spec Change Log

## Verification

**Commands:**
- `go build ./internal/...` -- expected: no errors
- `go vet ./internal/...` -- expected: no warnings
- `go test ./internal/llm/... ./internal/classifier/...` -- expected: all pass

## Suggested Review Order

**Context propagation & correctness (start here)**

- Entry point: context now flows to HTTP call; uninterruptible LLM calls are fixed
  [`action_llm_classify.go:76`](../../internal/classifier/action_llm_classify.go#L76)

- File truncation guard moved before write; was emitting 21 lines instead of 20
  [`client.go:224`](../../internal/llm/openai/client.go#L224)

**HTTP client refactor**

- `doRequest` now delegates to `doRequestRaw`; retry logic lives in one place only
  [`client.go:244`](../../internal/llm/openai/client.go#L244)

- `doRequestRaw` — unchanged single source of truth for retry + HTTP handling
  [`client.go:260`](../../internal/llm/openai/client.go#L260)

**Batch improvements**

- ContentTypes now merged from all inputs instead of only `inputs[0]`
  [`client.go:148`](../../internal/llm/openai/client.go#L148)

- Warning log fires when batch result count mismatches input count
  [`batch.go:115`](../../internal/llm/openai/batch.go#L115)

**Lifecycle & module**

- `OnStop` hook removed; `registry.Flush()` was always a no-op (empty configPath)
  [`module.go:83`](../../internal/classifier/classifierllm/module.go#L83)

**Supporting / type-level changes**

- `buildContentTypeList()` now uses `model.ContentTypeNames()` — stays in sync with enum
  [`action_llm_classify.go:152`](../../internal/classifier/action_llm_classify.go#L152)
