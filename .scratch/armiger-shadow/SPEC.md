# SPEC: shadow-mode `armiger_classify` action

Status: done

Evidence: `.scratch/classifier-parser-research/armiger/METRICS.md` and `.scratch/classifier-parser-research/llm/METRICS.md` (200 hand-labeled names).

## Problem Statement

Unknown torrents are typed by file-extension rules, then the LLM as a last resort; bare titles never reach TMDB because the parser needs a year or episode marker first. armiger-ultra, an 8M-parameter ONNX torrent classifier running as a separate service, scored 0.892 on the labeled set at ~10 ms per name and was 100% correct at confidence ≥0.8 (107/200 names); an armiger-then-LLM hybrid reached 0.949–0.968 with ~65% fewer LLM calls. But 200 names (13 movies) is too small to pick confidence thresholds per content type, and letting an unvalidated model set content types on live data risks silently mis-filing torrents.

## Solution

Add a workflow action, `armiger_classify`, that sends the torrent's metadata to a configured armiger service and records its answer as tags only: the predicted type and a confidence bucket. It never changes content type, title or any other attribute, and never fails classification. The operator runs it on live traffic for about a week, then compares armiger's tags against what the rules, LLM and TMDB decided, to choose per-type thresholds before a later spec lets armiger set the type ahead of TMDB.

## User Stories

1. As an operator, I want armiger's prediction recorded on each newly classified unknown torrent, so that I can compare it with the final classification on live data.
2. As an operator, I want the prediction as tags (`armiger-<type>`, e.g. `armiger-tv-show`), so that I can filter and count them with the existing tag search and facets.
3. As an operator, I want a confidence bucket tag (`armiger-conf-0` … `armiger-conf-90`, in tenths), so that I can measure accuracy per confidence band and pick thresholds.
4. As an operator, I want shadow mode never to change content type or metadata, so that classification results are identical with the action on or off.
5. As an operator, I want classification to continue normally when armiger is down, slow or returns garbage, so that the shadow experiment can't hurt the indexer.
6. As an operator, I want a short, configurable timeout on armiger calls, so that a hung service doesn't stall the classifier.
7. As an operator, I want to enable or disable the action and set the armiger URL in classifier config, so that I can turn the experiment on and off without editing the workflow.
8. As an operator, I want the action to be a no-op when disabled or unconfigured, so that the default workflow works for users without armiger.
9. As an operator, I want armiger to see the torrent name, total size and file list (path + size), so that it gets the same evidence it was built for.
10. As an operator, I want the call to run only for torrents without a hinted content type, so that already-typed torrents don't cost a request.
11. As an operator, I want the call placed before the extension rules and the LLM, so that every unknown torrent gets a shadow tag regardless of which later step decides its type.
12. As an operator, I want armiger's `unknown` prediction recorded too (`armiger-unknown`), so that abstentions are visible.
13. As an operator, I want armiger's adult/CSAM/junk heads ignored, so that safety decisions stay with the existing banned-keyword rules (armiger called a CSAM-descriptive name `safe` in testing).
14. As an operator, I want failures logged at warn level with the reason, so that I can see if the shadow data is incomplete.
15. As a maintainer, I want the armiger HTTP contract (request fields, `category` + `heads.category.confidence` response) isolated in a small client, so that a future version bump touches one place.
16. As a maintainer, I want the action registered and documented in the classifier JSON schema like the other actions, so that workflow validation and editor tooling know about it.
17. As a maintainer, I want the config to follow the existing classifier config resolution (snake_case keys, live apply), so that it behaves like the LLM settings.

## Implementation Decisions

- New workflow action `armiger_classify` (literal payload, like `llm_classify`), registered with the default features and in the classifier JSON schema.
- New config block under the classifier config: `armiger.enabled` (default false), `armiger.url` (base URL of the armiger service, e.g. the container on tower), `armiger.timeout` (default ~2s). Applied through the existing classifier live-apply path.
- New small armiger client module: `Classify(ctx, name, size, files) → (category, confidence, error)`, calling `POST /v1/classify` with `{name, size, files:[{path,size}]}` and reading `category` and `heads.category.confidence`. Unknown categories map to `unknown`.
- Tag mapping: `armiger-` + category in kebab-case (`tv_show` → `tv-show`); confidence bucket = floor(confidence × 10) × 10 → `armiger-conf-<n>`. Both validated with the existing tag-name rule.
- The action adds tags to the result's tag set and returns the result otherwise unchanged; every error path returns the unchanged result with no error.
- Default workflow: the action runs inside the "content type unknown" branch, before `find_match`. Because it is a no-op when disabled, shipping it in the default workflow changes nothing for users without armiger.
- armiger must run with `ARMIGER_UPDATE_CHANNEL=off` and sharing off; the deploy note records this, the code does not enforce it.

## Testing Decisions

- Single seam: compile the core workflow and call `workflow.Run`, as `TestClassifier` does, with the real armiger client pointed at an `httptest` server. Assert only the returned classification result (tags present, content type and attributes unchanged versus the same run with armiger disabled).
- Cases: confident `tv_show` prediction → `armiger-tv-show` + `armiger-conf-90`; low-confidence prediction → correct bucket; `unknown` → `armiger-unknown`; server 500, timeout and malformed JSON → no armiger tags, run succeeds, rest of result identical; disabled → server never called; torrent with hinted content type → server never called; request body carries name, size and file list.
- Prior art: `TestClassifier` (workflow-level run with mocks), the LLM classify action tests (provider fakes, behaviour preserved on errors), and the openai client tests (httptest servers).

## Out of Scope

- Letting armiger set content type, or routing low-confidence names to the LLM (follow-up spec once thresholds are chosen from shadow data).
- Using armiger's adult, junk, CSAM, swarm or honeypot outputs.
- Running armiger's own Bitmagnet connector or Torznab proxy.
- Deploying or operating the armiger container (operator task); a dashboard for the comparison (a SQL query over tags is enough for the first week).

## Further Notes

- Comparison after a week: group torrents by `armiger-<type>` and `armiger-conf-<n>` against their final content type and whether TMDB matched; the labeled set suggests ~0.7–0.8 as a starting threshold, varying by type (TV and xxx strong; comic, ebook and software noisy).
- armiger never abstained on the labeled set, so expect few `armiger-unknown` tags.

## Comments

Implemented as `armiger_classify` (tags only, never errors).

Verification:

- Tests written first against the workflow seam (`internal/classifier/action_armiger_classify_test.go`: core workflow compiled, `workflow.Run`, real `armiger.Client` against `httptest`). Red run before the action existed: `TestArmigerClassify_TagsOnly` and `_RequestCarriesNameSizeAndFiles` failed on assertions. Green after.
- Covered: confident `tv_show` -> `armiger-tv-show` + `armiger-conf-90`; bucket edges (0, 0.37, 0.7, 1.0 -> 90); `unknown`; unrecognised category -> `armiger-unknown`; HTTP 500, timeout, malformed JSON, missing confidence head and unreachable server -> no armiger tags, no error, result identical to a disabled run; disabled/unconfigured and hinted content type -> server never called; request body has name, size and files.
- Also: config defaults/snake_case override test, live-apply test (`classifierllm`), `TestJSONSchema` (core workflow validates against the schema).
- `go test ./...` passes. golangci-lint (CI config): 13 findings, all pre-existing.
- Smoke check against the real service (`127.0.0.1:18096`): `Breaking.Bad.S01E01.720p.BluRay.x264` -> `tv_show`, confidence 0.955 (~0.9 s including cold connection).
- `bitmagnet.io/schemas/classifier-0.1.json` regenerated with `go run . classifier schema --format json`; the committed file was already stale (missing `llm_classify`), so the diff also adds that entry.

Example config:

```yaml
classifier:
  armiger:
    enabled: true
    url: http://armiger:8080
    timeout: 2s
```

Notes: applied live via the `classifier` LiveApplier (the shared client's config is swapped atomically; the action reads it per call). Single-file torrents send no `files` (the list is only sent when file rows exist). Run armiger with `ARMIGER_UPDATE_CHANNEL=off` and sharing off.
