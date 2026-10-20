# Go release-name parser libraries

Research date: 2026-10-11. Scope: native Go libraries that extract media metadata from human release names, not bencoded `.torrent` files or Go source code. No library was installed, compiled or executed; no accuracy claim is established on Bitmagnet data. No production code or dependency changes.

## Recommendation

Evaluate **autobrr/rls** and **dreulavelle/jhin/parser** first. The former has ongoing scene-parser maintenance and demonstrated use of its older fork in Autobrr; the latter exposes dates, episode/season lists, ISO language codes and explicit dual-audio evidence with a parser-only import. This is a recommendation for comparative evaluation, not a claim that either is more accurate than Bitmagnet.

Bitmagnet currently declares **Go 1.23.6** in `go.mod`. None of the current maintained versions below is a drop-in dependency at that minimum. Do not upgrade the toolchain or replace the classifier solely on these findings. If retaining Go 1.23.6 is essential, evaluate `moistari/rls v0.6.0` or the older `autobrr/rls v0.8.1` snapshot with the compatibility/module-identity caveats below.

## Maintained shortlist

| Library | Verified maintenance | Version and minimum Go | License | Fit and trade-off |
|---|---|---|---|---|
| [autobrr/rls](https://github.com/autobrr/rls) | Actual parser work through 2026-09-20; dependency update 2026-10-09. Maintained fork of moistari/rls. | Latest published module tag `v0.9.0`, 2026-08-23: Go 1.25.0. Current master: Go 1.26.0. Later September fixes are not in that tag. | MIT | Broad scene-release fields: title/date, season/episode, languages, resolution/source/codec/HDR/audio/group and non-video types. Preserve Bitmagnet's own content-type policy; parser type inference is not identity matching. |
| [dreulavelle/jhin](https://github.com/dreulavelle/jhin) | Default branch and latest release `v0.8.0`, 2026-09-16. Earlier releases and parser corrections visible in changelog. Repository created 2026-07-24: genuinely recent work, short history. | `v0.8.0`: Go 1.24.1. | MIT | Parser-only subpackage; date, seasons/episodes/volumes lists, ISO languages, dual-audio flag, quality/codecs/HDR/audio/group. Ranking/filtering/rules also exist but are unnecessary for this classifier. PTT lineage, including the original MunifTanjim Go port. |
| [chill-institute/torrentname](https://github.com/chill-institute/torrentname) | Release `v1.4.4`, 2026-10-10: fixes dotted titles before release metadata. Other same-day fixes address bare-domain prefixes and false episode detections from sizes/3D tags. Created 2026-03-22: still a young project. | `v1.4.4`: Go 1.26.0, with toolchain directive go1.27.2. The directive is distinct from the minimum Go requirement. | MIT | Focused parser returning `TorrentInfo`: title/year, season/episode/end, quality/source/codec/audio/HDR/group and a string language field. Best-effort parsing leaves uncertain fields at zero values; returned error is reserved for future hard failures, not a confidence signal. |
| [MunifTanjim/go-ptt](https://github.com/MunifTanjim/go-ptt) | Release `v0.14.1`, 2026-02-16: fixes an Italian-language false positive. Earlier releases improve languages, seasons/episodes and title/site/group extraction. Last branch commit that date is a JavaScript-wrapper release; distinguish it from the actual Go parser fix. | `v0.14.1`: Go 1.24.1. | MIT | Broad fields: date, season/episode lists, language lists, source/quality/HDR/audio/codec/group. More dependencies than Jhin's advertised parser-only runtime; includes pond for parallelism and CLI/server/tooling dependencies. Multilingual marker strings are mixed into the language output and require normalization. |

### Primary maintenance and module sources

- Autobrr: [September parser feature](https://github.com/autobrr/rls/commit/7e70bd7f5dd644a5b16531f5a8d9e038837b0af1), [October dependency commit](https://github.com/autobrr/rls/commit/845dd215fabe6536be1b4a00724379f25625da49), [tag metadata](https://proxy.golang.org/github.com/autobrr/rls/@latest), [v0.9.0 go.mod](https://raw.githubusercontent.com/autobrr/rls/v0.9.0/go.mod), [master go.mod](https://raw.githubusercontent.com/autobrr/rls/master/go.mod).
- Jhin: [v0.8.0 release](https://github.com/dreulavelle/jhin/releases/tag/v0.8.0), [tagged go.mod](https://raw.githubusercontent.com/dreulavelle/jhin/v0.8.0/go.mod), [README/API reference](https://github.com/dreulavelle/jhin#readme), [parser source](https://github.com/dreulavelle/jhin/blob/main/parser/parser.go), [repository metadata](https://api.github.com/repos/dreulavelle/jhin).
- Chill: [v1.4.4 release](https://github.com/chill-institute/torrentname/releases/tag/v1.4.4), [tagged go.mod](https://raw.githubusercontent.com/chill-institute/torrentname/v1.4.4/go.mod), [public API](https://github.com/chill-institute/torrentname/blob/main/parser.go), [README](https://github.com/chill-institute/torrentname#readme), [repository metadata](https://api.github.com/repos/chill-institute/torrentname).
- Go PTT: [tag metadata](https://proxy.golang.org/github.com/!munif!tanjim/go-ptt/@latest), [changelog](https://github.com/MunifTanjim/go-ptt/blob/main/CHANGELOG.md), [actual February parser fix](https://github.com/MunifTanjim/go-ptt/commit/a313e2e8498e282c71f03878366f51d552e59843), [tagged go.mod](https://raw.githubusercontent.com/MunifTanjim/go-ptt/v0.14.1/go.mod), [README/API fields](https://github.com/MunifTanjim/go-ptt#result-reference).

“Zero runtime dependencies” for Jhin and Chill is upstream documentation language; their go.mod files still contain test, CLI or developer-tool dependencies. Module requirements and imported runtime dependencies are different questions. No builds were performed to verify an entire dependency graph.

## Older or constrained alternatives

### moistari/rls v0.6.0: compatible upstream baseline

MIT; `go 1.23.0`, `toolchain go1.24.2`. Runtime x/sync and x/text versions also declare Go 1.23.0. The toolchain directive in a dependency is not the same as a minimum requirement on an importing main module. Last upstream default-branch source commit was **2025-06-28**, fixing multiple episodes. That is useful prior maintenance, not evidence of present active development. Later issue/PR reports remain open. GitHub Releases lags tags: v0.6.0 exists as a tag, while the sole release object is v0.5.13.

`ParseString(string) Release` closely matches our title/date/language/video attributes. Use `SeriesEpisodes()` rather than scalar `Episode` for supported multi-episode forms. Some ranges/pack forms remain incomplete. Its language tags mix languages, MULTi/DL, subtitle/dub modes and other markers; they cannot all be treated as ISO language codes. Type inference can infer movie from resolution alone and should not replace Bitmagnet's cautious decision.

Sources: [v0.6.0 source](https://github.com/moistari/rls/tree/v0.6.0), [go.mod](https://raw.githubusercontent.com/moistari/rls/v0.6.0/go.mod), [multi-episode commit](https://github.com/moistari/rls/commit/f84797726c6af8d64a228d9349832002c8f33888), [API](https://raw.githubusercontent.com/moistari/rls/v0.6.0/rls.go), [dictionary](https://raw.githubusercontent.com/moistari/rls/v0.6.0/taginfo/taginfo.csv), [known subtitle-boundary issue](https://github.com/moistari/rls/issues/14).

### autobrr/rls v0.8.1: older improved compatible snapshot

Tag dated **2026-06-08**; `go 1.23.0`, `toolchain go1.24.2`. Includes AV1, bit depth, yearly seasons after 2000 and HDR10P normalization fixes. It still declares module `github.com/moistari/rls`, so direct adoption under the fork path is not straightforward: use a `replace` directive. Autobrr itself demonstrates this arrangement. It misses changes after June 8. The renamed `github.com/autobrr/rls` module starts after this tag; v0.9.0 requires Go 1.25.

Sources: [v0.8.1 go.mod](https://raw.githubusercontent.com/autobrr/rls/v0.8.1/go.mod), [HDR10P/tag commit](https://github.com/autobrr/rls/commit/9e5562401a74fbf6e2f22fc5fe2ffc2b791b8c0e), [module rename](https://github.com/autobrr/rls/commit/e298c67cda42f1666393c1ada7157101ad9e7f3a), [Autobrr adoption](https://raw.githubusercontent.com/autobrr/autobrr/develop/go.mod).

### Additional libraries

| Library | Source activity/version | Constraint |
|---|---|---|
| [ProfChaos/torrent-name-parser](https://github.com/ProfChaos/torrent-name-parser) | v0.5.1, 2025-11-21; title/year ambiguity fix and outside-contributor PR history. | Go 1.25; no explicit license found in repository metadata/tree. Do not adopt without license clarification. v0.5.0 uses Go 1.23 but does not resolve the license issue. |
| [itsrenoria/ptt-go](https://github.com/itsrenoria/ptt-go) | Two commits in February 2026; latest 2026-02-06, no tags/releases. | MIT, Go 1.21, no dependencies; broad PTT-shaped output but too little history to call sustained maintenance. |
| [cehbz/torrentname](https://github.com/cehbz/torrentname) | v1.2.1, 2025-07-12; entire development burst was one week. | MIT, Go 1.21; compact fields/confidence, singular episode/language. README episode-list example does not match source. |
| [razsteinmetz/go-ptn](https://github.com/razsteinmetz/go-ptn) | Latest default-branch change 2024-12-18 fixes slice bounds; old v1.0.0 tag predates that fix. | MIT, Go 1.19; conventional names but no date/HDR/multiple-language output. Eager IsMovie heuristic is unsuitable as authoritative classification. |
| [nssteinbrenner/anitogo](https://github.com/nssteinbrenner/anitogo) | v1.0.0, 2023-12-22; no later source activity observed. | MPL-2.0, Go 1.21.5; anime-specialist rather than general movie/TV release parser. |
| [5rahim/tanuki](https://github.com/5rahim/tanuki) | Last source change 2023-11-20; Anitogo-derived. | MPL-2.0, Go 1.21; specialist, no evidence of ongoing maintenance. |
| [peterhellberg/release](https://github.com/peterhellberg/release) | README change 2022-06-14; parser last changed 2019-12-21. | MIT, Go 1.18; too stale/narrow for maintained shortlist. |
| [middelink/go-parse-torrent-name](https://github.com/middelink/go-parse-torrent-name) | Last branch change 2019-03-01 adds go.mod. | MIT, Go 1.12; legacy library, not actively maintained. |

Exact supporting source snapshots: [ProfChaos fix](https://github.com/ProfChaos/torrent-name-parser/commit/33feb6c4a1d03322250c36cbe85b4cb7d880ccb7), [ProfChaos module](https://raw.githubusercontent.com/ProfChaos/torrent-name-parser/33feb6c4a1d03322250c36cbe85b4cb7d880ccb7/go.mod), [ProfChaos tree/license evidence](https://api.github.com/repos/ProfChaos/torrent-name-parser/git/trees/33feb6c4a1d03322250c36cbe85b4cb7d880ccb7?recursive=1), [itsrenoria source](https://github.com/itsrenoria/ptt-go/tree/4a423b13e344196ff15afead40d732fff988b06c), [cehbz source](https://github.com/cehbz/torrentname/tree/803fda3c3c111ae99bfb8ecd98eaca0719503068), [go-ptn source](https://github.com/razsteinmetz/go-ptn/tree/ed99045bc5cf7d3044f29b8b385f39450a484a2f), [Anitogo source](https://github.com/nssteinbrenner/anitogo/tree/260d546976961245b80ae7bd02f0c0b52a3ecc44), [Tanuki source](https://github.com/5rahim/tanuki/tree/0df5a84751dde5a15b62f71c45fd813027473ca6), [peterhellberg source](https://github.com/peterhellberg/release/tree/681f91c2b326b7722fbae94f0ae27301c6df05f9), [middelink source](https://github.com/middelink/go-parse-torrent-name/tree/3ff4efacd4c4f1d986926da2289f428c89971594).

## Integration seam and evaluation criteria

The relevant existing seam is `internal/classifier/parsers/video.go:ParseVideoContent`, including `ParseTitleYearEpisodes`; `internal/classifier/parsers/date.go` handles daily-date evidence separately. Current `ParseVideoContent` derives title/year/episodes first, then language/multilingual and technical video attributes, and uses known content type, episodes/date or year to decide movie/TV conservatively.

A release parser does not resolve TMDB identity, validate a torrent's actual file contents, or reliably classify arbitrary books/music/software from filenames. Keep those responsibilities separate. In particular, broad scene parsers' movie/game/music guesses must not silently replace the current policy.

Before choosing a dependency, compare our existing parser with pinned candidates on the same labeled names. Score title and year/date accuracy, season/episode sets, language vs subtitle/dub/multilingual evidence and technical attributes. Include numeric titles, daily releases, season packs/ranges, absolute anime episodes, WEB inside titles, ambiguous language words, source-before-resolution and attached codec/size/3D tokens. Reject gains that increase confident false matches. No benchmark has yet established that a dependency beats the current parser.

## Evidence limits

Maintenance was checked against commits, tagged versions, releases and substantive fixes, not GitHub updated_at or star counts. Repository age and toolchain requirements are reported separately from parser coverage. GitHub API rate limiting occurred; raw files, GitHub release pages and the public Go module proxy supplied corroborating evidence. Upstream tests/fixture corpora demonstrate intended behavior, not verified accuracy on Bitmagnet data. Research only: no library execution, no new dependency, no classifier change, no live mutation.
