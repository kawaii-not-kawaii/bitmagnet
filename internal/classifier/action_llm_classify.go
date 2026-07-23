package classifier

import (
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

const llmClassifyActionName = "llm_classify"

type llmClassifyAction struct{}

func (llmClassifyAction) name() string {
	return llmClassifyActionName
}

var llmClassifyPayloadSpec = payloadLiteral[string]{
	literal:     llmClassifyActionName,
	description: "Classify a torrent using an LLM provider when normal classification fails",
}

func (llmClassifyAction) compileAction(ctx compilerContext) (action, error) {
	if _, err := llmClassifyPayloadSpec.Unmarshal(ctx); err != nil {
		return action{}, ctx.error(err)
	}

	path := ctx.path

	return action{
		run: func(ctx executionContext) (classification.Result, error) {
			cl := ctx.result
			event := llmobs.Event{
				Timestamp:   time.Now(),
				InfoHash:    ctx.torrent.InfoHash.String(),
				TorrentName: ctx.torrent.Name,
			}

			// Only classify if content type is not yet determined
			if cl.ContentType.Valid {
				return cl, nil
			}

			if ctx.llmEnabled != nil && !ctx.llmEnabled() {
				return cl, nil
			}

			// Get the providers current at classification time, so a runtime
			// LLM config update is observed without recompiling the workflow.
			var providers map[string]llm.Provider
			if ctx.llmProviders != nil {
				providers = ctx.llmProviders()
			}
			if len(providers) == 0 {
				event.Outcome = llmobs.OutcomeSkipped
				ctx.recorder.Record(event)

				ctx.logger.Warn("no llm providers configured, skipping llm classification")
				return cl, classification.RuntimeError{
					Cause: classification.ErrUnmatched,
					Path:  path,
				}
			}

			// Use first available provider
			var provider llm.Provider
			for _, p := range providers {
				provider = p
				break
			}
			event.Provider = provider.Name()

			// Build input from torrent
			input := llm.ClassifyInput{
				Name:         ctx.torrent.Name,
				ContentTypes: buildContentTypeList(),
			}

			// Include file paths if available
			if len(ctx.torrent.Files) > 0 {
				files := make([]string, 0, min(len(ctx.torrent.Files), 20))
				for i, f := range ctx.torrent.Files {
					if i >= 20 {
						break
					}
					files = append(files, f.Path)
				}
				input.Files = files
			}

			// Call LLM
			done := ctx.recorder.Begin()
			started := time.Now()
			result, err := provider.Classify(ctx, input)
			if result != nil {
				event.PromptTokens = result.PromptTokens
				event.CompletionTokens = result.CompletionTokens
			}
			event.Duration = time.Since(started)
			done()

			if err != nil {
				event.Outcome = llmobs.OutcomeError
				event.Error = err.Error()
				ctx.recorder.Record(event)
				ctx.logger.Warnw("llm classification failed",
					"provider", provider.Name(),
					"error", err)
				return cl, classification.RuntimeError{
					Cause: classification.ErrUnmatched,
					Path:  path,
				}
			}

			// Apply result
			cl = applyLLMResult(cl, result)
			event.Outcome = llmobs.OutcomeMatched
			if !usableLLMResult(result) {
				event.Outcome = llmobs.OutcomeUnmatched
			}

			event.ContentType = result.ContentType
			event.Title = result.Title
			event.Year = result.Year
			event.Season = result.Season
			event.Episode = result.Episode
			event.Languages = result.Language
			ctx.recorder.Record(event)
			ctx.logger.Infow("llm classification",
				"provider", provider.Name(),
				"content_type", result.ContentType,
				"title", result.Title)

			return cl, nil
		},
	}, nil
}

func (llmClassifyAction) JSONSchema() JSONSchema {
	return llmClassifyPayloadSpec.JSONSchema()
}

// applyLLMResult maps LLM output onto classification attributes.
func applyLLMResult(cl classification.Result, r *llm.ClassifyResult) classification.Result {
	if r.ContentType != "" {
		cl.ContentType = model.NewNullContentType(r.ContentType)
	}

	if r.Title != "" {
		cl.BaseTitle = model.NewNullString(r.Title)
	}

	if r.Year > 0 {
		cl.Date = model.Date{Year: model.Year(r.Year)}
	}

	if r.Season > 0 && r.Episode > 0 {
		if cl.Episodes == nil {
			cl.Episodes = make(model.Episodes)
		}

		if cl.Episodes[r.Season] == nil {
			cl.Episodes[r.Season] = make(map[int]struct{})
		}

		cl.Episodes[r.Season][r.Episode] = struct{}{}
	}

	if r.VideoResolution != "" {
		cl.VideoResolution = model.NewNullVideoResolution(r.VideoResolution)
	}

	if r.VideoSource != "" {
		cl.VideoSource = model.NewNullVideoSource(r.VideoSource)
	}

	if r.VideoCodec != "" {
		cl.VideoCodec = model.NewNullVideoCodec(r.VideoCodec)
	}

	if r.ReleaseGroup != "" {
		cl.ReleaseGroup = model.NewNullString(r.ReleaseGroup)
	}

	if len(r.Language) > 0 {
		if cl.Languages == nil {
			cl.Languages = make(model.Languages)
		}
		// Defer validation to model.ParseLanguage, the canonical helper used
		// everywhere else in the codebase (gql facets, search criteria, etc.).
		// Unrecognized codes are dropped silently — the LLM has a history of
		// inventing values, and we already have 'invalid tag name' failures
		// downstream from unvalidated LLM output. Same posture here.
		newFromLLM := make(map[model.Language]struct{})

		for _, code := range r.Language {
			lang := model.ParseLanguage(code)
			if lang.Valid {
				cl.Languages[lang.Language] = struct{}{}
				newFromLLM[lang.Language] = struct{}{}
			}
		}
		// Flip LanguageMulti only when the LLM provided 2+ DISTINCT valid
		// languages — direct evidence of a multi-language release. Mirrors
		// the torrent-name parser's multiRegex rule at parsers/video.go:215,
		// and makes AttachContent (result.go:32-40) MERGE rather than REPLACE
		// the language set when content.OriginalLanguage is later attached.
		if len(newFromLLM) > 1 {
			cl.LanguageMulti = true
		}
	}

	if len(r.Tags) > 0 {
		if cl.Tags == nil {
			cl.Tags = classification.NewTagAction()
		}

		if cl.Tags.Add == nil {
			cl.Tags.Add = make(map[string]struct{})
		}

		for _, tag := range r.Tags {
			sanitized := sanitizeTag(tag)
			if sanitized != "" {
				cl.Tags.Add[sanitized] = struct{}{}
			}
		}
	}

	return cl
}

func usableLLMResult(r *llm.ClassifyResult) bool {
	return r.ContentType != "" ||
		r.Title != "" ||
		r.Year > 0 ||
		r.Season > 0 && r.Episode > 0 ||
		len(r.Language) > 0 ||
		r.VideoResolution != "" ||
		r.VideoSource != "" ||
		r.VideoCodec != "" ||
		r.ReleaseGroup != "" ||
		len(r.Tags) > 0
}

func buildContentTypeList() string {
	return strings.Join(model.ContentTypeNames(), ", ")
}

// sanitizeTag normalizes an LLM-generated tag to match the torrent_tags CHECK constraint:
// ^[a-z0-9]+(-[a-z0-9]+)*$
func sanitizeTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	// replace spaces and underscores with hyphens
	tag = strings.ReplaceAll(tag, " ", "-")
	tag = strings.ReplaceAll(tag, "_", "-")
	// remove invalid characters
	var b strings.Builder

	for _, r := range tag {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}

	result := b.String()
	// collapse multiple hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	// trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	if len(result) > model.TagNameMaxLength {
		result = result[:model.TagNameMaxLength]
		// truncation may have landed on a trailing hyphen; trim again
		result = strings.TrimRight(result, "-")
	}

	return result
}
