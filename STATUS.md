# Current state
- parser-patterns COMPLETE: language hyphen-boundary fix + four episode forms; commits d0af495, 8af7f7a (fixed point 5793f5c); two-scout review closed (P2 Cap-decode addressed, P3 polish addressed/deferred in ticket); suites/build/lint green; 200-name sample: +17 real extractions, 0 noise, 2 false-'sa' removals. Not deployed; existing DB rows unchanged.

- Parser comparison COMPLETE (read-only): report `.scratch/classifier-parser-research/COMPARISON.md`; sample+raw outputs in `sample/` (200 names, versions in `sample/VERSIONS.txt`). Headline: current parser misses 14/17 clear episode patterns (bracket anime, Korean E376, Spanish Cap.NNN, leading/lowercase sxxexx) that all four candidates catch; candidates bring false episode guesses on numeric noise (jhin/go-ptt ~15), autobrr types 41/200 as movie (mostly wrong here), release-group extraction gap (bitmagnet 4 vs 35-81). Live-DB confirmed bitmagnet `InferLanguages` false positive ('sa' from Shikimori-san). No adoption decision; needs labeled check first.
- Harness was an isolated throwaway copy at `/tmp/bitmagnet-parser-harness` (go1.26.0 via goproxy.cn); production tree and go.mod untouched.
- Go release-parser research complete: `.scratch/classifier-parser-research/RESEARCH.md`; source/maintenance review only, no package installed or executed.
- Maintained shortlist: autobrr/rls, dreulavelle/jhin, chill-institute/torrentname and MunifTanjim/go-ptt (all MIT). ProfChaos excluded pending license.
- SF-01 complete: fresh browser reloads and manual Refresh, preserved criteria/page/order, error recovery and full search lifecycle teardown; commits 60464ce, 5ab56ea, 5793f5c; reviews closed, not deployed.
- Branch: `main`; review branch point: `236c129d745fca499252fc40167ea1d806302e51`; latest commit `5793f5c`.
- Running instance: `http://192.168.1.2:3333` (revision 236c129); access is read-only for this experiment.
- Remaining work: labeled non-LLM classification check (after comparison informs it).
- Cached Chromium: `/tmp/chromium`; no configured language server; no Git identity (per-command attribution when needed).
- SF-01 verification detail: spec/ticket complete in `.scratch/search-freshness/`; Standards/Spec reviews 0 findings; focused suite 8 passed, full suite 84 passed; lint, embedded build and built-display smoke passed; local only, not deployed.
- Architecture report: `/tmp/architecture-review-20261011-123800.html`.
