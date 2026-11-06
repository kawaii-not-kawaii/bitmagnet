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
		name     string
		episodes []int
		season   int
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
			// Spanish convention: Cap.605 -> season 6, episode 5
			name:     "Peaky Blinders - Temporada 6 [HDTV 720p][Cap.605][AC3 5.1 Castellano][www.AtomoHD.XYZ]",
			episodes: []int{5},
			season:   6,
		},
		{
			name:     "Privilegios [HDTV][Cap.105]",
			episodes: []int{5},
			season:   1,
		},
		{
			name:     "S03E06 - Family Guy - 306 - Death Lives.avi",
			episodes: []int{6},
			season:   3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, episodes, _, err := ParseTitleYearEpisodes(model.NullContentType{}, tt.name)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.episodes, flattenEpisodes(episodes))

			if tt.season != 0 {
				assert.Contains(t, episodes, tt.season)
			}
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

func TestParseTitleYearDateAndVersionGuard(t *testing.T) {
	t.Parallel()

	//nolint:gosmopolitan // corpus verbatim: CJK name from the labeled set
	pixiv := "[Pixiv] [Twitter] [Bilibili] ATDAN丨小原原原子Dan 2026.09.06.zip"

	noYear := []string{
		"Met-Art 2019-05-25",
		pixiv,
		"Raining Blood Demo v14.02.2023.rar",
		"Some Show 25.05.2019",
		"Some Show 05-25-2019",
		"Some Show 2019/05/25",
		"Some Tool v1.2.2020",
	}
	for _, name := range noYear {
		t.Run("no year/"+name, func(t *testing.T) {
			t.Parallel()

			_, year, _, _, _ := ParseTitleYearEpisodes(model.NullContentType{}, name)
			assert.True(t, year.IsNil(), "year: %v", year)

			attrs, err := ParseVideoContent(model.Torrent{Name: name}, classification.Result{})
			if err == nil {
				assert.NotEqual(t, model.ContentTypeMovie, attrs.ContentType.ContentType)
			}
		})
	}

	withYear := []struct {
		name  string
		title string
		year  model.Year
	}{
		{name: "The Regular Movie (2000)", title: "The Regular Movie", year: 2000},
		{name: "Title.2000.1080p.BluRay.x264", title: "Title", year: 2000},
		{name: "Title 2019 1080p", title: "Title", year: 2019},
		{name: "Title 2019", title: "Title", year: 2019},
		{name: "Blade Runner 2049 (2017) 1080p", title: "Blade Runner 2049", year: 2017},
		// sequel numbers and bit-depth tokens next to the year are not date parts
		{name: "Apollo.13.1995.1080p.BluRay.x264", title: "Apollo 13", year: 1995},
		{name: "District.9.2009.720p.BluRay", title: "District 9", year: 2009},
		{name: "Toy Story 3 2010 1080p", title: "Toy Story 3", year: 2010},
		{name: "Movie.2019.10bit.x265", title: "Movie", year: 2019},
		{name: "Movie 2019 5.1 Audio", title: "Movie", year: 2019},
	}
	for _, tt := range withYear {
		t.Run("year/"+tt.name, func(t *testing.T) {
			t.Parallel()

			title, year, _, _, err := ParseTitleYearEpisodes(model.NullContentType{}, tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.title, title)
			assert.Equal(t, tt.year, year)

			attrs, err := ParseVideoContent(model.Torrent{Name: tt.name}, classification.Result{})
			require.NoError(t, err)
			assert.Equal(t, model.ContentTypeMovie, attrs.ContentType.ContentType)
		})
	}
}

// A leading site/tracker advert must not become part of the title sent to TMDB:
// on the live instance, movie/tv names starting with "www." were 88% unmatched
// versus 38% for everything else.
func TestParseTitleYearEpisodesSitePrefix(t *testing.T) {
	t.Parallel()

	//nolint:gosmopolitan // corpus verbatim: CJK release-site advert from the live DB
	cjkAdvert := "【高清影视之家发布 www.WHATMV.com】峡谷[简繁英字幕].The.Gorge.2025.2160p.Apple.TV+.WEB-DL"

	tests := []struct {
		name     string
		title    string
		year     model.Year
		episodes string
	}{
		{"www.UIndex.org    -    Chicago.Fire.S14E17.1080p.AV1.10bit-MeGusta", "Chicago Fire", 0, "S14E17"},
		{
			"www.UIndex.org    -    Family.Guy.S21E15.Adoptation.720p.HEVC.x265-MeGusta",
			"Family Guy",
			0,
			"S21E15",
		},
		{
			"www.Torrenting.com - A Quiet Place Day One (2024) 1080p BluRay 5.1-LAMA",
			"A Quiet Place Day One",
			2024,
			"",
		},
		{
			"www.Torrenting.org       Obsession Dark Desires S03E06 Beauty in the Bronx 1080p WEB h2",
			"Obsession Dark Desires", 0, "S03E06",
		},
		{
			"www.5MovieRulz.luxe - Veera Dheera Sooran (2025) 1080p Telugu HDRip - x264",
			"Veera Dheera Sooran",
			2025,
			"",
		},
		{"[ www.Torrenting.com ] - Storage.Wars.NY.S01.480p.HDTV.x264-mSD", "Storage Wars NY", 0, "S01"},
		// controls: no prefix, and a domain that is not a leading advert
		{"Caddo Lake 2024 1080p WEBRip x265-DH", "Caddo Lake", 2024, ""},
		{"The.1.Percent.Club.S03E11.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "The 1 Percent Club", 0, "S03E11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			title, year, episodes, _, err := ParseTitleYearEpisodes(model.NullContentType{}, tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.title, title)
			assert.Equal(t, tt.year, year)
			assert.Equal(t, tt.episodes, episodes.String())
		})
	}

	// A bracketed advert is dropped; splitting the remaining bilingual title
	// ("峡谷 … The Gorge") is a separate problem.
	t.Run("cjk advert", func(t *testing.T) {
		t.Parallel()

		title, year, _, _, err := ParseTitleYearEpisodes(model.NullContentType{}, cjkAdvert)
		require.NoError(t, err)
		assert.NotContains(t, title, "www")
		assert.NotContains(t, title, "WHATMV")
		assert.Equal(t, model.Year(2025), year)
	})

	// Nothing left after the prefix: keep the old behaviour rather than an empty title.
	t.Run("prefix only", func(t *testing.T) {
		t.Parallel()

		title, _, _, _, err := ParseTitleYearEpisodes(model.NullContentType{}, "www.example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, title)
	})
}
