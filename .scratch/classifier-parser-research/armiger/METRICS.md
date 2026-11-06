# armiger-ultra on the 200-name labeled set

Model: ghcr.io/spencernorton/armiger:3.12.2 (baked bundle, update channel off, sharing off), on tower.
Input per item: torrent name, total size, and a one-entry file list for single-file torrents (the sample has no multi-file paths). Labels: `labeled/labels.json`; `other` rows are excluded from accuracy but count as false positives when predicted as a class.

Overall category accuracy on 159 non-`other` rows: **0.887**

| class    | n   | tp  | fp  | recall | precision |
| -------- | --- | --- | --- | ------ | --------- |
| comic    | 12  | 12  | 15  | 1.000  | 0.444     |
| ebook    | 6   | 6   | 9   | 1.000  | 0.400     |
| game     | 13  | 8   | 2   | 0.615  | 0.800     |
| movie    | 13  | 12  | 8   | 0.923  | 0.600     |
| music    | 4   | 3   | 4   | 0.750  | 0.429     |
| software | 3   | 3   | 6   | 1.000  | 0.333     |
| tv_show  | 38  | 36  | 6   | 0.947  | 0.857     |
| xxx      | 70  | 61  | 4   | 0.871  | 0.938     |

## movie / tv_show vs bitmagnet parser (post-fix 8af7f7a)

| class   | armiger recall/precision | bitmagnet recall/precision |
| ------- | ------------------------ | -------------------------- |
| movie   | 0.923 / 0.600            | 0.154 / 0.400              |
| tv_show | 0.947 / 0.857            | 0.526 / 1.000              |

Predictions on the 41 `other` rows: {'game': 2, 'comic': 13, 'ebook': 6, 'course': 3, 'tv_show': 4, 'software': 3, 'xxx': 3, 'movie': 5, 'music': 2}

## Confusions (gold -> predicted, count)

- game -> movie: 2
- xxx -> tv_show: 2
- game -> software: 2
- xxx -> music: 2
- xxx -> course: 2
- xxx -> movie: 1
- xxx -> software: 1
- tv_show -> comic: 1
- music -> xxx: 1
- movie -> ebook: 1
- game -> ebook: 1
- xxx -> comic: 1
- tv_show -> ebook: 1

## Misclassified rows

- [2] gold `xxx` -> `movie` (0.53): 欧美wife.mp4
- [11] gold `game` -> `movie` (0.34): Gunman.Contracts.Stand.Alone
- [25] gold `xxx` -> `tv_show` (0.35): ZGSHLWAYP1[9V1.8G]
- [48] gold `xxx` -> `software` (0.43): emilyaddisonts092712-p01
- [55] gold `game` -> `software` (0.38): Nympho_Tamer
- [58] gold `xxx` -> `music` (0.60): Caticornplay - Lucy.zip
- [64] gold `game` -> `software` (0.68): Arma*Reforger_ZoS_1.8*[Release]
- [69] gold `xxx` -> `music` (0.48): PoppaChan - Nanyong Yu 15Videos.zip
- [70] gold `tv_show` -> `comic` (0.25): Sekai Saikyou no Majo, Hajimemashita
- [90] gold `xxx` -> `course` (0.24): 大陆迷奸14岁小女孩
- [104] gold `game` -> `movie` (0.44): Call_of_Beyond
- [116] gold `music` -> `xxx` (0.36): BFD Stanton Moore Grooves
- [123] gold `xxx` -> `course` (0.64): TMG012到府男私教强奸女学员-莉莉
- [144] gold `xxx` -> `tv_show` (0.36): 【牛仔裤白鞋踩踏】.mp4
- [153] gold `movie` -> `ebook` (0.30): ЖИВАЯ СТАЛЬ 1-2
- [154] gold `game` -> `ebook` (0.40): Mercenaries
- [175] gold `xxx` -> `comic` (0.39): [無碼]七海
- [186] gold `tv_show` -> `ebook` (0.25): The Amazing World of Gumball

Latency per request over SSH tunnel (sequential): median 627 ms, p95 646 ms, max 657 ms.

## Notes (2026-11-06)

- The request latency above is mostly SSH-tunnel overhead. Measured on tower itself it is **~8–12 ms per request**, and the container uses ~470 MiB RSS at idle-after-run.
- armiger never abstains: it predicted `unknown` 0 times in 200. Many of the 41 `other` "false positives" are reasonable calls our label set had no class for: AI-generated CG/image packs → `comic`, a Udemy course → `course`, `Raining Blood Demo v14.02.2023.rar` → `game`. So comic/ebook/course precision above is understated.
- Confidence threshold tradeoff (all 200 rows, counting `other` as wrong): ≥0.6 types 145 at 0.862 precision; ≥0.8 types 107 at 0.916; ≥0.95 types 55 at 0.982.
- Safety: row 90 (a name explicitly describing abuse of a minor) came back `csam_safety.label = safe`, `is_safe = true`. It was only held back from indexing by the out-of-scope gate (`overall_safe_to_index = false`). Do not rely on its CSAM head as a filter.
- Run setup: container `armiger-ultra-eval` on tower, bound to 127.0.0.1:18096 (Jellyfin owns 8096), `ARMIGER_UPDATE_CHANNEL=off`, sharing off (status page reported 0 outbound bytes). Script: `/tmp/armiger-eval-scripts/run_eval.py` (not in repo).
