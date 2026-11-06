# 01: live OpenRouter check of the batching fix

What to build: nothing new. Verify commit 1a7f90f (scaled batch timeouts, provider-sized flush deadline, admission ≥ batch_size, `output-truncated` error) in a running bitmagnet against OpenRouter, using the config below, and record the evidence here.
Blocked by: none.
Status: ready-for-human

Config under test (classifier.llm):

- provider_base_url `https://openrouter.ai/api`, provider_model `nvidia/nemotron-3.5-super-vl:free`, provider_api_key (a fresh key; the one pasted in chat on 2026-11-06 should be rotated)
- batch_size 100, max_tokens 600, interval 60s, timeout 30s

Acceptance:

- [ ] Reprocess ~300 unknown torrents; logs show batched requests (one request per up to 100 torrents, not one per torrent) and OpenRouter usage shows ~3–4 requests, not ~300.
- [ ] No batch fails on timeout; per-request duration recorded (expect ~1–3 min for 100).
- [ ] Dashboard effective concurrency shows ≥ batch_size while batching, autoscale paused.
- [ ] With `max_tokens: 50`, failures are reported as `output-truncated` in the LLM stats, not `empty-content` / `invalid-json`.
- [ ] No processor job hits its 10-minute timeout.
- [ ] Evidence (log excerpts, request counts, timings) appended below under `## Comments`.
