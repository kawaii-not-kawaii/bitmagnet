# Parser comparison on live unknown torrents

Date: 2026-10-11 · Read-only experiment · No production change, no dependency change.

## Method

- Sample: 200 unique torrents with `contentType: null` from the running instance (`http://192.168.1.2:3333`, revision `236c129`), collected 2026-10-11T14:10Z via authenticated browser GraphQL reads with `cached` omitted (ModeNoCache).
- Strata: 100 newest by `published_at` (reflects active DHT ingestion) + 100 by `info_hash` ascending (effectively uniform over the ~4.6M unknowns). A third hash page (100) was fetched but not retained. Live `totalCount` returned 0 with `totalCountIsEstimate: false` on these no-cache searches, so the ~4,600,000 figure comes from the UI.
- Bias limits: the unknown bucket is dominated by adult content, CJK archives, games and opaque IDs; results apply to this population, not to torrents in general. No ground-truth labels were used: more populated fields ≠ better classification.
- Parsers compared on identical names (`sample/names.txt`):
  - Bitmagnet baseline: `internal/classifier/parsers` — `ParseTitleYearEpisodes`, conservative `ParseVideoContent`, and `ParseDate` as daily-date evidence.
  - `github.com/autobrr/rls v0.9.0` (commit `5050cd3d`, 2026-08-23)
  - `github.com/dreulavelle/jhin/parser v0.8.0` (commit `ee5459bb`, 2026-09-16)
  - `github.com/chill-institute/torrentname v1.4.4` (commit `27aba485`, 2026-10-10)
  - `github.com/MunifTanjim/go-ptt v0.14.1` (commit `ae293827`, 2026-02-16)
- Toolchain: go1.26.0 (auto-resolved via goproxy.cn; proxy.golang.org 403s toolchain zips and dl.google.com binaries are blocked at this host's egress). Harness: isolated repo copy at `/tmp/bitmagnet-parser-harness` (throwaway) adding `cmd/parsercompare`; the production tree and its `go.mod` were not touched.
- Raw per-name output for all five parsers: `sample/results_raw.jsonl` (200 rows, one JSON object per name).

## Extraction coverage (of 200 names)

| field | bitmagnet¹ | autobrr | jhin | chill | go-ptt |
|---|---|---|---|---|---|
| title (any parse) | 200 | 200 | 199 | 188 | 198 |
| year | 5 | 5 | 4 | 5 | 4 |
| season/episode evidence | 3 | 21 | 36 | 17 | 38 |
| resolution | 37² | 29 | 37 | 32 | 34 |
| codec | 11² | 16 | 16 | 16 | 16 |
| source/quality | 18² | 17 | 20 | 16 | 20 |
| release group | 4² | 81 | 53 | 35 | 54 |
| language | (see note) | 8 | 75 | 1 | 78 |

¹ bitmagnet_parse row = loose `ParseTitleYearEpisodes`; the classifier-facing `ParseVideoContent` keeps a title on only 8/200 and sets contentType on 8/200 (5 movie, 3 TV) — by design, since title-only matches without year/episode evidence are discarded.
² from `ParseVideoContent`/`InferVideoAttributes`.

## Findings

### 1. Episode-pattern coverage is the real gap
Of 17 names with unambiguous episode patterns, Bitmagnet matched only 3 (strict `SxxExx`): `Coven.Academy.S01E06`, `American.Dad.S19E07`, `Marvel.Studios.Legends.S02E19`. All four candidates caught 14–17, including forms Bitmagnet misses:
- anime bracket episode: `[ASW] ... - 12 [1080p HEVC]`, `[SubsPlease] ... - 11 (720p)` (all four candidates)
- Korean daily variety: `사장님 귀는 당나귀 귀.E376.261011.720p-NEXT.mp4`, `전지적 참견 시점.E420...` (autobrr, jhin, go-ptt; chill misses)
- Spanish `Cap.NNN`: `Peaky Blinders ... [Cap.605]`, `Privilegios [HDTV][Cap.105]`, `Good Trouble ... [Cap.211_214]` (jhin + go-ptt only)
- pattern at string start: `S03E06 - Family Guy - 306 ...` (all four; Bitmagnet's regex needs a preceding title)
- lowercase `s15e13`: `bobross-bobross-s15e13-1080p` (all four)

### 2. Higher coverage buys false episode guesses
On clearly non-episode names (numeric noise, archives, image counts) jhin and go-ptt hallucinate episodes: `51原创...`→ep 51, `0488-【新片】...`→488, `272【欢乐谷...】`→272, `2637.zip`→2637, `HQ - 3D - Landscape ... 35 шт`→35 (go-ptt/autobrr: 3200), `all_lady_do_it[576-312]...`→576, `[Naizou ...] (32) ... .zip`→32. Counting on non-video rows: jhin 15, go-ptt 17 episode guesses vs autobrr 2, chill 2 — and most of jhin/go-ptt's are false. Any adoption needs a numeric-noise guard (e.g. require episode evidence alongside a season marker or known pattern).

### 3. Autobrr's `Type` is default-to-movie — unfit for Bitmagnet's policy
autobrr typed 41/200 as movie on this bucket; eyeballing them, many are wrong: a tennis match (`Darya Kasatkina - Gil Belen Teichmann`), adult clips/IDs (`欧美wife.mp4`, `KMVR-426.mp4`, `SGA-012 .avi`), a Spanish TV episode (`Peaky Blinders ... Cap.605`), a Polish-lektor movie rip with `.mts` extension taken as group. Its `episode` typing (21) is mostly accurate, and `app` (4/4), `game` (2, one false: anime `[orion origin] Mahoutsukai Reimeiki [06]`), `music`, `education` flags are genuinely useful signals. Conclusion: use autobrr for tokens, not for typing; keep Bitmagnet's conservative gating.

### 4. Languages
- jhin/go-ptt return ISO codes largely via script inference: zh 53, ja 18, ru 13 from CJK/Cyrillic presence — useful hints but inference, not release evidence.
- autobrr returns noisy non-ISO values: `MULTiSUB`, `POLiSH`, `RUSSiAN`, `CHS`, `SUB` — matches RESEARCH.md's caveat.
- chill: 1 value total.
- Bitmagnet's own `InferLanguages` produced the DB's false `sa` (Sanskrit) on `[Anistar.org] Kawaii dake ja Nai Shikimori-san [TV-1] - 09` — substring match inside "Shikimori-**sa**n"; all four candidates correctly returned nothing. This is a live-DB-confirmed Bitmagnet false positive, independent of any adoption decision.

### 5. Release group extraction is the biggest technical-attr gap
autobrr 81 / go-ptt 54 / jhin 53 / chill 35 vs Bitmagnet's 4. Caveat: ~a third of autobrr's groups are noise (`595` from `MXGS-595`, `mts` extension, `072` from `voss-072`); go-ptt/jhin are cleaner.

### 6. Opaque adult IDs (`MXGS-595` style)
Bitmagnet, jhin, go-ptt keep the full ID as title; autobrr splits into title `MXGS` + group `595`; chill truncates to `MXGS`, silently losing the number. For this bucket, ID preservation (bitmagnet/jhin/go-ptt behavior) is the safer default.

### 7. Resolution/codec/source values agree
Where both Bitmagnet and a candidate extracted a resolution, values agreed after normalization (V-prefix stripping) in 100% of the overlap (28/28 vs autobrr). Candidates add group + occasional extra catches (autobrr caught `4320p/8K` on the VR row), not better values.

## Assessment

- **Useful extraction the current parser lacks:** bracket/Korean/Spanish episode forms, lowercase/leading `sxxexx`, release groups, app/game flags (autobrr).
- **Obvious false guesses to guard against:** numeric-noise episodes (jhin/go-ptt ≈15 on this sample), default-to-movie typing (autobrr, 41 labels mostly wrong here), ID truncation (chill), substring language matching (Bitmagnet's own `sa`).
- **No accuracy claims:** no labels; a populated field is not a correct field. Bitmagnet's conservative output (8/200 typed) is policy, not failure — most of this bucket is genuinely not movies/TV.
- **If adoption is ever approved**, the evidence favors jhin or go-ptt for episode/language/group extraction behind Bitmagnet's existing gating, plus autobrr-style `Cap.NNN` support being the one pattern go-ptt handles and chill doesn't need to. That decision needs the labeled non-LLM classification check first (already in TODO.md).

## Reproduce

```
# isolated copy (throwaway): /tmp/bitmagnet-parser-harness
GOTOOLCHAIN=auto GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct go get \
  github.com/autobrr/rls@v0.9.0 github.com/dreulavelle/jhin/parser@v0.8.0 \
  github.com/chill-institute/torrentname@v1.4.4 github.com/MunifTanjim/go-ptt@v0.14.1
go build ./cmd/parsercompare
./parsercompare sample/names.txt sample/results_raw.jsonl
```

## Postscript (2026-10-11)

The recommendation was executed natively (no library adopted): commits `d0af495` + `8af7f7a` fix the `InferLanguages` hyphen false positive and add the bracket/E-prefix/Cap.NNN/leading-`SxxExx` episode forms with numeric-noise guards. On this same 200-name sample: +17 real extractions (0 noise names affected), 17 unknown→tv_show, 2 false-`sa` removals. Re-run output: `sample/results_new.jsonl`.
