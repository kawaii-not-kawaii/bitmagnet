# SPEC: parser episode patterns + language boundary

Compressed spec (small bugfix + pattern addition; grilling compressed to this plan).

## Problem (evidence: .scratch/classifier-parser-research/COMPARISON.md, 200-name live sample)

1. `model.InferLanguages` matches language tokens with `[^\p{L}\d]+` boundaries that treat `-` as a delimiter, so hyphenated fragments match: live DB shows `sa` (Sanskrit) on `[Anistar.org] ... Shikimori-san ...` via alpha3 `san`. Class-level problem (also `fin`, `cat`, `slo` style alpha3 collisions), not just "san".
2. `parsers.ParseTitleYearEpisodes` catches only season-prefixed `SxxExx`/`xYY`/`season N` forms. Misses four evidence-backed patterns on unknown torrents: bracket anime `- 12 [720p]`, Korean standalone `E376`, Spanish `Cap.605` (+`Temporada 6`), and leading `S03E06 - Title`.

## Non-goals

- No new parser dependency; no classifier policy change (conservative gating in `ParseVideoContent` stays: episodes/year evidence only).
- Not fixing hyphen-glued lowercase `bobross-s15e13` (title grammar treats `-` as a word joiner; changing that risks broad regressions). Disclosed limitation.
- No reclassification of existing data; no production run.

## Seams (tests anchor here, public interfaces only)

- `model.InferLanguages(input) Languages`
- `parsers.ParseTitleYearEpisodes(NullContentType{}, name)` and `parsers.ParseVideoContent(torrent, result)` for the new evidence forms.

## Design

- `internal/model/language.go`: build `languagesRegex` with boundaries `[^\p{L}\d-]` (hyphen is NOT a language-token delimiter). Case-insensitivity and token set unchanged. Keep capture-group shape for `InferLanguages` indices.
- `internal/model/episodes_parser.go`: extend `EpisodesToken` composite with three guarded alternatives (ordered after existing forms):
  - standalone episode `[eE]\p{.}?\d{1,3}` (explicit letter prefix keeps numeric noise out; covers Korean `E376.261011`),
  - Spanish `Cap.MMM` (optionally one bracketed technical group before it); the hundreds digit encodes the season (`Cap.605` → S06E05, bare `Cap.105` → S01E05) — this replaces capturing a separate `Temporada N` marker, which the greedy title grammar absorbs first,
  - bracket episode `- N{1,3} [` / `- N{1,3} (` (digits immediately followed by bracket; excludes Chinese prose numbers and fullwidth brackets).
- `internal/classifier/parsers/video.go`: `titleEpisodesRegex` title part becomes optional so leading `S03E06 - Family Guy` yields episodes with empty title (title-only results stay discarded by `ParseVideoContent` policy).
- Known false-positive ceilings marked with `ponytail:` comments where a guard is heuristic.

## Acceptance

- New tests: `TestInferLanguages` case `Shikimori-san [TV-1] - 09` → no `sa` (and no other language); episode-pattern tests for the four forms at the `ParseTitleYearEpisodes`/`ParseVideoContent` seams; numeric-noise regression cases (`2637.zip`, Chinese prose, `0488-`) still yield no episodes.
- Full `internal/model` + `internal/classifier/parsers` suites pass; changed packages build; existing tests unmodified except appending cases.
