package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://api.themoviedb.org/3"

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	cacheMu    sync.RWMutex
	cache      map[string][]byte
	requests   sync.Map
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Movie struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	PosterPath    string  `json:"poster_path"`
	Overview      string  `json:"overview"`
	Genres        []Genre `json:"genres"`
}

type Show struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	OriginalName     string  `json:"original_name"`
	FirstAirDate     string  `json:"first_air_date"`
	PosterPath       string  `json:"poster_path"`
	Overview         string  `json:"overview"`
	OriginalLanguage string  `json:"original_language"`
	Genres           []Genre `json:"genres"`
}

type CastMember struct {
	Name      string `json:"name"`
	Order     int    `json:"order"`
	Character string `json:"character"`
}

type CrewMember struct {
	Name string `json:"name"`
	Job  string `json:"job"`
}

type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

type Season struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
}

type Episode struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	AirDate       string `json:"air_date"`
	Overview      string `json:"overview"`
}

type ReleaseDate struct {
	Date          string `json:"release_date"`
	Certification string `json:"certification"`
	Type          int    `json:"type"`
}

type Error struct {
	StatusCode int
	Message    string
}

func (err *Error) Error() string {
	if err.Message == "" {
		return fmt.Sprintf("TMDB returned HTTP %d", err.StatusCode)
	}
	return fmt.Sprintf("TMDB returned HTTP %d: %s", err.StatusCode, err.Message)
}

func New(token string) *Client {
	return NewWithHTTPClient(token, defaultBaseURL, &http.Client{Timeout: 15 * time.Second})
}

func NewWithHTTPClient(token, baseURL string, httpClient *http.Client) *Client {
	return &Client{
		token:      strings.TrimSpace(token),
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		cache:      make(map[string][]byte),
	}
}

func (client *Client) SearchMovies(ctx context.Context, query string, year int, language string, adult bool) ([]Movie, error) {
	values := url.Values{
		"query":         {query},
		"language":      {language},
		"include_adult": {strconv.FormatBool(adult)},
	}
	if year > 0 {
		values.Set("year", strconv.Itoa(year))
	}
	var response struct {
		Results []Movie `json:"results"`
	}
	if err := client.get(ctx, "/search/movie", values, &response); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (client *Client) SearchTV(ctx context.Context, query string, year int, language string, adult bool) ([]Show, error) {
	values := url.Values{
		"query":         {query},
		"language":      {language},
		"include_adult": {strconv.FormatBool(adult)},
	}
	if year > 0 {
		values.Set("first_air_date_year", strconv.Itoa(year))
	}
	var response struct {
		Results []Show `json:"results"`
	}
	if err := client.get(ctx, "/search/tv", values, &response); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (client *Client) MovieDetails(ctx context.Context, id int, language string) (Movie, error) {
	var movie Movie
	err := client.get(ctx, fmt.Sprintf("/movie/%d", id), url.Values{"language": {language}}, &movie)
	return movie, err
}

func (client *Client) MovieReleaseDates(ctx context.Context, id int, country string) ([]ReleaseDate, error) {
	var response struct {
		Results []struct {
			Country string        `json:"iso_3166_1"`
			Dates   []ReleaseDate `json:"release_dates"`
		} `json:"results"`
	}
	if err := client.get(ctx, fmt.Sprintf("/movie/%d/release_dates", id), nil, &response); err != nil {
		return nil, err
	}
	for _, result := range response.Results {
		if result.Country == country {
			return result.Dates, nil
		}
	}
	return nil, nil
}

func (client *Client) MovieCredits(ctx context.Context, id int) (Credits, error) {
	var credits Credits
	err := client.get(ctx, fmt.Sprintf("/movie/%d/credits", id), nil, &credits)
	return credits, err
}

func (client *Client) ShowDetails(ctx context.Context, id int, language string) (Show, error) {
	var show Show
	err := client.get(ctx, fmt.Sprintf("/tv/%d", id), url.Values{"language": {language}}, &show)
	return show, err
}

func (client *Client) ShowCredits(ctx context.Context, id int) (Credits, error) {
	var credits Credits
	err := client.get(ctx, fmt.Sprintf("/tv/%d/credits", id), nil, &credits)
	return credits, err
}

func (client *Client) EpisodeCredits(ctx context.Context, seriesID, season, episode int) (Credits, error) {
	var credits Credits
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d/credits", seriesID, season, episode)
	err := client.get(ctx, path, nil, &credits)
	return credits, err
}

func (client *Client) ShowContentRatings(ctx context.Context, id int, country string) (string, error) {
	var response struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	}
	if err := client.get(ctx, fmt.Sprintf("/tv/%d/content_ratings", id), nil, &response); err != nil {
		return "", err
	}
	for _, result := range response.Results {
		if result.Country == country && result.Rating != "" {
			return result.Rating, nil
		}
	}
	return "", nil
}

func (client *Client) Find(
	ctx context.Context,
	externalID, externalSource, language string,
) ([]Movie, []Show, error) {
	var response struct {
		Movies []Movie `json:"movie_results"`
		Shows  []Show  `json:"tv_results"`
	}
	values := url.Values{"external_source": {externalSource}, "language": {language}}
	err := client.get(ctx, "/find/"+url.PathEscape(externalID), values, &response)
	return response.Movies, response.Shows, err
}

func (client *Client) ShowSeasons(ctx context.Context, seriesID int, language string) ([]Season, error) {
	var response struct {
		Seasons []Season `json:"seasons"`
	}
	path := fmt.Sprintf("/tv/%d", seriesID)
	if err := client.get(ctx, path, url.Values{"language": {language}}, &response); err != nil {
		return nil, err
	}
	return response.Seasons, nil
}

func (client *Client) SeasonEpisodes(ctx context.Context, seriesID, season int, language string) ([]Episode, error) {
	var response struct {
		Episodes []Episode `json:"episodes"`
	}
	path := fmt.Sprintf("/tv/%d/season/%d", seriesID, season)
	if err := client.get(ctx, path, url.Values{"language": {language}}, &response); err != nil {
		return nil, err
	}
	return response.Episodes, nil
}

func (client *Client) EpisodeDetails(
	ctx context.Context,
	seriesID, season, episode int,
	language string,
) (Episode, error) {
	var result Episode
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", seriesID, season, episode)
	err := client.get(ctx, path, url.Values{"language": {language}}, &result)
	return result, err
}

func (client *Client) Translations(ctx context.Context) ([]string, error) {
	var response []string
	if err := client.get(ctx, "/configuration/primary_translations", nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (client *Client) get(ctx context.Context, path string, values url.Values, target any) error {
	if client.token == "" {
		return errors.New("TMDB Read Access Token is required")
	}
	endpoint := client.baseURL + path
	if len(values) > 0 {
		endpoint += "?" + values.Encode()
	}
	if found, err := client.cached(endpoint, target); found {
		return err
	}

	value, _ := client.requests.LoadOrStore(endpoint, make(chan struct{}, 1))
	request := value.(chan struct{})
	select {
	case request <- struct{}{}:
		defer func() { <-request }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if found, err := client.cached(endpoint, target); found {
		return err
	}

	var lastErr error
	for attempt := range 3 {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+client.token)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "FileGot/0.1")

		response, err := client.httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			if attempt < 2 {
				if err := wait(ctx, retryDelay(attempt)); err != nil {
					return err
				}
				continue
			}
			break
		}

		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		closeErr := response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			if err := json.Unmarshal(body, target); err != nil {
				return fmt.Errorf("decode TMDB response: %w", err)
			}
			client.cacheMu.Lock()
			client.cache[endpoint] = body
			client.cacheMu.Unlock()
			return nil
		}

		apiErr := &Error{StatusCode: response.StatusCode}
		var payload struct {
			StatusMessage string `json:"status_message"`
		}
		if json.Unmarshal(body, &payload) == nil {
			apiErr.Message = payload.StatusMessage
		}
		lastErr = apiErr

		if (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500) && attempt < 2 {
			delay := retryDelay(attempt)
			if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
			if err := wait(ctx, delay); err != nil {
				return err
			}
			continue
		}
		return apiErr
	}
	return lastErr
}

func (client *Client) cached(endpoint string, target any) (bool, error) {
	client.cacheMu.RLock()
	body, found := client.cache[endpoint]
	client.cacheMu.RUnlock()
	if !found {
		return false, nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return true, fmt.Errorf("decode cached TMDB response: %w", err)
	}
	return true, nil
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(attempt+1) * 250 * time.Millisecond
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
