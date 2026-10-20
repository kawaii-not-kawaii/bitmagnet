package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  Language
	}{
		{
			input: "en",
			want:  Language("en"),
		},
		{
			input: "eng",
			want:  Language("en"),
		},
		{
			input: "English",
			want:  Language("en"),
		},
		{
			input: "moldovan",
			want:  Language("ro"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			lang := ParseLanguage(tt.input)
			assert.Equal(t, tt.want, lang.Language)
			assert.True(t, lang.Valid)
		})
	}
}

func TestInferLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []Language
	}{
		{
			input: "eng",
			want:  []Language{Language("en")},
		},
		{
			input: "English",
			want:  []Language{Language("en")},
		},
		{
			input: "moldovan",
			want:  []Language{Language("ro")},
		},
		{
			input: "en, eng, English, moldovan",
			want:  []Language{Language("en"), Language("ro")},
		},
		{
			input: "en, eng, English, moldovan, fr",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			input: "foo bar en, eng, English, moldovan, fr, French bat bing",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			input: "en, eng, English, moldovan, fr, French, fr",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			input: "en, eng, English, moldovan, fr, French, fr, fr",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			input: "en, eng, English, moldovan, fr, French, fr, fr, fr",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			input: "en, eng, English, moldovan, fr, French, fr, fr, fr, fr",
			want:  []Language{Language("en"), Language("fr"), Language("ro")},
		},
		{
			// (live DB false positive: 'sa' via alpha3 'san' inside 'Shikimori-san')
			input: "[Anistar.org] Kawaii dake ja Nai Shikimori-san [TV-1] - 09 [720p].mkv",
			want:  []Language{},
		},
		{
			// hyphen-joined fragments excluded, but bracket/dot/underscore boundaries still match
			input: "Movie [JPN_SUB] FRENCH.WEB x-rus y",
			want:  []Language{Language("fr"), Language("ja")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			langs := InferLanguages(tt.input).Slice()
			assert.Equal(t, tt.want, langs)
		})
	}
}
