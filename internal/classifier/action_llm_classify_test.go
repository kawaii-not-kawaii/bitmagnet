package classifier

import (
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/stretchr/testify/assert"
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
			languages:         []string{"rus", "spa"}, // alpha-3 inputs
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
		tc := tc
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
