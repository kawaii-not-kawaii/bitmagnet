package classifier

import (
	"context"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
	tmdb_mocks "github.com/bitmagnet-io/bitmagnet/internal/tmdb/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Torrents often carry a romaji or alias title that TMDB's search finds but
// that matches neither name nor original_name (e.g. "Sousou no Frieren" vs
// "Frieren: Beyond Journey's End" / "葬送のフリーレン"). These tests pin the
// fallback that checks TMDB's alternative titles for the top search results.

var anyCtx = mock.MatchedBy(func(ctx any) bool {
	_, ok := ctx.(context.Context)
	return ok
})

func newAltTitlesContext(t *testing.T) (executionContext, *tmdb_mocks.Client) {
	t.Helper()
	client := tmdb_mocks.NewClient(t)

	return executionContext{
		Context:      context.Background(),
		dependencies: dependencies{tmdbClient: client},
	}, client
}

func tvAltTitles(titles ...string) tmdb.TvDetailsResponse {
	var d tmdb.TvDetailsResponse
	for _, title := range titles {
		d.AlternativeTitles.Results = append(d.AlternativeTitles.Results, tmdb.AlternativeTitle{Title: title})
	}

	return d
}

func TestTmdbSearchTVShowMatchesRomajiAlternativeTitle(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	client.On("SearchTv", anyCtx, tmdb.SearchTvRequest{Query: "Sousou no Frieren", IncludeAdult: true}).
		Return(tmdb.SearchTvResponse{Results: []tmdb.SearchTvResult{
			{ID: 209867, Name: "Frieren: Beyond Journey's End", OriginalName: "葬送のフリーレン"},
		}}, nil)

	details := tvAltTitles("Sōsō no Furīren", "Sousou no Frieren")
	details.ID = 209867
	details.Name = "Frieren: Beyond Journey's End"
	details.OriginalName = "葬送のフリーレン"
	details.FirstAirDate = "2023-09-29"
	client.On("TvDetails", anyCtx, tmdb.TvDetailsRequest{
		SeriesID:         209867,
		AppendToResponse: []string{"external_ids", "alternative_titles"},
	}).Return(details, nil)

	content, err := c.tmdbSearchTVShow("Sousou no Frieren", model.Year(0))

	require.NoError(t, err)
	assert.Equal(t, "209867", content.ID)
	assert.Equal(t, "Frieren: Beyond Journey's End", content.Title)
}

func TestTmdbSearchTVShowMatchesAlternativeTitleBeforeColon(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	client.On("SearchTv", anyCtx, tmdb.SearchTvRequest{Query: "Tensei Kenja no Isekai Life", IncludeAdult: true}).
		Return(tmdb.SearchTvResponse{Results: []tmdb.SearchTvResult{
			{ID: 117889, Name: "My Isekai Life: I Gained a Second Character Class and Became the Strongest Sage in the World!"},
		}}, nil)

	details := tvAltTitles("Tensei Kenja no Isekai Life: Daini no Shokugyou o Ete, Sekai Saikyou ni Narimashita")
	details.ID = 117889
	details.Name = "My Isekai Life"
	client.On("TvDetails", anyCtx, tmdb.TvDetailsRequest{
		SeriesID:         117889,
		AppendToResponse: []string{"external_ids", "alternative_titles"},
	}).Return(details, nil)

	content, err := c.tmdbSearchTVShow("Tensei Kenja no Isekai Life", model.Year(0))

	require.NoError(t, err)
	assert.Equal(t, "117889", content.ID)
}

func TestTmdbSearchTVShowStaysUnmatchedWhenNoAlternativeTitleFits(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	client.On("SearchTv", anyCtx, tmdb.SearchTvRequest{Query: "The Bear", IncludeAdult: true}).
		Return(tmdb.SearchTvResponse{Results: []tmdb.SearchTvResult{
			{ID: 1, Name: "Masha and the Bear"},
			{ID: 2, Name: "Bear Grylls: Survival Challenge"},
		}}, nil)
	client.On("TvDetails", anyCtx, tmdb.TvDetailsRequest{SeriesID: 1, AppendToResponse: []string{"external_ids", "alternative_titles"}}).
		Return(tvAltTitles("Masha i Medved"), nil)
	client.On("TvDetails", anyCtx, tmdb.TvDetailsRequest{SeriesID: 2, AppendToResponse: []string{"external_ids", "alternative_titles"}}).
		Return(tvAltTitles("Bear Grylls Survival School"), nil)

	_, err := c.tmdbSearchTVShow("The Bear", model.Year(0))

	assert.ErrorIs(t, err, classification.ErrUnmatched)
}

func TestTmdbSearchTVShowChecksAtMostThreeResultsForAlternativeTitles(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	results := make([]tmdb.SearchTvResult, 0, 5)
	for id := int64(1); id <= 5; id++ {
		results = append(results, tmdb.SearchTvResult{ID: id, Name: "Unrelated Show"})
	}
	client.On("SearchTv", anyCtx, tmdb.SearchTvRequest{Query: "Some Romaji Title", IncludeAdult: true}).
		Return(tmdb.SearchTvResponse{Results: results}, nil)
	client.On("TvDetails", anyCtx, mock.Anything).Return(tvAltTitles("Nothing Close"), nil).Times(3)

	_, err := c.tmdbSearchTVShow("Some Romaji Title", model.Year(0))

	assert.ErrorIs(t, err, classification.ErrUnmatched)
	client.AssertNumberOfCalls(t, "TvDetails", 3)
}

func TestTmdbSearchMovieMatchesAlternativeTitle(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	client.On("SearchMovie", anyCtx, tmdb.SearchMovieRequest{Query: "Kimi no Na wa", Year: 2016, IncludeAdult: true}).
		Return(tmdb.SearchMovieResponse{Results: []tmdb.SearchMovieResult{
			{ID: 372058, Title: "Your Name.", OriginalTitle: "君の名は。"},
		}}, nil)

	var details tmdb.MovieDetailsResponse
	details.ID = 372058
	details.Title = "Your Name."
	details.OriginalTitle = "君の名は。"
	details.ReleaseDate = "2016-08-26"
	details.AlternativeTitles.Titles = []tmdb.AlternativeTitle{{Title: "Kimi no Na wa."}}
	client.On("MovieDetails", anyCtx, tmdb.MovieDetailsRequest{
		ID:               372058,
		AppendToResponse: []string{"alternative_titles"},
	}).Return(details, nil)

	content, err := c.tmdbSearchMovie("Kimi no Na wa", model.Year(2016))

	require.NoError(t, err)
	assert.Equal(t, "372058", content.ID)
	assert.Equal(t, "Your Name.", content.Title)
}

func TestTmdbSearchTVShowDirectNameMatchSkipsAlternativeTitles(t *testing.T) {
	t.Parallel()

	c, client := newAltTitlesContext(t)
	client.On("SearchTv", anyCtx, tmdb.SearchTvRequest{Query: "Shogun", IncludeAdult: true}).
		Return(tmdb.SearchTvResponse{Results: []tmdb.SearchTvResult{{ID: 126308, Name: "Shōgun"}}}, nil)
	client.On("TvDetails", anyCtx, tmdb.TvDetailsRequest{SeriesID: 126308, AppendToResponse: []string{"external_ids"}}).
		Return(tmdb.TvDetailsResponse{ID: 126308, Name: "Shōgun"}, nil).Once()

	content, err := c.tmdbSearchTVShow("Shogun", model.Year(0))

	require.NoError(t, err)
	assert.Equal(t, "126308", content.ID)
}
