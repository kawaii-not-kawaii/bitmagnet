# Labeled classification check — non-LLM parser

Date: 2026-10-23 · Closes the TODO item "labeled non-LLM classification check".

## Method

- Data: the existing 200-name live sample of unknown torrents (`../sample/`, collected 2026-10-11, two strata; see `../COMPARISON.md`).
- Ground truth: hand labels per name (`labels.csv`; `xxx` = adult content — bitmagnet has no such type — `other` = non-classifiable/non-media/uncertain). Labels were assigned by name + fileType/files evidence, then audited by joining label↔name; three mis-anchored regions (47–79, 108–118, 177–198) were caught in audit and rebuilt from the names. Remaining label noise: a handful of individually uncertain calls (e.g. `City on fire.mkv`, `ELO-202.avi`, `Mercenaries`) — flagged `other`/best-guess, not silently confident.
- Prediction source: actual `parsers.ParseVideoContent` output (conservative gating) recorded in `../sample/results_raw.jsonl` (pre-fix, commit 5793f5c parsers) and `../sample/results_new.jsonl` (post-fix, commit 8af7f7a).
- Metrics script: this file was generated from `labels.json` + both result files; full per-row lists in `METRICS.md`.

## Results (n=200; labeled: 38 tv_show, 13 movie, 149 non-movie/tv)

| class | pre-fix tp/fp | pre recall/precision | post-fix tp/fp | post recall/precision |
|---|---|---|---|---|
| tv_show | 3/0 | 0.079 / 1.000 | 20/0 | **0.526 / 1.000** |
| movie | 2/3 | 0.154 / 0.400 | 2/3 | 0.154 / 0.400 |

- False-typed on non-movie/tv labels: 3/149 (2.0%), identical before and after the fix — the episode work added **zero** new false classifications.
- All 3 false positives are the same pre-existing failure mode: **date-like tokens parsed as years** → `movie`: `Met-Art 2019-05-25`, `[Pixiv] ... 2026.09.06.zip`, `Raining Blood Demo v14.02.2023.rar`.
- 29 of 51 movie/tv labels remain untyped — by policy: no year or episode evidence in the name (bare titles like `Heartbreak High`, `Oregairu`-style). Recovering these is content-matching (TMDB) territory, not parser territory; the conservative parser is behaving as designed.

## Conclusions

1. The episode-pattern fix (d0af495/8af7f7a) raised TV recall from 8% to 53% on genuinely unknown-bucket names at **100% TV precision** — measured, not assumed.
2. The one measured weakness left in the parser is the year heuristic: any delimited `19xx/20xx` token ⇒ movie, including full dates and version strings. A guard (reject year candidates embedded in date/version patterns like `YYYY.MM.DD` / `vDD.MM.YYYY`) would take movie precision from 0.40 toward 1.0 on this bucket. Candidate next fix; not authorized here.
3. No basis for adopting an external parser library: the remaining gap is matching, not extraction (see `../COMPARISON.md`).
