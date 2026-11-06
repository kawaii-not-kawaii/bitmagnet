# LLM vs armiger on the labeled set

Row 90 excluded from all systems (never sent to a third-party API). n = 199; 158 rows with a definite label. LLM = bitmagnet's real `openai.Classify` (default prompt, content types from llm-bench) via OpenRouter free tier, MaxTokens 4096.

## Overall

| system         | accuracy (labeled) | errors/empty |
| -------------- | ------------------ | ------------ |
| armiger        | 0.892              | 0            |
| super-single   | 0.930              | 2            |
| super-batch25  | 0.899              | 0            |
| super-batch100 | 0.956              | 0            |

## Split by armiger confidence (high ≥0.8: 107 rows, low <0.8: 92 rows)

| system         | accuracy on high | accuracy on low |
| -------------- | ---------------- | --------------- |
| armiger        | 1.000            | 0.717           |
| super-single   | 0.980            | 0.850           |
| super-batch25  | 0.939            | 0.833           |
| super-batch100 | 1.000            | 0.883           |

## Hybrid: armiger when confidence ≥ T, else LLM

| LLM            | T   | accuracy | LLM calls |
| -------------- | --- | -------- | --------- |
| super-single   | 0.6 | 0.943    | 54        |
| super-single   | 0.7 | 0.949    | 70        |
| super-single   | 0.8 | 0.943    | 92        |
| super-single   | 0.9 | 0.943    | 121       |
| super-batch25  | 0.6 | 0.937    | 54        |
| super-batch25  | 0.7 | 0.943    | 70        |
| super-batch25  | 0.8 | 0.937    | 92        |
| super-batch25  | 0.9 | 0.937    | 121       |
| super-batch100 | 0.6 | 0.956    | 54        |
| super-batch100 | 0.7 | 0.956    | 70        |
| super-batch100 | 0.8 | 0.956    | 92        |
| super-batch100 | 0.9 | 0.956    | 121       |

## Per class (recall / precision, all 199 rows; `other` rows count as false positives)

| class    | n   | armiger     | super-single | super-batch25 | super-batch100 |
| -------- | --- | ----------- | ------------ | ------------- | -------------- |
| movie    | 13  | 0.92 / 0.60 | 1.00 / 0.48  | 0.92 / 0.60   | 1.00 / 0.59    |
| tv_show  | 38  | 0.95 / 0.86 | 0.97 / 0.90  | 0.95 / 0.92   | 0.97 / 0.97    |
| xxx      | 69  | 0.88 / 0.94 | 0.97 / 0.86  | 0.99 / 0.76   | 0.99 / 0.99    |
| comic    | 12  | 1.00 / 0.44 | 0.83 / 0.56  | 0.58 / 0.70   | 1.00 / 0.44    |
| ebook    | 6   | 1.00 / 0.40 | 0.50 / 0.60  | 0.33 / 0.50   | 0.50 / 0.50    |
| game     | 13  | 0.62 / 0.80 | 0.77 / 0.91  | 0.85 / 0.69   | 0.92 / 0.71    |
| music    | 4   | 0.75 / 0.43 | 1.00 / 0.57  | 0.75 / 0.50   | 0.75 / 0.38    |
| software | 3   | 1.00 / 0.33 | 1.00 / 0.33  | 1.00 / 0.21   | 1.00 / 0.27    |

super-single latency: median 5.9s, p95 13.3s

super-batch25 latency: median 63.3s, p95 179.5s

super-batch100 latency: median 142.5s, p95 152.3s

## Rows where armiger was low-confidence: gold / armiger / LLMs

- [2] gold `xxx` · armiger `movie` 0.53 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — 欧美wife.mp4
- [4] gold `tv_show` · armiger `tv_show` 0.59 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — БесогонTV
- [11] gold `game` · armiger `movie` 0.34 · super-single:movie / super-batch25:game / super-batch100:game — Gunman.Contracts.Stand.Alone
- [13] gold `music` · armiger `music` 0.73 · super-single:music / super-batch25:music / super-batch100:music — [Saji] T.M.Revolution - RAIMEI.rar
- [16] gold `xxx` · armiger `xxx` 0.66 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — 0488-【新片】麻豆传媒 苏畅纯子韩语嫣 处男转生乱伦家庭无尽做爱 下集大年初三操逼拜年庆团圆
- [17] gold `xxx` · armiger `xxx` 0.70 · super-single:tv_show / super-batch25:xxx / super-batch100:xxx — kabukichogirls-12-HDwhole-RMVB.rmvb
- [25] gold `xxx` · armiger `tv_show` 0.35 · super-single:movie / super-batch25:software / super-batch100:xxx — ZGSHLWAYP1[9V1.8G]
- [26] gold `xxx` · armiger `xxx` 0.46 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — Empty_Balls_for_Empty_Card_MileySin_8K_original_p_VRixxens_video_180_L
- [27] gold `tv_show` · armiger `tv_show` 0.75 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — [orion origin] Mahoutsukai Reimeiki [06] [1080p+] [H265 AAC] [GB].mkv
- [31] gold `movie` · armiger `movie` 0.59 · super-single:movie / super-batch25:movie / super-batch100:movie — [QTS] Theatrical feature Martian Successor Nadesico - The prince of da
- [38] gold `xxx` · armiger `xxx` 0.78 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — Met-Art 2019-05-25
- [42] gold `music` · armiger `music` 0.55 · super-single:music / super-batch25:music / super-batch100:music — Louna - Клипы
- [45] gold `xxx` · armiger `xxx` 0.64 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — HBAD-274 性感老师学生没法正常上课了.XLSub.srt
- [48] gold `xxx` · armiger `software` 0.43 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — emilyaddisonts092712-p01
- [49] gold `game` · armiger `game` 0.43 · super-single:game / super-batch25:game / super-batch100:game — Hades.II-InsaneRamZes
- [50] gold `tv_show` · armiger `tv_show` 0.34 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — Звёзды. 3 сезон
- [53] gold `tv_show` · armiger `tv_show` 0.63 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — Kyuuketsuki Sugu Shinu - AniLibria.TV [WEBRip 1080p]
- [54] gold `xxx` · armiger `xxx` 0.75 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — 272【欢乐谷原创】被捆绑放置在泳池别墅里尤物以及深度尺度调教完整版 调教SM
- [55] gold `game` · armiger `software` 0.38 · super-single:xxx / super-batch25:xxx / super-batch100:game — Nympho_Tamer
- [58] gold `xxx` · armiger `music` 0.60 · super-single:xxx / super-batch25:xxx / super-batch100:comic — Caticornplay - Lucy.zip
- [62] gold `game` · armiger `game` 0.39 · super-single:game / super-batch25:game / super-batch100:game — Princess Survivors.zip
- [64] gold `game` · armiger `software` 0.68 · super-single:game / super-batch25:game / super-batch100:game — Arma*Reforger_ZoS_1.8*[Release]
- [69] gold `xxx` · armiger `music` 0.48 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — PoppaChan - Nanyong Yu 15Videos.zip
- [70] gold `tv_show` · armiger `comic` 0.25 · super-single:ebook / super-batch25:ebook / super-batch100:ebook — Sekai Saikyou no Majo, Hajimemashita
- [73] gold `tv_show` · armiger `tv_show` 0.53 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — [异域-11番小队][妄想学生会合集 Seitokai Yakuindomo][BDRIP][720P][X264-10bit_AAC]
- [76] gold `movie` · armiger `movie` 0.47 · super-single:movie / super-batch25:xxx / super-batch100:movie — all_lady_do_it[576-312](1h32m)(Tinto_brass)
- [77] gold `tv_show` · armiger `tv_show` 0.78 · super-single:tv*show / super-batch25:tv_show / super-batch100:tv_show — (tokusatsu.org)Kamen_Rider_Zeztz*-\_Series_of_Sister's_Substory_Agent_M
- [78] gold `movie` · armiger `movie` 0.78 · super-single:movie / super-batch25:movie / super-batch100:movie — chas*pik_1.46*[torrents.ru]\_by_bm11.avi
- [82] gold `ebook` · armiger `ebook` 0.59 · super-single:movie / super-batch25:movie / super-batch100:movie — Анклавы
- [86] gold `xxx` · armiger `xxx` 0.75 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — 重金购买【网红-Ria 久留美】全网首发【银发天使-艾米莉娅】之【两个仆人来抚摸湿透、颤抖的阴部】用力操她，举到空中，她反复高潮，直到她失去
- [94] gold `ebook` · armiger `ebook` 0.69 · super-single:audiobook / super-batch25:audiobook / super-batch100:music — Герман Горшенев_Звездная Кровь Архераил
- [104] gold `game` · armiger `movie` 0.44 · super-single:game / super-batch25:game / super-batch100:game — Call_of_Beyond
- [110] gold `ebook` · armiger `ebook` 0.70 · super-single:music / super-batch25:music / super-batch100:music — Цикл - «Ал»
- [111] gold `game` · armiger `game` 0.78 · super-single:game / super-batch25:game / super-batch100:game — Backpack.Survivors.rar
- [116] gold `music` · armiger `xxx` 0.36 · super-single:music / super-batch25:software / super-batch100:software — BFD Stanton Moore Grooves
- [117] gold `xxx` · armiger `xxx` 0.55 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — Taboo Charming Mother
- [118] gold `xxx` · armiger `xxx` 0.67 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — GUT-Jukuyo-Series_1 01 by RipeSluts.mp4
- [120] gold `game` · armiger `game` 0.76 · super-single:game / super-batch25:game / super-batch100:game — TERRARIA (XBOX360) RF SOPARAXBOX360JOGOSTORRENT.COM
- [123] gold `xxx` · armiger `course` 0.64 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — TMG012到府男私教强奸女学员-莉莉
- [125] gold `tv_show` · armiger `tv_show` 0.51 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — Heartbreak High
- [128] gold `xxx` · armiger `xxx` 0.37 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — PSA_Candy_Alexa_SD.mp4
- [138] gold `music` · armiger `music` 0.26 · super-single:music / super-batch25:music / super-batch100:music — Wale - Clappers
- [144] gold `xxx` · armiger `tv_show` 0.36 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — 【牛仔裤白鞋踩踏】.mp4
- [146] gold `tv_show` · armiger `tv_show` 0.60 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — [uni] Kirameki Project
- [147] gold `xxx` · armiger `xxx` 0.73 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — MARKED Nyssa Nevers, Sister Dee
- [152] gold `tv_show` · armiger `tv_show` 0.73 · super-single:tv_show / super-batch25:movie / super-batch100:tv_show — Koutetsu Tenshi Kurumi RUS
- [153] gold `movie` · armiger `ebook` 0.30 · super-single:movie / super-batch25:movie / super-batch100:movie — ЖИВАЯ СТАЛЬ 1-2
- [154] gold `game` · armiger `ebook` 0.40 · super-single:movie / super-batch25:movie / super-batch100:movie — Mercenaries
- [158] gold `tv_show` · armiger `tv_show` 0.74 · super-single:tv*show / super-batch25:tv_show / super-batch100:tv_show — Hitsugi_no_Chaika*[AniMedia.TV]
- [160] gold `xxx` · armiger `xxx` 0.75 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — voss-072
- [161] gold `ebook` · armiger `ebook` 0.74 · super-single:ebook / super-batch25:ebook / super-batch100:ebook — Конофальский Борис
- [162] gold `tv_show` · armiger `tv_show` 0.57 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — Discovery.Austin.Stevens.Adventures.Night.of.the.Hyena.HDTV.x264.720p.
- [166] gold `xxx` · armiger `xxx` 0.80 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — Big.Wet.Butts.5
- [167] gold `movie` · armiger `movie` 0.79 · super-single:movie / super-batch25:movie / super-batch100:movie — Anatolij Papanov.avi
- [172] gold `game` · armiger `game` 0.65 · super-single:game / super-batch25:game / super-batch100:game — DRIVING GAMES
- [174] gold `movie` · armiger `movie` 0.73 · super-single:movie / super-batch25:movie / super-batch100:movie — Blind Fury (Гаврилов, MVO Видеосервис, DUB Варус, MVO)
- [175] gold `xxx` · armiger `comic` 0.39 · super-single:xxx / super-batch25:xxx / super-batch100:xxx — [無碼]七海
- [186] gold `tv_show` · armiger `ebook` 0.25 · super-single:tv_show / super-batch25:tv_show / super-batch100:tv_show — The Amazing World of Gumball
- [194] gold `ebook` · armiger `ebook` 0.57 · super-single:ebook / super-batch25:ebook / super-batch100:ebook — Carl Hiaasen - Team Rodent-How Disney Devours the World.htm
- [199] gold `movie` · armiger `movie` 0.69 · super-single:movie / super-batch25:movie / super-batch100:movie — How to Get Ahead in Advertising DVD-5_Criterion#120
