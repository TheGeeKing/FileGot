package matcher

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/settings"
	"github.com/TheGeeKing/FileGot/internal/tmdb"
)

var nonAlphanumeric = regexp.MustCompile(`[^\pL\pN]+`)

type Matcher struct {
	client *tmdb.Client
}

func New(client *tmdb.Client) *Matcher {
	return &Matcher{client: client}
}

func (matcher *Matcher) Match(ctx context.Context, input []media.File, options settings.Settings) []media.File {
	files := append([]media.File(nil), input...)
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup

	var tvGroups [][]int
	var movieIndices []int
	grouped := make(map[int]bool)
	for index := range files {
		if files[index].Imported || files[index].Status == media.Unsupported {
			continue
		}
		if files[index].Parsed.Kind == media.Episode {
			if !grouped[index] {
				group := EpisodeGroupIndices(files, index)
				tvGroups = append(tvGroups, group)
				for _, member := range group {
					grouped[member] = true
				}
			}
			continue
		}
		movieIndices = append(movieIndices, index)
	}

	for _, index := range movieIndices {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			withLimit(ctx, semaphore, func() {
				candidates, err := matcher.Search(ctx, files[index].Parsed, options)
				if err != nil {
					setError(&files[index], err)
					return
				}
				applyCandidates(ctx, &files[index], candidates, options, matcher)
			})
		}(index)
	}

	for _, indices := range tvGroups {
		wait.Add(1)
		go func(indices []int) {
			defer wait.Done()
			candidates, selected, matchedParsed, automatic, err := matcher.searchEpisodeGroup(ctx, files, indices, options, semaphore)
			if automatic {
				for _, index := range indices {
					files[index].Parsed.Query = matchedParsed.Query
					files[index].Parsed.Year = matchedParsed.Year
					withLimit(ctx, semaphore, func() {
						files[index] = matcher.Resolve(ctx, files[index], selected, options)
					})
				}
				return
			}
			for _, index := range indices {
				files[index].Candidates = candidates
				if len(candidates) > 0 {
					files[index].Status = media.Review
					files[index].Message = "choose a TMDB series"
				} else if err != nil {
					setError(&files[index], err)
				} else {
					files[index].Status = media.Unmatched
					files[index].Message = "no TMDB results"
				}
			}
		}(indices)
	}

	wait.Wait()
	return files
}

func (matcher *Matcher) Search(ctx context.Context, parsed media.Parsed, options settings.Settings) ([]media.Candidate, error) {
	switch parsed.Kind {
	case media.Movie:
		results, err := matcher.client.SearchMovies(ctx, parsed.Query, parsed.Year, options.Language, options.IncludeAdult)
		if err != nil {
			return nil, err
		}
		candidates := make([]media.Candidate, 0, len(results))
		for _, result := range results {
			candidates = append(candidates, media.Candidate{
				ID: result.ID, Kind: media.Movie, Title: chooseTitle(result.Title, result.OriginalTitle, options.PreferOriginalTitle),
				OriginalTitle: result.OriginalTitle, Year: dateYear(result.ReleaseDate),
				PosterPath: result.PosterPath, Overview: result.Overview,
			})
		}
		return candidates, nil
	case media.Episode:
		results, err := matcher.client.SearchTV(ctx, parsed.Query, parsed.Year, options.Language, options.IncludeAdult)
		if err != nil {
			return nil, err
		}
		candidates := make([]media.Candidate, 0, len(results))
		for _, result := range results {
			candidates = append(candidates, media.Candidate{
				ID: result.ID, Kind: media.Episode, Title: chooseTitle(result.Name, result.OriginalName, options.PreferOriginalTitle),
				OriginalTitle: result.OriginalName, SeriesYear: dateYear(result.FirstAirDate),
				PosterPath: result.PosterPath, Overview: result.Overview,
				Season: parsed.Season, Episode: parsed.Episode,
			})
		}
		return candidates, nil
	default:
		return nil, fmt.Errorf("unsupported media kind %q", parsed.Kind)
	}
}

func (matcher *Matcher) ResolveGroup(
	ctx context.Context,
	input []media.File,
	indices []int,
	candidate media.Candidate,
	options settings.Settings,
) []media.File {
	files := append([]media.File(nil), input...)
	for _, index := range indices {
		if ctx.Err() != nil {
			return input
		}
		files[index] = matcher.Resolve(ctx, files[index], candidate, options)
	}
	if ctx.Err() != nil {
		return input
	}
	return files
}

func EpisodeGroupIndices(files []media.File, selected int) []int {
	if selected < 0 || selected >= len(files) || files[selected].Parsed.Kind != media.Episode {
		return nil
	}
	group := []int{selected}
	seen := map[int]bool{selected: true}
	keys := episodeKeys(files[selected])

	// ponytail: O(n²) grouping is fine for review-sized batches; index keys if imports grow past thousands.
	for changed := true; changed; {
		changed = false
		for index, file := range files {
			if seen[index] || file.Imported || file.Status == media.Unsupported || file.Parsed.Kind != media.Episode {
				continue
			}
			if !sharesKey(keys, episodeKeys(file)) {
				continue
			}
			seen[index] = true
			group = append(group, index)
			for key := range episodeKeys(file) {
				keys[key] = struct{}{}
			}
			changed = true
		}
	}
	return group
}

func (matcher *Matcher) searchEpisodeGroup(
	ctx context.Context,
	files []media.File,
	indices []int,
	options settings.Settings,
	semaphore chan struct{},
) ([]media.Candidate, media.Candidate, media.Parsed, bool, error) {
	seenQueries := make(map[string]struct{})
	seenCandidates := make(map[int]struct{})
	var candidates []media.Candidate
	var lastErr error
	for _, index := range indices {
		hints := append(media.ParentHints(files[index].Path), files[index].Parsed)
		for _, parsed := range hints {
			parsed.Kind = media.Episode
			parsed.Season = files[index].Parsed.Season
			parsed.Episode = files[index].Parsed.Episode
			key := parsedKey(parsed)
			if key == "" {
				continue
			}
			if _, seen := seenQueries[key]; seen {
				continue
			}
			seenQueries[key] = struct{}{}
			var found []media.Candidate
			withLimit(ctx, semaphore, func() {
				found, lastErr = matcher.Search(ctx, parsed, options)
			})
			if lastErr != nil {
				continue
			}
			if selected, automatic := automaticCandidate(parsed, found); automatic {
				return found, selected, parsed, true, nil
			}
			for _, candidate := range found {
				if _, seen := seenCandidates[candidate.ID]; !seen {
					seenCandidates[candidate.ID] = struct{}{}
					candidates = append(candidates, candidate)
				}
			}
		}
	}
	return candidates, media.Candidate{}, media.Parsed{}, false, lastErr
}

func episodeKeys(file media.File) map[string]struct{} {
	keys := make(map[string]struct{})
	if key := parsedKey(file.Parsed); key != "" {
		keys[key] = struct{}{}
	}
	hints := media.ParentHints(file.Path)
	if len(hints) > 0 {
		hint := hints[0]
		if key := parsedKey(hint); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func parsedKey(parsed media.Parsed) string {
	query := normalize(parsed.Query)
	if query == "" {
		return ""
	}
	return query + "/" + strconv.Itoa(parsed.Year)
}

func sharesKey(left, right map[string]struct{}) bool {
	for key := range left {
		if _, found := right[key]; found {
			return true
		}
	}
	return false
}

func (matcher *Matcher) Resolve(ctx context.Context, file media.File, candidate media.Candidate, options settings.Settings) media.File {
	file.Candidates = nil
	candidate.Season = file.Parsed.Season
	candidate.Episode = file.Parsed.Episode

	if candidate.Kind == media.Episode {
		episodes, err := matcher.client.SeasonEpisodes(ctx, candidate.ID, candidate.Season, options.Language)
		if err != nil {
			setError(&file, err)
			return file
		}
		var episode tmdb.Episode
		for _, current := range episodes {
			if current.EpisodeNumber == candidate.Episode {
				episode = current
				break
			}
		}
		if episode.EpisodeNumber == 0 {
			setError(&file, fmt.Errorf("TMDB season %d has no episode %d", candidate.Season, candidate.Episode))
			return file
		}
		candidate.EpisodeTitle = chooseTitle(episode.Name, episode.OriginalName, options.PreferOriginalTitle)
		if candidate.EpisodeTitle == "" {
			candidate.EpisodeTitle = fmt.Sprintf("Episode %02d", candidate.Episode)
		}
	}

	proposed, err := options.FormatName(file.Path, candidate)
	if err != nil {
		setError(&file, err)
		return file
	}
	file.Candidate = candidate
	file.Proposed = proposed
	file.Status = media.Ready
	file.Message = ""
	return file
}

func applyCandidates(ctx context.Context, file *media.File, candidates []media.Candidate, options settings.Settings, matcher *Matcher) {
	file.Candidates = candidates
	selected, automatic := automaticCandidate(file.Parsed, candidates)
	if !automatic {
		if len(candidates) == 0 {
			file.Status = media.Unmatched
			file.Message = "no TMDB results"
		} else {
			file.Status = media.Review
			file.Message = "choose a TMDB match"
		}
		return
	}
	*file = matcher.Resolve(ctx, *file, selected, options)
}

func automaticCandidate(parsed media.Parsed, candidates []media.Candidate) (media.Candidate, bool) {
	var exact []media.Candidate
	query := normalize(parsed.Query)
	for _, candidate := range candidates {
		titleMatches := normalize(candidate.Title) == query || normalize(candidate.OriginalTitle) == query
		year := candidate.Year
		if parsed.Kind == media.Episode {
			year = candidate.SeriesYear
		}
		yearMatches := parsed.Year == 0 || year == parsed.Year
		if titleMatches && yearMatches {
			exact = append(exact, candidate)
		}
	}
	return first(exact), len(exact) == 1
}

func normalize(value string) string {
	value = strings.ToLower(nonAlphanumeric.ReplaceAllString(value, " "))
	return strings.Join(strings.Fields(value), " ")
}

func dateYear(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func chooseTitle(localized, original string, preferOriginal bool) string {
	if preferOriginal && original != "" {
		return original
	}
	if localized != "" {
		return localized
	}
	return original
}

func setError(file *media.File, err error) {
	file.Status = media.Error
	file.Message = err.Error()
	file.Proposed = ""
}

func first(candidates []media.Candidate) media.Candidate {
	if len(candidates) == 0 {
		return media.Candidate{}
	}
	return candidates[0]
}

func withLimit(ctx context.Context, semaphore chan struct{}, work func()) {
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
		work()
	case <-ctx.Done():
	}
}

func Destination(file media.File) string {
	return filepath.Join(filepath.Dir(file.Path), file.Proposed)
}
