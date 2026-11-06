# SPEC: year-heuristic guard

Status: done

Evidence: `.scratch/classifier-parser-research/labeled/LABELED-CHECK.md` (200 hand-labeled unknown-bucket names).

## Problem Statement

When a torrent arrives with no known content type, the video parser treats any delimited `18xx`/`19xx`/`20xx` token as a release year, and a release year alone makes the torrent a `movie`. Full dates and version strings contain such tokens, so non-movies get typed as movies and are then searched against TMDB under a wrong title and year. On the labeled set this is the only measured weakness left in the parser: all 3 false movie classifications come from it (`Met-Art 2019-05-25`, `[Pixiv] … 2026.09.06.zip`, `Raining Blood Demo v14.02.2023.rar`), which holds movie precision at 0.40.

## Solution

The parser stops accepting a year candidate that is part of a larger date or version token. A 4-digit year still counts when it stands alone between delimiters (`Movie (2000)`, `Movie.2000.1080p`). When it is attached to month/day parts (`2019-05-25`, `2026.09.06`, `25.05.2019`) or sits inside a version (`v14.02.2023`, `v1.2.2020`), it is not a release year, so the name gets no year and is not typed as a movie by this rule.

## User Stories

1. As an operator, I want date-stamped adult and photo-set releases not to be typed as movies, so that my movie category isn't polluted.
2. As an operator, I want versioned software/game releases (`v14.02.2023`) not to be typed as movies, so that they stay available to the rules, the LLM and (later) armiger for a correct type.
3. As an operator, I want genuine movie names with a year (`Title (2000)`, `Title.2000.1080p.BluRay`) to keep classifying exactly as today, so that the fix costs no recall.
4. As an operator, I want a year followed by resolution or source tokens (`Title 2019 1080p`) to keep working, so that common scene names are unaffected.
5. As an operator, I want two-years-in-a-name cases (`Blade Runner 2049 (2017)`) to behave as they do today, so that the change is narrowly scoped.
6. As an operator, I want TMDB lookups to stop running with a bogus year taken from a date, so that wasted or wrong matches go away.
7. As a maintainer, I want the rule expressed in the parser's existing token grammar, so that it reads alongside the episode and title tokens.
8. As a maintainer, I want the three labeled false positives pinned as regression cases, so that the measured failure can't return silently.
9. As a maintainer, I want the labeled-set metrics re-run after the change, so that movie precision is measured, not assumed.

## Implementation Decisions

- Change is confined to the video parser's year token (the `year` named group used by the title+year expression). No change to content-type policy in `ParseVideoContent`: a valid year still implies `movie` when no other evidence exists.
- A year candidate is rejected when it is immediately adjacent (through `.`, `-`, `/` or `_`) to 1–2 digit numeric parts forming a date (`YYYY.MM.DD`, `YYYY-MM-DD`, `DD.MM.YYYY`, `MM-DD-YYYY`), or when the token containing it starts with a version prefix (`v` followed by digits and dots).
- Behaviour for a lone year at the end of the name or before quality tokens is unchanged.
- The separate full-date handling (`parse_date` action and the "valid date ⇒ tv_show" daily-show rule) is not changed by this spec.

## Testing Decisions

- Seam: the parser's public entry points `ParseTitleYearEpisodes` and `ParseVideoContent`, table-driven like the existing episode-pattern and numeric-noise tests in the video parser test file. Assert observable output only (title, year, content type), never regex internals.
- Required cases: the three labeled false positives (no year, not `movie`); positive controls `The Regular Movie (2000)`, `Title.2000.1080p.BluRay.x264`, `Title 2019 1080p`; the existing `TestClassifier` workflow cases must stay green.
- Re-run the labeled metrics (the classifier-parser-research scripts) and record the new movie precision/recall next to the 0.154 / 0.40 baseline.

## Out of Scope

- Recovering bare-title movies with no year (needs TMDB matching or armiger, not the parser).
- Changing the date ⇒ tv_show daily-show rule or the `parse_date` action.
- Any external parser library.

## Further Notes

- In the full workflow, `parse_date` runs before video parsing and a valid full date currently pushes untyped video names toward `tv_show`; if the labeled re-run shows date names moving from `movie` to `tv_show` rather than to untyped, record it as a follow-up rather than widening this spec.

## Comments

2026-11-06: implemented in `internal/classifier/parsers/video.go` as a post-match check in `parseTitleYear` (`isYearInDateOrVersion`): the matched year is rejected when the text before it ends with `D.`/`DD.` (any of `. - / _`) or a `vN.N.` version prefix, or the text after it starts with `.D`/`-DD` etc. A rejected year falls through to the title-only parse, so no year and no movie typing.

- Tests: `TestParseTitleYearDateAndVersionGuard` (3 labeled false positives plus date/version variants must have no year and not be `movie`; positive controls `The Regular Movie (2000)`, `Title.2000.1080p.BluRay.x264`, `Title 2019 1080p`, `Title 2019`, `Blade Runner 2049 (2017) 1080p`). Confirmed red before the fix (7 failures, positives green), green after. `go test ./...` passes.
- Labeled re-run (`../classifier-parser-research/labeled/LABELED-CHECK.md`, "Year-guard re-run"): movie precision 0.400 to 1.000, recall 0.154 unchanged; tv 0.526 / 1.000 unchanged; only the three target rows changed.
- Deviation: implemented as a post-match check rather than in the rex token grammar, since Go regexp has no lookaround. Only the first year match is checked; a name with a date followed by a later lone year is not retried.
- Follow-up (not done): parser-only measurement; the full workflow's `parse_date` may still type full-date names as `tv_show`.

Review (efad6ee): the first version treated a single 1–2 digit number before the year as a date part, which dropped the year from sequel names (`Apollo.13.1995`, `District.9.2009`) and treated `.10bit` after a year as a month. Dates now need two numeric parts on one side; regression cases for both added. Labeled-set result unchanged.
