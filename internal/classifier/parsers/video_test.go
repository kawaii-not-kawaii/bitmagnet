package parsers

import (
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flattenEpisodes returns all episode numbers across seasons.
func flattenEpisodes(eps model.Episodes) (out []int) {
	for _, es := range eps {
		out = appendEpisodes(out, es)
	}

	return out
}

func appendEpisodes(out []int, episodes map[int]struct{}) []int {
	for e := range episodes {
		out = append(out, e)
	}

	return out
}

func TestParseTitleYearEpisodesEpisodePatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		episodes   []int
		seasonOnly bool
		season     int
	}{
		{
			name:     "[SubsPlease] Necronomico no Cosmic Horror Show - 11 (720p) [3731CD79].mkv",
			episodes: []int{11},
		},
		{
			name:     "사장님 귀는 당나귀 귀.E376.261011.720p-NEXT.mp4",
			episodes: []int{376},
		},
		{
			name:     "Peaky Blinders - Temporada 6 [HDTV 720p][Cap.605][AC3 5.1 Castellano][www.AtomoHD.XYZ]",
			episodes: []int{605},
		},
		{
			name:     "Privilegios [HDTV][Cap.105]",
			episodes: []int{105},
		},
		{
			name:     "S03E06 - Family Guy - 306 - Death Lives.avi",
			episodes: []int{6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, episodes, _, err := ParseTitleYearEpisodes(model.NullContentType{}, tt.name)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.episodes, flattenEpisodes(episodes))
		})
	}
}

func TestParseTitleYearEpisodesNumericNoise(t *testing.T) {
	t.Parallel()

	names := []string{
		"2637.zip",
		//nolint:gosmopolitan // corpus verbatim: CJK noise-regression names
		"0488-【新片】麻豆传媒  苏畅纯子韩语嫣 处男转生乱伦家庭无尽做爱 下集大年初三操逼拜年庆团圆",
		//nolint:gosmopolitan // corpus verbatim: CJK noise-regression name
		"51原创小姨子的秘密 小姨子瑜伽球翘臀扭腰球上口交 姐夫抓奶猛插到后入逼水四溅！",
		"HQ - 3D - Landscape.Ландшафт - 3200х1200 - 35 шт. (картинки)",
		"all_lady_do_it[576-312](1h32m)(Tinto_brass)",
		"260421142",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, episodes, _, _ := ParseTitleYearEpisodes(model.NullContentType{}, name)
			assert.Empty(t, flattenEpisodes(episodes), "episodes: %v", episodes)
		})
	}
}

func TestParseVideoContentEpisodePatterns(t *testing.T) {
	t.Parallel()

	tvNames := []string{
		"[SubsPlease] Necronomico no Cosmic Horror Show - 11 (720p) [3731CD79].mkv",
		"사장님 귀는 당나귀 귀.E376.261011.720p-NEXT.mp4",
		"Peaky Blinders - Temporada 6 [HDTV 720p][Cap.605][AC3 5.1 Castellano][www.AtomoHD.XYZ]",
		"S03E06 - Family Guy - 306 - Death Lives.avi",
	}
	for _, name := range tvNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			attrs, err := ParseVideoContent(model.Torrent{Name: name}, classification.Result{})
			require.NoError(t, err)
			assert.True(t, attrs.ContentType.Valid)
			assert.Equal(t, model.ContentTypeTvShow, attrs.ContentType.ContentType)
			assert.NotEmpty(t, attrs.Episodes)
		})
	}

	t.Run("noise stays unknown", func(t *testing.T) {
		t.Parallel()

		attrs, err := ParseVideoContent(model.Torrent{Name: "2637.zip"}, classification.Result{})
		require.NoError(t, err)
		assert.False(t, attrs.ContentType.Valid)
		assert.Empty(t, attrs.Episodes)
	})

	t.Run("year-only stays movie", func(t *testing.T) {
		t.Parallel()

		survivalist := "The Survivalist (2021) PL.480p.BRRip.x264.AC3-CrOOnos Lektor PL.mkv.mts"
		attrs, err := ParseVideoContent(model.Torrent{Name: survivalist}, classification.Result{})
		require.NoError(t, err)
		assert.True(t, attrs.ContentType.Valid)
		assert.Equal(t, model.ContentTypeMovie, attrs.ContentType.ContentType)
	})
}
