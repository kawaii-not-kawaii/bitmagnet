package classifier

import (
	"errors"
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
)

func (c executionContext) tmdbSearchMovie(title string, year model.Year) (model.Content, error) {
	req := tmdb.SearchMovieRequest{
		Query:        title,
		IncludeAdult: true,
	}
	if !year.IsNil() {
		req.Year = year
	}

	searchResult, searchErr := c.tmdbClient.SearchMovie(c.Context, req)
	if searchErr != nil {
		return model.Content{}, searchErr
	}

	bestMatch, ok := levenshteinFindBestMatch[tmdb.SearchMovieResult](
		title,
		searchResult.Results,
		func(item tmdb.SearchMovieResult) []string {
			return []string{item.Title, item.OriginalTitle}
		},
	)

	if !ok {
		return c.tmdbMatchMovieByAlternativeTitles(title, searchResult.Results)
	}

	return c.tmdbGetMovieByTMDBID(bestMatch.ID)
}

func (c executionContext) tmdbSearchTVShow(title string, year model.Year) (model.Content, error) {
	req := tmdb.SearchTvRequest{
		Query:        title,
		IncludeAdult: true,
	}
	if !year.IsNil() {
		req.FirstAirDateYear = year
	}

	searchResult, searchErr := c.tmdbClient.SearchTv(c.Context, req)
	if searchErr != nil {
		return model.Content{}, searchErr
	}

	bestMatch, ok := levenshteinFindBestMatch[tmdb.SearchTvResult](
		title,
		searchResult.Results,
		func(item tmdb.SearchTvResult) []string {
			return []string{item.Name, item.OriginalName}
		},
	)

	if !ok {
		return c.tmdbMatchTVShowByAlternativeTitles(title, searchResult.Results)
	}

	return c.tmdbGetTVShowByTMDBID(bestMatch.ID)
}

// alternativeTitleSearchResults bounds the extra details calls made when no
// search result's name matches: TMDB orders results by relevance, and the
// right show is almost always first when the query is a romaji/alias title.
const alternativeTitleSearchResults = 3

// tmdbMatchTVShowByAlternativeTitles is the fallback for titles that TMDB's
// search finds but that match neither name nor original_name, typically romaji
// anime names ("Sousou no Frieren" for "Frieren: Beyond Journey's End").
func (c executionContext) tmdbMatchTVShowByAlternativeTitles(
	title string,
	results []tmdb.SearchTvResult,
) (model.Content, error) {
	for i, result := range results {
		if i >= alternativeTitleSearchResults {
			break
		}

		d, err := c.tmdbClient.TvDetails(c.Context, tmdb.TvDetailsRequest{
			SeriesID:         result.ID,
			AppendToResponse: []string{"external_ids", "alternative_titles"},
		})
		if err != nil {
			if errors.Is(err, tmdb.ErrNotFound) {
				continue
			}

			return model.Content{}, err
		}

		if alternativeTitlesMatch(title, d.AlternativeTitles.Results) {
			return tmdb.TvShowDetailsToTvShowModel(d)
		}
	}

	return model.Content{}, classification.ErrUnmatched
}

// tmdbMatchMovieByAlternativeTitles is the movie counterpart of
// tmdbMatchTVShowByAlternativeTitles.
func (c executionContext) tmdbMatchMovieByAlternativeTitles(
	title string,
	results []tmdb.SearchMovieResult,
) (model.Content, error) {
	for i, result := range results {
		if i >= alternativeTitleSearchResults {
			break
		}

		d, err := c.tmdbClient.MovieDetails(c.Context, tmdb.MovieDetailsRequest{
			ID:               result.ID,
			AppendToResponse: []string{"alternative_titles"},
		})
		if err != nil {
			if errors.Is(err, tmdb.ErrNotFound) {
				continue
			}

			return model.Content{}, err
		}

		if alternativeTitlesMatch(title, d.AlternativeTitles.Titles) {
			return tmdb.MovieDetailsToMovieModel(d)
		}
	}

	return model.Content{}, classification.ErrUnmatched
}

// alternativeTitlesMatch reports whether title is within the usual Levenshtein
// threshold of any alternative title, or of its part before a colon (long
// light-novel titles are usually released under their short form).
func alternativeTitlesMatch(title string, alternatives []tmdb.AlternativeTitle) bool {
	candidates := make([]string, 0, 2*len(alternatives))
	for _, alt := range alternatives {
		candidates = append(candidates, alt.Title)
		if head, _, found := strings.Cut(alt.Title, ":"); found && strings.TrimSpace(head) != "" {
			candidates = append(candidates, head)
		}
	}

	if len(candidates) == 0 {
		return false
	}

	distance := levenshteinFindMinDistance(title, candidates)

	return distance >= 0 && distance <= levenshteinThreshold
}

func (c executionContext) tmdbGetMovieByTMDBID(id int64) (movie model.Content, err error) {
	d, getDetailsErr := c.tmdbClient.MovieDetails(c.Context, tmdb.MovieDetailsRequest{
		ID: id,
	})
	if getDetailsErr != nil {
		if errors.Is(getDetailsErr, tmdb.ErrNotFound) {
			getDetailsErr = classification.ErrUnmatched
		}

		err = getDetailsErr

		return
	}

	return tmdb.MovieDetailsToMovieModel(d)
}

func (c executionContext) tmdbGetTVShowByTMDBID(id int64) (movie model.Content, err error) {
	d, getDetailsErr := c.tmdbClient.TvDetails(c.Context, tmdb.TvDetailsRequest{
		SeriesID:         id,
		AppendToResponse: []string{"external_ids"},
	})
	if getDetailsErr != nil {
		if errors.Is(getDetailsErr, tmdb.ErrNotFound) {
			getDetailsErr = classification.ErrUnmatched
		}

		err = getDetailsErr

		return
	}

	return tmdb.TvShowDetailsToTvShowModel(d)
}

func (c executionContext) tmdbGetTMDBIDByExternalID(ref model.ContentRef) (int64, error) {
	externalSource, externalID, externalSourceErr := tmdb.ExternalSource(ref)
	if externalSourceErr != nil {
		return 0, externalSourceErr
	}

	byIDResult, byIDErr := c.tmdbClient.FindByID(c.Context, tmdb.FindByIDRequest{
		ExternalSource: externalSource,
		ExternalID:     externalID,
	})
	if byIDErr != nil {
		return 0, byIDErr
	}

	switch ref.Type {
	case model.ContentTypeMovie, model.ContentTypeXxx:
		if len(byIDResult.MovieResults) == 0 {
			return 0, classification.ErrUnmatched
		}

		return byIDResult.MovieResults[0].ID, nil
	case model.ContentTypeTvShow:
		if len(byIDResult.TvResults) == 0 {
			return 0, classification.ErrUnmatched
		}

		return byIDResult.TvResults[0].ID, nil
	default:
		return 0, classification.ErrUnmatched
	}
}
