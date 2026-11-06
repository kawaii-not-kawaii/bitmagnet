package parsers

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/keywords"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/regex"
	"github.com/hedhyw/rex/pkg/dialect"
	"github.com/hedhyw/rex/pkg/rex"
)

var titleTokens = []dialect.Token{
	rex.Group.Define(
		rex.Group.Composite(
			rex.Group.NonCaptured(
				regex.AnyWordChar().Repeat().OneOrMore(),
				rex.Group.NonCaptured(
					rex.Chars.Single('-'), regex.AnyWordChar().Repeat().OneOrMore(),
				).Repeat().ZeroOrMore(),
			),
			regex.AnyNonWordChar().Repeat().OneOrMore(),
		).NonCaptured().Repeat().OneOrMore(),
		rex.Group.Composite(
			regex.AnyNonWordChar().Repeat().OneOrMore(),
			rex.Chars.End(),
		).NonCaptured(),
	).WithName("title"),
}

var titleRegex = rex.New(
	rex.Chars.Begin(),
	rex.Group.NonCaptured(titleTokens...),
).MustCompile()

var yearTokens = []dialect.Token{
	rex.Group.NonCaptured(rex.Common.NotClass(rex.Chars.WordCharacter()).Repeat().ZeroOrMore()),
	rex.Group.Define(
		rex.Group.Composite(
			rex.Common.Text("18"), rex.Common.Text("19"), rex.Common.Text("20"),
		).NonCaptured(),
		rex.Chars.Digits().Repeat().Exactly(2),
	).WithName("year"),
	rex.Group.Composite(
		rex.Common.NotClass(rex.Chars.WordCharacter()),
		rex.Chars.End(),
	).NonCaptured(),
}

var titleYearRegex = rex.New(
	rex.Chars.Begin(),
	rex.Group.NonCaptured(rex.Group.NonCaptured(titleTokens...), rex.Group.NonCaptured(yearTokens...)),
).MustCompile()

var titleEpisodesRegex = rex.New(
	rex.Chars.Begin(),
	rex.Group.NonCaptured(
		rex.Group.NonCaptured(titleTokens...),
		model.EpisodesToken,
	),
).MustCompile()

// titleEpisodesLooseRegex additionally allows an optional title (leading
// 'S03E06 - Title') and bracket-guarded episode numbers ('- 11 (720p)');
// tried only after titleEpisodesRegex, whose season-aware forms take
// precedence.
var titleEpisodesLooseRegex = rex.New(
	rex.Chars.Begin(),
	rex.Group.NonCaptured(
		rex.Group.NonCaptured(titleTokens...).Repeat().ZeroOrOne(),
		model.EpisodesWithBracketsToken,
	),
).MustCompile()

var multiRegex = keywords.MustNewRegexFromKeywords("multi", "dual")

var separatorToken = rex.Chars.Runes(" ._")

var titlePartRegex = rex.New(
	separatorToken.Repeat().ZeroOrOne(),
	rex.Group.Define(regex.WordToken()).WithName("part"),
	separatorToken.Repeat().ZeroOrOne(),
).MustCompile()

var trimTitleRegex = rex.New(
	rex.Chars.Begin(),
	rex.Group.Composite(
		rex.Group.NonCaptured(
			rex.Chars.Single('['),
			rex.Common.NotClass(rex.Chars.Single(']')).Repeat().OneOrMore(),
			rex.Chars.Single(']'),
		),
		rex.Group.NonCaptured(
			rex.Chars.Single('【'),
			rex.Common.NotClass(rex.Chars.Single('】')).Repeat().OneOrMore(),
			rex.Chars.Single('】'),
		),
	).NonCaptured().Repeat().ZeroOrOne(),
	regex.AnyNonWordChar().Repeat().ZeroOrMore(),
	rex.Group.Define(
		regex.WordToken(),
		rex.Group.NonCaptured(
			rex.Chars.Any(),
			regex.WordToken(),
		).Repeat().ZeroOrMore(),
	).WithName("trimmed"),
	regex.AnyNonWordChar().Repeat().ZeroOrMore(),
	rex.Chars.End(),
).MustCompile()

func cleanTitle(title string) string {
	title = titlePartRegex.ReplaceAllStringFunc(title, func(s string) string {
		partMatch := titlePartRegex.FindStringSubmatch(s)
		if partMatch == nil {
			return ""
		}

		return partMatch[titlePartRegex.SubexpIndex("part")] + " "
	})
	title = trimTitleRegex.ReplaceAllString(title, "${trimmed}")

	return title
}

// yearBeforeDatePartRegex matches the tail of the text preceding a year
// candidate when that candidate is the last part of a date ('25.05.', '05-25-')
// or of a version ('v14.02.', 'v1.2.'). A date needs two numeric parts: a single
// number before the year is usually a sequel ('Apollo.13.1995', 'District.9.2009').
var yearBeforeDatePartRegex = regexp.MustCompile(`(?:^|\W)(?:\d{1,2}[./_-]\d{1,2}|[vV]\d+(?:\.\d+)*)[./_-]$`)

// yearAfterDatePartRegex matches the text following a year candidate when it
// is the first part of a date ('.05.25', '-05-25', '/05/25'). Two numeric parts
// are required so tokens like '.10bit' or ' 5.1' after a year don't count.
var yearAfterDatePartRegex = regexp.MustCompile(`^[./_-]\d{1,2}[./_-]\d{1,2}(?:\D|$)`)

// isYearInDateOrVersion reports whether the year at input[start:end] is part
// of a full date or version string rather than a standalone release year.
func isYearInDateOrVersion(input string, start, end int) bool {
	return yearBeforeDatePartRegex.MatchString(input[:start]) ||
		yearAfterDatePartRegex.MatchString(input[end:])
}

func parseTitleYear(input string) (string, model.Year, string, error) {
	if idx := titleYearRegex.FindStringSubmatchIndex(input); idx != nil {
		yearIdx := titleYearRegex.SubexpIndex("year")
		titleIdx := titleYearRegex.SubexpIndex("title")

		if !isYearInDateOrVersion(input, idx[2*yearIdx], idx[2*yearIdx+1]) {
			yearMatch, _ := strconv.ParseUint(input[idx[2*yearIdx]:idx[2*yearIdx+1]], 10, 16)
			title := cleanTitle(input[idx[2*titleIdx]:idx[2*titleIdx+1]])

			if title != "" {
				return title, model.Year(yearMatch), input[idx[1]:], nil
			}
		}
	}

	return "", 0, "", classification.ErrUnmatched
}

func parseTitle(input string) (title string, rest string, err error) {
	if match := titleRegex.FindStringSubmatch(input); match != nil {
		title = cleanTitle(match[titleRegex.SubexpIndex("title")])
		if title != "" {
			return title, input[len(match[0]):], nil
		}
	}

	return "", "", classification.ErrUnmatched
}

func parseTitleYearEpisodes(input string) (string, model.Year, model.Episodes, string, error) {
	for _, re := range []*regexp.Regexp{titleEpisodesRegex, titleEpisodesLooseRegex} {
		if match := re.FindStringSubmatch(input); match != nil {
			title := match[re.SubexpIndex("title")]
			year := model.Year(0)

			if t, y, _, err := parseTitleYear(title); err == nil {
				title = t
				year = y
			} else {
				title = cleanTitle(title)
			}

			episodes := model.EpisodesMatchToEpisodes(re, match)

			return title, year, episodes, input[len(match[0]):], nil
		}
	}

	return "", 0, nil, "", classification.ErrUnmatched
}

// sitePrefixRegex matches a leading release-site advert: a bare domain
// ("www.UIndex.org - ", "www.Torrenting.org   ") or a bracketed group that
// contains one ("[ www.Torrenting.com ] - ", "【高清影视之家发布 www.WHATMV.com】").
// A bare domain must be followed by whitespace or a separator, so a name like
// "www.site.com.Movie.2020" is never cut mid-token.
var sitePrefixRegex = regexp.MustCompile(
	`^\s*(?:` +
		`(?:https?://)?www\.[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)+(?:\s*[-–—:|]+\s*|\s+)` +
		`|\[[^\]]*www\.[^\]]*\]\s*[-–—:|]*\s*` +
		`|【[^】]*www\.[^】]*】\s*[-–—:|]*\s*` +
		`|\([^)]*www\.[^)]*\)\s*[-–—:|]*\s*` +
		`)`,
)

// stripSitePrefix removes a leading release-site advert so it can't become
// part of the title searched on TMDB. The name is returned unchanged when
// nothing would be left.
func stripSitePrefix(input string) string {
	loc := sitePrefixRegex.FindStringIndex(input)
	if loc == nil || strings.TrimSpace(input[loc[1]:]) == "" {
		return input
	}

	return input[loc[1]:]
}

func ParseTitleYearEpisodes(
	contentType model.NullContentType,
	input string,
) (string, model.Year, model.Episodes, string, error) {
	input = stripSitePrefix(input)

	if !contentType.Valid || contentType.ContentType == model.ContentTypeTvShow {
		if title, year, episodes, rest, err := parseTitleYearEpisodes(input); err == nil {
			return title, year, episodes, rest, nil
		}
	}

	if title, year, rest, err := parseTitleYear(input); err == nil {
		return title, year, nil, rest, nil
	}

	if title, rest, err := parseTitle(input); err == nil {
		return title, 0, nil, rest, nil
	}

	return "", 0, nil, "", classification.ErrUnmatched
}

func ParseVideoContent(torrent model.Torrent, result classification.Result) (classification.ContentAttributes, error) {
	title, year, episodes, rest, err := ParseTitleYearEpisodes(result.ContentType, torrent.Name)
	if err != nil {
		if !result.ContentType.Valid {
			return classification.ContentAttributes{}, err
		}

		rest = torrent.Name
	}

	ct := model.NullContentType{}

	switch {
	case result.ContentType.Valid:
		ct = model.NullContentType{Valid: true, ContentType: result.ContentType.ContentType}
	case len(episodes) > 0 || result.Date.IsValid():
		ct = model.NullContentType{Valid: true, ContentType: model.ContentTypeTvShow}
	case !year.IsNil():
		ct = model.NullContentType{Valid: true, ContentType: model.ContentTypeMovie}
	}

	if ct.ContentType != model.ContentTypeTvShow {
		episodes = nil

		if year.IsNil() {
			title = ""
			rest = torrent.Name
		}
	}

	attrs := classification.ContentAttributes{
		ContentType:   ct,
		BaseTitle:     model.NullString{Valid: title != "", String: title},
		Date:          model.Date{Year: year},
		Episodes:      episodes,
		Languages:     model.InferLanguages(rest),
		LanguageMulti: multiRegex.MatchString(rest),
	}
	attrs.InferVideoAttributes(rest)

	return attrs, nil
}
