# Addendum: LLM Classifier Action — Technical Details

## Provider Interface

```go
package llm

import "context"

type Provider interface {
    Name() string
    Classify(ctx context.Context, input ClassifyInput) (*ClassifyResult, error)
    BatchClassify(ctx context.Context, inputs []ClassifyInput) ([]ClassifyResult, error)
}

type ClassifyInput struct {
    Name  string
    Files []string
}

type ClassifyResult struct {
    ContentType     string  // "movie", "tv_show", etc.
    Title           string
    Year            int
    Season          *int
    Episode         *int
    Language        *string
    VideoResolution *string
    VideoSource     *string
    VideoCodec      *string
    ReleaseGroup    *string
    Tags            []string
}
```

## OpenAI-Compatible Client

The primary provider implementation calls `/v1/chat/completions` with:

```json
{
  "model": "gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "response_format": { "type": "json_object" },
  "temperature": 0.1,
  "max_tokens": 256
}
```

`response_format: json_object` is supported by llama.cpp's OpenAI-compatible endpoint. Temperature 0.1 for deterministic output.

## Prompt Template (Default)

```
System: You are a BitTorrent content classifier. Given a torrent name and optional
file list, determine the content type and extract metadata.

Available content types: movie, tv_show, music, ebook, comic, audiobook, game, software, xxx

Rules:
- Use the filename structure to determine content type
- Look for patterns like S01E01, season/episode markers for tv_show
- Look for years (1900-2099) to identify movies
- Return ONLY valid JSON matching the schema

Examples:
Input: "The.Matrix.1999.1080p.BluRay.x264-GROUP"
Output: {"content_type": "movie", "title": "The Matrix", "year": 1999,
         "video_resolution": "V1080p", "video_source": "BluRay",
         "video_codec": "x264", "release_group": "GROUP"}

Input: "Better.Call.Saul.S06E03.720p.WEB.h264-NTb"
Output: {"content_type": "tv_show", "title": "Better Call Saul",
         "year": 2022, "season": 6, "episode": 3,
         "video_resolution": "V720p", "video_source": "WEB",
         "video_codec": "h264", "release_group": "NTb"}

User torrent:
Name: {{.Name}}
{{if .Files}}Files:
{{range .Files}}  {{.}}
{{end}}{{end}}
```

## Classifier Action Integration

New file: `internal/classifier/action_llm_classify.go`

The action is registered in the defaults list at `internal/classifier/features.go`. It follows the same pattern as `attach_tmdb_content_by_search`:

1. `compileAction` — validates provider config from YAML source
2. `run` — extracts torrent info, builds prompt, calls provider, validates response, sets result attributes

The action is placed AFTER existing match actions in the workflow and BEFORE `unmatched`. Only fires if `!result.ContentType.Valid`.

## Batching Strategy

When `batch_size > 1`, the action collects inputs into a slice. After N inputs accumulate (or a flush timer expires), the batch is sent as:

```json
{
  "torrents": [
    {"name": "...", "files": [...]},
    {"name": "...", "files": [...]}
  ]
}
```

And the response is an array of classification results in the same order. Each result is independently validated against enums.

## Cache Schema

Cache key format: `llm_classify:{hex(sha256(normalized))}`

Where `normalized` = toLower(removeNonAlphanumeric(name)).

Value stored as JSON of `ClassifyResult` plus `cached_at` timestamp and `ttl` duration.

## Configuration Wiring

The provider config is loaded from the classifier's existing YAML source chain (core + user overrides). No new global config struct needed — the action reads from `classifier.llm.providers` in the YAML config.

The YAML schema extension for `llm_classify`:

```yaml
workflows:
  default:
    - find_match:
      - parse_video_content
    - if_else:
        condition: "result.hasContentType"
        if_action: unmatched
        else_action:
          - llm_classify:
              providers: [gemma4]
              batch_size: 5
              cache_ttl: 720h
    - attach_tmdb_content_by_search: {}
```

## Target Server Config

On `miniscoffee` (100.125.213.44), the Gemma 4 26B QAT service is defined in:

`/etc/systemd/system/gemma-26b-mtp.service`

```ini
ExecStart=/opt/lemonade/llama-b9549/llama-server \
  -m /opt/lemonade/models/gemma-4-26B-A4B-qat-mtp/gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf \
  --port 8082 --host 0.0.0.0 -ngl 99 -fa \
  -ctk q8_0 -ctv q8_0 --mlock --no-mmap
```

The endpoint will be reachable at `http://100.125.213.44:8082/v1/chat/completions`.

Fallback: LFM 2.5-8B-A1B on `http://100.125.213.44:8081/v1/chat/completions`.
