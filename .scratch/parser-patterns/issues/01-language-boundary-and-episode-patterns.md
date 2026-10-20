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
- Design deviation from spec: Temporada N is not parsed as a separate marker (greedy title grammar absorbs it); superseded by the review fix decoding Cap.NNN hundreds digits into season+episode (Cap.605 → S06E05), which recovers the season for Spanish forms.

## Review (two-scout, fixed point 5793f5c vs commit d0af495)

Standards: Approve. Addressed: P2 Cap.NNN season decode (hundreds digit = season, Cap.605 → S06E05); P3 saEpisode renamed standaloneEpisode; P3 dead test fields removed and season assertions added; P3 3-digit rationale comment. Deferred: P3 shared prefix-token helper and switch-case loop (token shapes diverge after decode; loop no longer clean); P3 language boundary duplication (8 boring lines).

Spec: Approve, 3×P3. Addressed: SPEC.md design amended to the Cap-hundreds decode; S03E06 season assertion added. Noted: EpisodesToken growth widens public model.ParseEpisodes semantics for bare 'Cap.NNN'/'ENNN' inputs — no non-test callers exist in the repo today.

Review-fix commit: see git log (same branch). Lint on changed files: only 2 pre-existing findings remain (date.go golines, episodes.go nolintlint — untouched files).
