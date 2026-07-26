package media

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	tokenPattern     = regexp.MustCompile(`\{[a-z_]+\}`)
	whitespace       = regexp.MustCompile(`\s+`)
	reservedBasename = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)`)
)

var allowedTokens = map[Kind]map[string]struct{}{
	Movie: {
		"{title}": {}, "{year}": {}, "{tmdb_id}": {},
	},
	Episode: {
		"{series}": {}, "{series_year}": {}, "{season}": {}, "{episode}": {},
		"{episode_title}": {}, "{tmdb_id}": {},
	},
}

func ValidatePattern(kind Kind, pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("%s pattern cannot be empty", kind)
	}
	for _, token := range tokenPattern.FindAllString(pattern, -1) {
		if _, ok := allowedTokens[kind][token]; !ok {
			return fmt.Errorf("token %s is not valid for %s names", token, kind)
		}
	}
	if strings.Contains(pattern, "{") || strings.Contains(pattern, "}") {
		stripped := tokenPattern.ReplaceAllString(pattern, "")
		if strings.Contains(stripped, "{") || strings.Contains(stripped, "}") {
			return fmt.Errorf("invalid token syntax")
		}
	}
	return nil
}

func Format(pattern, originalPath string, candidate Candidate) (string, error) {
	if err := ValidatePattern(candidate.Kind, pattern); err != nil {
		return "", err
	}

	values := map[string]string{
		"{title}":         candidate.Title,
		"{year}":          number(candidate.Year),
		"{tmdb_id}":       number(candidate.ID),
		"{series}":        candidate.Title,
		"{series_year}":   number(candidate.SeriesYear),
		"{season}":        fmt.Sprintf("%02d", candidate.Season),
		"{episode}":       fmt.Sprintf("%02d", candidate.Episode),
		"{episode_title}": candidate.EpisodeTitle,
	}
	name := pattern
	for token, value := range values {
		name = strings.ReplaceAll(name, token, value)
	}
	if tokenPattern.MatchString(name) {
		return "", fmt.Errorf("pattern contains unresolved tokens")
	}

	return finishName(name, originalPath)
}

func finishName(name, originalPath string) (string, error) {
	extension := filepath.Ext(originalPath)
	name = Sanitize(name, 240-len([]rune(extension)))
	if name == "" {
		return "", fmt.Errorf("pattern produced an empty filename")
	}
	return name + extension, nil
}

func Sanitize(value string, maxRunes int) string {
	var builder strings.Builder
	for _, character := range value {
		if character < 32 || strings.ContainsRune(`<>:"/\|?*`, character) {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(character)
	}
	value = whitespace.ReplaceAllString(builder.String(), " ")
	value = strings.TrimRight(strings.TrimSpace(value), ". ")
	if reservedBasename.MatchString(value) {
		value = "_" + value
	}
	if maxRunes > 0 {
		runes := []rune(value)
		if len(runes) > maxRunes {
			runes = runes[:maxRunes]
			for len(runes) > 0 && unicode.IsSpace(runes[len(runes)-1]) {
				runes = runes[:len(runes)-1]
			}
			value = strings.TrimRight(string(runes), ". ")
		}
	}
	return value
}
