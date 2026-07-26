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
	"time"
)

const defaultBaseURL = "https://api.themoviedb.org/3"

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type Movie struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
}

type Show struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
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

func (client *Client) Episode(ctx context.Context, seriesID, season, episode int, language string) (Episode, error) {
	var response Episode
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", seriesID, season, episode)
	err := client.get(ctx, path, url.Values{"language": {language}}, &response)
	return response, err
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
