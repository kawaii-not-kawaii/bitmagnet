# 01: language boundary + episode patterns

What to build: SPEC.md items — `[^\p{L}\d-]` boundaries for languagesRegex; EpisodesToken alternatives (standalone E\d{1,3}, Spanish Cap.NNN, bracket-guarded `- N [`); optional-title loose regex tried after the strict one.
Blocked by: none.
Status: done.
Acceptance:
- [x] InferLanguages("...Shikimori-san [TV-1] - 09 [720p].mkv") excludes `sa`; existing language tests pass.
- [x] ParseTitleYearEpisodes extracts episodes for `- 12 [720p]`, `E376.261011`, `Cap.605`, leading `S03E06`; ParseVideoContent → tv_show for each.
- [x] Noise regressions: `2637.zip`, `0488-【新片】...`, `51原创...` extract no episodes.
- [x] go test ./internal/model/... ./internal/classifier/... green; module build green; golangci-lint clean on changed files (2 remaining findings are pre-existing in untouched files date.go/episodes.go).

Verification evidence:
- Red→green: TestInferLanguages reproduced live-DB 'sa' before fix; 9 failing pattern subtests before implementation.
- 200-name live sample re-run (results_new.jsonl): +17 real episode extractions (0 noise names), 0 lost episodes, 17 →tv_show flips all correct, 2 language changes both removals of false 'sa'; season semantics preserved (`[Group] Cool Anime Name S4 - 02 (1080p)` classifier test green after strict-then-loose split).
- Design deviation from spec: Temporada N season is not captured (greedy title grammar absorbs it before the token); Spanish forms yield episode-only (season 0).
