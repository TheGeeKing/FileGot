package media

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	multiEpisodePattern  = regexp.MustCompile(`(?i)\bs(\d{1,2})[\s._-]*e(\d{1,3})(?:[\s._-]*e\d{1,3})+`)
	seasonEpisodePattern = regexp.MustCompile(`(?i)^(.*?)[\s._-]*s(\d{1,2})[\s._-]*e(\d{1,3})(?:\b|[\s._-])`)
	xEpisodePattern      = regexp.MustCompile(`(?i)^(.*?)[\s._-]+(\d{1,2})x(\d{1,3})(?:\b|[\s._-])`)
	yearPattern          = regexp.MustCompile(`(?:^|[\s._([{-])((?:19|20)\d{2})(?:$|[\s._)\]}-])`)
	bracketPattern       = regexp.MustCompile(`[\[\{][^\]\}]*[\]\}]`)
	spacePattern         = regexp.MustCompile(`\s+`)
)

var releaseTags = map[string]struct{}{
	"2160p": {}, "1080p": {}, "720p": {}, "480p": {},
	"bluray": {}, "bdrip": {}, "brrip": {}, "webrip": {}, "webdl": {}, "web": {},
	"hdtv": {}, "dvdrip": {}, "remux": {}, "x264": {}, "x265": {}, "h264": {},
	"h265": {}, "hevc": {}, "av1": {}, "hdr": {}, "dv": {}, "aac": {}, "dts": {},
	"truehd": {}, "atmos": {},
}

func Parse(path string) Parsed {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	if match := multiEpisodePattern.FindStringSubmatch(base); match != nil {
		query, year := titleAndYear(base[:strings.Index(strings.ToLower(base), strings.ToLower(match[0]))])
		return Parsed{
			Kind:         Episode,
			Query:        query,
			Year:         year,
			Season:       atoi(match[1]),
			Episode:      atoi(match[2]),
			MultiEpisode: true,
		}
	}

	for _, pattern := range []*regexp.Regexp{seasonEpisodePattern, xEpisodePattern} {
		if match := pattern.FindStringSubmatch(base); match != nil {
			query, year := titleAndYear(match[1])
			return Parsed{
				Kind:    Episode,
				Query:   query,
				Year:    year,
				Season:  atoi(match[2]),
				Episode: atoi(match[3]),
			}
		}
	}

	query, year := titleAndYear(base)
	return Parsed{Kind: Movie, Query: query, Year: year}
}

func titleAndYear(value string) (string, int) {
	year := 0
	if match := yearPattern.FindStringSubmatch(value); match != nil {
		candidate := atoi(match[1])
		if candidate <= time.Now().Year()+1 {
			year = candidate
			value = strings.Replace(value, match[1], " ", 1)
		}
	}
	return cleanTitle(value), year
}

func cleanTitle(value string) string {
	value = bracketPattern.ReplaceAllString(value, " ")
	value = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(value)

	words := strings.Fields(value)
	for i, word := range words {
		normalized := strings.ToLower(strings.Trim(word, " ._-"))
		if _, found := releaseTags[normalized]; found {
			words = words[:i]
			break
		}
	}

	return strings.TrimSpace(spacePattern.ReplaceAllString(strings.Join(words, " "), " "))
}

func atoi(value string) int {
	number, _ := strconv.Atoi(value)
	return number
}
