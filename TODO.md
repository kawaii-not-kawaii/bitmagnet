# Remaining tasks

- (optional, next session) Year-heuristic guard: reject year candidates embedded in date/version patterns (`2026.09.06`, `v14.02.2023`, `2019-05-25`) — would lift movie precision from 0.40 on the labeled set.
- Shadow-mode `armiger_classify` action: tag-only (`armiger:<type>`, confidence) for ~1 week of live traffic, then pick per-class thresholds before letting it set content type ahead of TMDB, with the LLM as low-confidence fallback.
- Live check of the batching fix against OpenRouter in a running instance (batch_size 100, max_tokens 600, interval ≤2m).
