package classifier

import (
	"context"
	"errors"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestApplyLLMResult_Language locks in the contract that the Language field
// — which the LLM is explicitly asked for in the system prompt and which was
// previously parsed then silently discarded — actually reaches the
// classification Result as model.Languages, validated against the canonical
// language list via model.ParseLanguage.
//
// Inputs use ISO 639-1 alpha-2 codes (en, ru, es, ta, hi). ParseLanguage
// normalizes alpha-3 and alias inputs to alpha-2 — that normalization path
// is covered by the "alpha3_alias_normalized_to_alpha2" case.
//
// Coverage matrix:
//   - single valid language -> 1 entry in cl.Languages, LanguageMulti=false
//   - multiple valid languages -> N entries, LanguageMulti=true (mirrors the
//     torrent-name parser's multiRegex rule at parsers/video.go:215)
//   - mixed valid+invalid -> invalid codes dropped, valid kept, LanguageMulti
//     stays false unless 2+ distinct VALID codes were provided
//   - all invalid -> no Languages populated, LanguageMulti unchanged
//   - duplicates collapsed (set semantics)
//   - LanguageMulti is OR-ed onto any prior true (Merge convention, result.go:75)
//   - alpha-3 inputs are normalized to alpha-2 by ParseLanguage
func TestApplyLLMResult_Language(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		languages         []string
		preExistingLangs  []model.Language // entries already in cl.Languages before applyLLMResult
		preExistingMulti  bool             // value of cl.LanguageMulti before applyLLMResult
		wantLanguages     []model.Language // expected set membership after applyLLMResult
		wantLanguageMulti bool
	}{
		{
			name:              "single_valid_language",
			languages:         []string{"en"},
			wantLanguages:     []model.Language{"en"},
			wantLanguageMulti: false,
		},
		{
			name:              "multiple_valid_languages_flips_multi_flag",
			languages:         []string{"ru", "es"},
			wantLanguages:     []model.Language{"ru", "es"},
			wantLanguageMulti: true,
		},
		{
			name:              "three_languages_real_world_police_polic",
			languages:         []string{"ta", "hi", "en"},
			wantLanguages:     []model.Language{"ta", "hi", "en"},
			wantLanguageMulti: true,
		},
		{
			name:              "mixed_valid_and_invalid_drops_invalid",
			languages:         []string{"ru", "XXINVALID", "es"},
			wantLanguages:     []model.Language{"ru", "es"},
			wantLanguageMulti: true, // 2 distinct valid codes still counts as multi
		},
		{
			name:              "single_invalid_code_dropped_no_multi",
			languages:         []string{"XXINVALID"},
			wantLanguages:     nil,
			wantLanguageMulti: false,
		},
		{
			name:              "all_invalid_codes_dropped",
			languages:         []string{"XX", "YY", "ZZ"},
			wantLanguages:     nil,
			wantLanguageMulti: false,
		},
		{
			name:              "duplicates_collapse_to_single_entry_no_multi",
			languages:         []string{"en", "en", "en"},
			wantLanguages:     []model.Language{"en"},
			wantLanguageMulti: false, // only 1 distinct valid code
		},
		{
			name:              "alpha3_alias_normalized_to_alpha2",
			languages:         []string{"rus", "spa"},       // alpha-3 inputs
			wantLanguages:     []model.Language{"ru", "es"}, // normalized to alpha-2
			wantLanguageMulti: true,
		},
		{
			name:              "preserves_pre_existing_languages_and_or_multi",
			languages:         []string{"es"},
			preExistingLangs:  []model.Language{"ru"},
			preExistingMulti:  true, // already multi from a prior source
			wantLanguages:     []model.Language{"ru", "es"},
			wantLanguageMulti: true, // OR-ed: stays true
		},
		{
			name:              "empty_language_slice_leaves_cl_untouched",
			languages:         nil,
			preExistingLangs:  []model.Language{"en"},
			wantLanguages:     []model.Language{"en"},
			wantLanguageMulti: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cl := classification.Result{
				ContentAttributes: classification.ContentAttributes{
					LanguageMulti: tc.preExistingMulti,
				},
			}
			if len(tc.preExistingLangs) > 0 {
				cl.Languages = make(model.Languages)
				for _, l := range tc.preExistingLangs {
					cl.Languages[l] = struct{}{}
				}
			}

			r := &llm.ClassifyResult{Language: tc.languages}
			cl = applyLLMResult(cl, r)

			// Build expected set for comparison.
			wantSet := make(model.Languages)
			for _, l := range tc.wantLanguages {
				wantSet[l] = struct{}{}
			}

			assert.Equal(t, wantSet, cl.Languages, "Languages set mismatch")
			assert.Equal(t, tc.wantLanguageMulti, cl.LanguageMulti, "LanguageMulti mismatch")
		})
	}
}

// TestApplyLLMResult_LanguageSmokeOneCode proves a known canonical code
// round-trips end-to-end through ParseLanguage into the set. This catches
// regressions where ParseLanguage changes its input contract or the
// languages.csv embedded data loses a common code.
func TestApplyLLMResult_LanguageSmokeOneCode(t *testing.T) {
	t.Parallel()

	cl := classification.Result{}
	r := &llm.ClassifyResult{Language: []string{"en"}}
	cl = applyLLMResult(cl, r)

	assert.Contains(t, cl.Languages, model.Language("en"))
	assert.False(t, cl.LanguageMulti, "single language must not flip LanguageMulti")
}

// TestSanitizeTag locks in the contract that sanitizeTag's output always
// passes model.ValidateTagName, which enforces both the kebab-case shape
// AND a 30-character max length. The length cap was previously unenforced
// here: an LLM tag that normalized to a valid but >30-char string sanitized
// "successfully" yet still failed at insert time via the
// TorrentTag.BeforeCreate hook, surfacing as a confusing downstream
// "invalid tag name" DB error instead of being caught at classification time.
func TestSanitizeTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already_clean", "japanese", "japanese"},
		{"uppercase", "Japanese", "japanese"},
		{"spaces_and_underscores", "final fantasy_vii", "final-fantasy-vii"},
		{"invalid_chars_stripped", "final-fantasy-vii!", "final-fantasy-vii"},
		{"double_hyphens_collapsed", "final--fantasy---vii", "final-fantasy-vii"},
		{"trailing_hyphen_trimmed", "final-fantasy-vii-", "final-fantasy-vii"},
		{
			"long_input_truncated_to_max_length",
			"this-is-a-very-long-descriptive-genre-tag-that-a-model-might-invent",
			"this-is-a-very-long-descriptiv",
		},
		{
			// truncation landing exactly on a hyphen boundary must not
			// leave a dangling trailing hyphen
			"truncation_on_hyphen_boundary_retrimmed",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeTag(tc.in)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, len(got), model.TagNameMaxLength)

			if got != "" {
				assert.NoError(
					t,
					model.ValidateTagName(got),
					"sanitizeTag output must always satisfy ValidateTagName",
				)
			}
		})
	}
}

type llmActionTestProvider struct {
	result *llm.ClassifyResult
	err    error
}

func (p llmActionTestProvider) Name() string {
	return "test"
}

func (p llmActionTestProvider) Classify(
	context.Context,
	llm.ClassifyInput,
) (*llm.ClassifyResult, error) {
	return p.result, p.err
}

func TestLLMClassifyRecordingPreservesBehavior(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("provider unavailable")
	matchedResult := &llm.ClassifyResult{
		ContentType: "movie",
		Title:       "Example",
		Year:        2024,
		Season:      1,
		Episode:     2,
		Language:    []string{"en", "es"},
	}
	cases := []struct {
		name        string
		providers   map[string]llm.Provider
		wantOutcome llmobs.Outcome
		wantError   string
	}{
		{
			name: "matched",
			providers: map[string]llm.Provider{
				"test": llmActionTestProvider{result: matchedResult},
			},
			wantOutcome: llmobs.OutcomeMatched,
		},
		{
			name: "unmatched",
			providers: map[string]llm.Provider{
				"test": llmActionTestProvider{result: &llm.ClassifyResult{}},
			},
			wantOutcome: llmobs.OutcomeUnmatched,
		},
		{
			name: "error",
			providers: map[string]llm.Provider{
				"test": llmActionTestProvider{err: providerErr},
			},
			wantOutcome: llmobs.OutcomeError,
			wantError:   providerErr.Error(),
		},
		{
			name:        "skipped",
			wantOutcome: llmobs.OutcomeSkipped,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			compiled, err := (llmClassifyAction{}).compileAction(compilerContext{
				source: llmClassifyActionName,
				path:   []string{"workflows", "test"},
			})
			require.NoError(t, err)

			torrent := model.Torrent{Name: "Example.Torrent"}
			run := func(recorder *llmobs.Recorder) (classification.Result, error) {
				logger := zap.NewNop().Sugar()

				return compiled.run(executionContext{
					Context: context.Background(),
					dependencies: dependencies{
						llmProviders: func() map[string]llm.Provider {
							return tc.providers
						},
						recorder: recorder,
						_logger:  logger,
						logger:   logger,
					},
					torrent: torrent,
				})
			}

			withoutRecorderResult, withoutRecorderErr := run(nil)
			recorder := llmobs.New()
			withRecorderResult, withRecorderErr := run(recorder)

			assert.Equal(t, withoutRecorderResult, withRecorderResult)
			assert.Equal(t, withoutRecorderErr, withRecorderErr)

			events := recorder.Events(1)
			require.Len(t, events, 1)

			event := events[0]
			assert.Equal(t, tc.wantOutcome, event.Outcome)
			assert.Equal(t, torrent.InfoHash.String(), event.InfoHash)
			assert.Equal(t, torrent.Name, event.TorrentName)
			assert.Equal(t, tc.wantError, event.Error)
			assert.Zero(t, recorder.Stats(0).InFlight)

			if tc.wantOutcome == llmobs.OutcomeSkipped {
				assert.Empty(t, event.Provider)
				assert.Zero(t, event.Duration)
			} else {
				assert.Equal(t, "test", event.Provider)
			}

			if tc.wantOutcome == llmobs.OutcomeMatched {
				assert.Equal(t, matchedResult.ContentType, event.ContentType)
				assert.Equal(t, matchedResult.Title, event.Title)
				assert.Equal(t, matchedResult.Year, event.Year)
				assert.Equal(t, matchedResult.Season, event.Season)
				assert.Equal(t, matchedResult.Episode, event.Episode)
				assert.Equal(t, matchedResult.Language, event.Languages)
			}
		})
	}
}
