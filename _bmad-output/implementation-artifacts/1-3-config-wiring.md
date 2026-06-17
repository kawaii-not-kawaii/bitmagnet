---
baseline_commit: HEAD
status: done
story_key: 1-3-config-wiring
epic: 1
story: 3
---

# Story 1.3: Wire Provider Config into Classifier YAML

## Story

As a user,
I want to configure LLM providers in the classifier YAML config,
So that I can define my LLM endpoint without code changes.

## Acceptance Criteria

- [x] 1. Add `LlmConfig` and `LlmProviderConfig` to `internal/classifier/config.go`
- [x] 2. Add `llmProviders` to `internal/classifier/dependencies.go`
- [x] 3. Add `LlmProviders` to `internal/classifier/factory.go` Params
- [x] 4. Create `internal/classifier/classifierllm/module.go` fx module
- [x] 5. Register classifierllm in `internal/app/appfx/module.go`
- [x] 6. Create `internal/llm/registry.go` — live-updatable provider registry with Flush on shutdown

## Dev Agent Record

### Completion Notes

Added full LLM config pipeline from YAML to runtime:
- `LlmConfig` struct in `classifier.Config` with providers, batch_size, max_context, max_tokens, interval, timeout
- `LlmProviderConfig` struct with base_url, model, api_key, timeout, system_prompt
- `llmProviders map[string]llm.Provider` in classifier dependencies
- `classifierllm` fx module reads config, creates providers via `openai.New()`, provides registry + static map
  - `*llm.Registry` for live-updatable providers (RWMutex, `Update()`, `Flush()`)
  - `map[string]llm.Provider` for backwards-compatible static access
  - `fx.Lifecycle.Append(OnStop)` hook calls `registry.Flush()` on graceful shutdown
- `llm.Registry` holds providers + config with concurrent-safe access
  - `Flush()` merges runtime LLM config changes back into the YAML config file on disk
  - `Update()` re-creates providers from new config at runtime
- Registered classifierllm module in appfx

## File List

- `internal/llm/registry.go` (new) — live-updatable provider registry with Flush
- `internal/classifier/config.go` (modified) — added LlmConfig, LlmProviderConfig
- `internal/classifier/dependencies.go` (modified) — added llmProviders
- `internal/classifier/factory.go` (modified) — added LlmProviders to Params
- `internal/classifier/classifierllm/module.go` (new) — fx module with lifecycle hook
- `internal/app/appfx/module.go` (modified) — registered classifierllm

## Change Log

- Story 1.3: LLM provider config wired through fx DI with live-reload and shutdown flush.

