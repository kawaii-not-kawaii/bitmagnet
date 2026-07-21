# Deferred Work — LLM Batch/Client/Classifier

> **Audited 2026-07-21 against current code (HEAD + uncommitted working-tree changes).**
> Every original entry below was verified by reading the code, not the prior doc.
> Items attributed to a **working-tree change** are present in the tree but NOT yet
> committed — `git status` will show `internal/llm/openai/batch.go` and
> `internal/classifier/classifierllm/module.go` as modified.
>
> Verdict legend: **Fixed** (verified in code), **Still Open** (unresolved or
> unverifiable), **Obsolete** (no longer applicable).
>
> Commits cited:
> - `73d131c2` — fix: LLM classifier correctness and quality cleanup (2026-06-17)
> - `1ba810b2` — fix: harden LLM batch client with lifecycle context and drain (2026-06-17)
> - `f4739faa` — fix: resolve deferred LLM batch/client issues and pass lint (2026-06-17)

## Still Open

- **`internal/llm/provider_test.go:44` `TestClassifyInput_ValueSemantics` fails at clean HEAD.**
  Test asserts that mutating `copied.Files[0]` on a value-copy of a `ClassifyInput`
  does not mutate the original. It currently fails with `Files slice was shared
  (slice header copy)` at `provider_test.go:57`. This is expected Go slice-header
  behavior — a value copy of a struct with a slice field shares the underlying
  array — so the *test* may be wrong rather than the type. **Under investigation by
  another agent**: confirm whether this is a test bug (test should clone the slice
  explicitly) or a real missing invariant on `ClassifyInput` (e.g. `Clone()` method
  or deep-copy on provider ingress). Do not resolve here.

- **`Registry.Update()` has no production caller** — informational, not a regression.
  The drain-on-eviction fix below is implemented and verified at
  `internal/llm/registry.go:107-111`, but `Update()` is never invoked from any
  production code path today (confirmed by grep across the module). When live
  reconfiguration is wired, it will work; until then the fix is unexercised in
  production. Listed here for visibility, not as a blocker.

## Fixed (verified)

### Originally from spec-llm-classifier-fixes (2026-06-17)

- **`flush()` no longer uses `context.Background()`** — `internal/llm/openai/batch.go:144`
  now does `ctx, cancel := context.WithTimeout(bc.lifecycleCtx, flushTimeout)`, where
  `lifecycleCtx` is cancelled by `Drain()`. Caller-context budgets are still not
  merged (the batch deliberately decouples caller cancellation from the shared HTTP
  call), but shutdown now aborts in-flight requests. **Fixed in `1ba810b2`.**

- **Unchecked `BatchProvider` type assertion** — `internal/llm/openai/batch.go:135`
  uses comma-ok form `bp, ok := bc.provider.(llm.BatchProvider)`; the `!ok` branch
  (lines 136-141) routes every pending caller to a descriptive error instead of
  panicking. **Fixed in `1ba810b2`.**

- **`BatchClient` has a shutdown drain** — `internal/llm/openai/batch.go:176-197`
  defines `Drain()`: sets `drained`, stops the timer, captures pending, cancels
  `lifecycleCtx`, routes pending callers to `fmt.Errorf("llm batch: shutting down")`,
  and `wg.Wait()`s on in-flight flush goroutines. Wired into the fx lifecycle at
  `internal/classifier/classifierllm/module.go:95-109`. **Fixed in `1ba810b2`
  (Drain + wiring); `wg.Wait()` hardening in `f4739faa`.**

- **Dead `batchMode` bool param removed from `buildUserMessage`** —
  `internal/llm/openai/client.go:235` signature is now
  `func (*client) buildUserMessage(input llm.ClassifyInput) string`; sole caller
  `buildRequest` at `client.go:198` passes no flag. **Fixed in `1ba810b2`.**

- **Empty `error.Message` no longer silently passes** —
  `internal/llm/openai/client.go:337` guards on `chatResp.Error != nil` (pointer
  check, not the empty-string `Message` check). Inside that block (line 338),
  `Message != "" || Type != ""` short-circuits; the both-empty branch (lines
  349-352) sets a descriptive `lastErr` (`"openai: API error: empty error object"`)
  and retries. **Fixed in `1ba810b2`.**

- **Body-level API errors short-circuit when permanent** —
  `internal/llm/openai/client.go:337-348`: any body-level error with a non-empty
  `Message` or `Type` returns immediately (`return "", lastErr`) instead of
  burning all `maxRetries` attempts. Only the ambiguous "both fields empty" case
  retries. **Fixed in `1ba810b2` (Error!=nil guard); `Type != ""` short-circuit
  added in `f4739faa`.**

- **`ParseBatchResponse` single-result fallback removed** —
  `internal/llm/openai/batch.go:232-261`: the third parsing branch that returned
  `[]*llm.ClassifyResult{&single}` for a bare JSON object is gone. The function now
  accepts only `{"results":[...]}` or a bare array, and returns `llm.ErrInvalidJSON`
  for a single-object response. `flush()` (lines 158-169) routes the error to every
  pending caller, which surfaces as `ErrUnmatched` upstream. Safe because
  `BatchClassify` short-circuits `len(inputs)==1` through `Classify`
  (`client.go:136-143`), so `ParseBatchResponse` is only ever reached with N>=2.
  Covered by `internal/llm/openai/batch_test.go`. **Fixed in working tree
  (uncommitted, `internal/llm/openai/batch.go` + new `batch_test.go`).**

### Originally from spec-llm-batch-hardening (2026-06-17)

- **`OnStop` no longer returns before in-flight flush goroutines complete** —
  `internal/llm/openai/batch.go:30` adds `wg sync.WaitGroup` to `BatchClient`;
  `flush()` calls `bc.wg.Add(1)` under the mutex at line 124 (so `Drain()` cannot
  race past it) and `defer bc.wg.Done()` at line 126. `Drain()` calls
  `bc.wg.Wait()` at line 196 before returning. The OnStop hook in
  `internal/classifier/classifierllm/module.go:95-109` invokes `Drain()` via the
  `llm.Drainer` interface, so fx teardown cannot overtake the HTTP unwind.
  **Fixed in `f4739faa`.**

- **Post-drain `Classify` calls return "shutting down", not `context.Canceled`** —
  `internal/llm/openai/batch.go:77-80`: `Classify` checks `bc.drained` under the
  mutex atomically with the enqueue and returns `fmt.Errorf("llm batch: shutting
  down")` immediately. `Drain()` sets `bc.drained = true` at line 179 before
  releasing the mutex. **Fixed in `f4739faa`.**

- **Flush timeout sized to the retry chain** — `internal/llm/openai/batch.go:16`:
  `const flushTimeout = (maxRetries + 1) * defaultTimeout` = `(3+1) * 30s` = `120s`,
  which covers the worst-case `(maxRetries+1) × per-request HTTP timeout`. Replaces
  the prior hardcoded 60s that cancelled mid-retry. **Fixed in `f4739faa`.**

- **Deterministic OnStop drain order** —
  `internal/classifier/classifierllm/module.go:97-106`: OnStop collects provider
  names into a slice, `sort.Strings(names)` at line 101, then iterates the sorted
  names to call `Drain()`. Eliminates the random map-iteration ordering. **Fixed in
  working tree (uncommitted, `internal/classifier/classifierllm/module.go`).**

- **`Drainer` is a named interface in the `llm` package** —
  `internal/llm/provider.go:83-88` defines `type Drainer interface { Drain() }`.
  The OnStop hook at `internal/classifier/classifierllm/module.go:103` type-asserts
  to `llm.Drainer` (not an anonymous duck-typed interface), so any decorator that
  wants to participate in shutdown must forward the interface explicitly.
  `NewBatchClient` still returns `llm.Provider` (`internal/llm/openai/batch.go:48`)
  — `Drain` remains reachable via the named interface assertion, which is the
  intended shape. **Fixed in `f4739faa`.**

### Originally from spec-llm-batch-hardening loop 2 (2026-06-17)

- **`Registry.Update()` drains evicted providers** —
  `internal/llm/registry.go:90-112`: docstring at line 93 states "Any evicted
  provider that implements Drainer is drained before being discarded." Lines
  107-111 iterate `old` (the pre-replacement provider map) and call `d.Drain()` on
  each provider that implements `Drainer`, *after* the new map is published but
  before `Update` returns. Prevents orphaned timers/goroutines on config swap.
  Caveat: see the Still Open note — `Update()` is not yet called from any
  production path. **Fixed in `f4739faa`.**

- **Type-only API errors (`Message==""`, `Type!=""`) short-circuit** —
  `internal/llm/openai/client.go:338`: the short-circuit condition is
  `chatResp.Error.Message != "" || chatResp.Error.Type != ""`. A response with only
  `Type` set now returns immediately at line 347 with a descriptive error
  (`"openai: API error: (no message) (type=<type>)"`) instead of wasting
  `maxRetries`. **Fixed in `f4739faa`.**

## Obsolete / No Longer Applicable

_None._ All fourteen original items mapped cleanly to a Fixed verdict; nothing was
rendered moot by API changes or removed features.
