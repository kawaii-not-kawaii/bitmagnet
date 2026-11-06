# Remaining tasks

- Turn on armiger shadow mode on tower (`classifier.armiger.enabled: true`, `url`), run ~1 week, then compare `armiger-*` tags with final types to pick per-type thresholds (follow-up spec: let armiger set the type ahead of TMDB).
- Follow-up check: in the full workflow, date-stamped names (`Met-Art 2019-05-25`) may now go `tv_show` via `parse_date`; measure on live data.
- Live OpenRouter check of the batching fix — ticket: `.scratch/llm-batching/issues/01-live-openrouter-check.md` (ready-for-human).
