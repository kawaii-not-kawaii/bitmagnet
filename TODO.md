# Remaining tasks

- Turn on armiger shadow mode on tower (`classifier.armiger.enabled: true`, `url`), run ~1 week, then compare `armiger-*` tags with final types to pick per-type thresholds (follow-up spec: let armiger set the type ahead of TMDB).
- Follow-up check: in the full workflow, date-stamped names (`Met-Art 2019-05-25`) may now go `tv_show` via `parse_date`; measure on live data.
- Live OpenRouter check of the batching fix — ticket: `.scratch/llm-batching/issues/01-live-openrouter-check.md` (ready-for-human).
- Re-match the ~344K unmatched movie/tv torrents with a leading `www.` site advert after deploying the site-prefix fix (TMDB at 20 req/s on the shared default key ≈ 5–10 h for just these; `reprocess --contentType movie,tv_show` would re-search all ~1.6M unmatched ≈ 1–2 days). Consider a personal TMDB key first.
- Remaining TMDB misses: bilingual CJK titles after `【..】` adverts (`完美世界 […]Perfect World`), TMDB alternate names (`Storage Wars NY`, `The 1 Percent Club`, romaji anime), transliterated Russian; and ~317K `tv_show` with no episodes (date ⇒ daily-show rule typing adult/sports clips) — armiger shadow data should size the latter.
