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

	tvGroups := make(map[string][]int)
	for index := range files {
		if files[index].Imported || files[index].Status == media.Unsupported {
			continue
		}
		if files[index].Parsed.Kind == media.Episode {
			key := normalize(files[index].Parsed.Query) + "/" + strconv.Itoa(files[index].Parsed.Year)
			tvGroups[key] = append(tvGroups[key], index)
			continue
		}

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
			first := files[indices[0]].Parsed
			var candidates []media.Candidate
			var err error
			withLimit(ctx, semaphore, func() {
				candidates, err = matcher.Search(ctx, first, options)
			})
			if err != nil {
				for _, index := range indices {
					setError(&files[index], err)
				}
				return
			}

			selected, automatic := automaticCandidate(first, candidates)
			if !automatic {
				for _, index := range indices {
					parsed := files[index].Parsed
					fileCandidates := candidates
					var fallbackSelected media.Candidate
					var fallbackAutomatic bool
					var fallbackErr error
					seen := map[string]struct{}{
						normalize(parsed.Query) + "/" + strconv.Itoa(parsed.Year): {},
					}
					for _, hint := range media.ParentHints(files[index].Path) {
						fallback := parsed
						fallback.Query = hint.Query
						if hint.Year != 0 {
							fallback.Year = hint.Year
						}
						key := normalize(fallback.Query) + "/" + strconv.Itoa(fallback.Year)
						if _, duplicate := seen[key]; duplicate {
							continue
						}
						seen[key] = struct{}{}
						found, err := matcher.Search(ctx, fallback, options)
						if err != nil {
							fallbackErr = err
							continue
						}
						fallbackErr = nil
						if len(found) > 0 {
							fileCandidates = found
						}
						if fallbackSelected, fallbackAutomatic = automaticCandidate(fallback, found); fallbackAutomatic {
							files[index].Parsed.Query = fallback.Query
							files[index].Parsed.Year = fallback.Year
							files[index] = matcher.Resolve(ctx, files[index], fallbackSelected, options)
							break
						}
					}
					if fallbackAutomatic {
						continue
					}
					if len(fileCandidates) == 0 && fallbackErr != nil {
						setError(&files[index], fallbackErr)
						continue
					}
					files[index].Candidates = fileCandidates
					if len(fileCandidates) == 0 {
						files[index].Status = media.Unmatched
						files[index].Message = "no TMDB results"
					} else {
						files[index].Status = media.Review
						files[index].Message = "choose a TMDB series"
					}
				}
				return
			}

			var episodeWait sync.WaitGroup
			for _, index := range indices {
				index := index
				episodeWait.Add(1)
				go func() {
					defer episodeWait.Done()
					withLimit(ctx, semaphore, func() {
						files[index] = matcher.Resolve(ctx, files[index], selected, options)
					})
				}()
			}
			episodeWait.Wait()
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
				Season: parsed.Season, Episode: parsed.Episode,
			})
		}
		return candidates, nil
	default:
		return nil, fmt.Errorf("unsupported media kind %q", parsed.Kind)
	}
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
