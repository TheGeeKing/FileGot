package matcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/settings"
	"github.com/TheGeeKing/FileGot/internal/tmdb"
)

func TestMatchMovieAndGroupedEpisodes(t *testing.T) {
	var tvSearches atomic.Int32
	var seasonLoads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/search/movie":
			_, _ = writer.Write([]byte(`{"results":[{"id":1,"title":"Dune Part Two","original_title":"Dune: Part Two","release_date":"2024-02-27"}]}`))
		case request.URL.Path == "/search/tv":
			tvSearches.Add(1)
			_, _ = writer.Write([]byte(`{"results":[{"id":2,"name":"The Last of Us","original_name":"The Last of Us","first_air_date":"2023-01-15"}]}`))
		case request.URL.Path == "/tv/2/season/1":
			seasonLoads.Add(1)
			_, _ = writer.Write([]byte(`{"episodes":[
				{"id":3,"name":"Episode one","season_number":1,"episode_number":1},
				{"id":4,"name":"Episode two","season_number":1,"episode_number":2}
			]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := tmdb.NewWithHTTPClient("token", server.URL, server.Client())
	engine := New(client)
	options := settings.Defaults()
	options.TMDBToken = "token"
	options.NamingMode = settings.NamingAdvanced
	options.MovieTemplate = `{n.upper()}`
	options.EpisodeTemplate = `{n}.{s00e00}.{t}`
	files := []media.File{
		{Path: "Dune.Part.Two.2024.mkv", Parsed: media.Parse("Dune.Part.Two.2024.mkv")},
		{Path: "The.Last.of.Us.S01E01.mkv", Parsed: media.Parse("The.Last.of.Us.S01E01.mkv")},
		{Path: "The.Last.of.Us.S01E02.mkv", Parsed: media.Parse("The.Last.of.Us.S01E02.mkv")},
	}

	got := engine.Match(context.Background(), files, options)
	for _, file := range got {
		if file.Status != media.Ready {
			t.Fatalf("%s status = %s (%s)", file.Path, file.Status, file.Message)
		}
	}
	if searches := tvSearches.Load(); searches != 1 {
		t.Fatalf("TV searched %d times, want 1", searches)
	}
	if loads := seasonLoads.Load(); loads != 1 {
		t.Fatalf("season loaded %d times, want 1", loads)
	}
	if got[0].Proposed != "DUNE PART TWO.mkv" || got[1].Proposed != "The Last of Us.S01E01.Episode one.mkv" {
		t.Fatalf("advanced names = %q and %q", got[0].Proposed, got[1].Proposed)
	}

	got = engine.Match(context.Background(), got, options)
	if tvSearches.Load() != 1 || seasonLoads.Load() != 1 {
		t.Fatalf("cached rematch made HTTP calls: searches=%d seasons=%d", tvSearches.Load(), seasonLoads.Load())
	}
}

func TestMatchFallsBackToShowFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/search/tv":
			if request.URL.Query().Get("query") == "Folder Show" {
				_, _ = writer.Write([]byte(`{"results":[{"id":2,"name":"Folder Show","original_name":"Folder Show"}]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"results":[{"id":1,"name":"Release","original_name":"Release"}]}`))
		case "/tv/2/season/1":
			_, _ = writer.Write([]byte(`{"episodes":[{"name":"Pilot","season_number":1,"episode_number":1}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := settings.Defaults()
	options.TMDBToken = "token"
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	path := filepath.Join("library", "Folder Show", "Season 1", "Release.S01E01.mkv")

	got := engine.Match(context.Background(), []media.File{{Path: path, Parsed: media.Parse(path)}}, options)
	if got[0].Status != media.Ready || got[0].Parsed.Query != "Folder Show" {
		t.Fatalf("folder fallback result = %#v", got[0])
	}
}

func TestMatchUsesFilenameAndParentIDs(t *testing.T) {
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/movie/123":
			_, _ = writer.Write([]byte(`{"id":123,"title":"Filename Movie","release_date":"2024-01-01"}`))
		case "/tv/456":
			_, _ = writer.Write([]byte(`{"id":456,"name":"Parent Show","first_air_date":"2020-01-01"}`))
		case "/tv/456/season/2":
			_, _ = writer.Write([]byte(`{"episodes":[{"name":"Exact Episode","season_number":2,"episode_number":3}]}`))
		case "/find/789":
			if request.URL.Query().Get("external_source") != "tvdb_id" {
				t.Fatalf("external source = %q", request.URL.Query().Get("external_source"))
			}
			_, _ = writer.Write([]byte(`{"tv_results":[{"id":456,"name":"Parent Show","first_air_date":"2020-01-01"}]}`))
		case "/find/tt7654321":
			if request.URL.Query().Get("external_source") != "imdb_id" {
				t.Fatalf("external source = %q", request.URL.Query().Get("external_source"))
			}
			_, _ = writer.Write([]byte(`{"movie_results":[{"id":321,"title":"IMDb Movie","release_date":"2022-01-01"}]}`))
		case "/search/movie":
			searches.Add(1)
			_, _ = writer.Write([]byte(`{"results":[{"id":7,"title":"Fallback","release_date":"2024-01-01"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := settings.Defaults()
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	files := []media.File{
		{Path: filepath.Join("Parent [tmdb-999]", "Movie [tmdb-123].mkv"), Parsed: media.Parsed{Kind: media.Movie}},
		{Path: filepath.Join("Show {tvdb-789}", "Season 2", "Episode.S02E03.mkv"), Parsed: media.Parsed{Kind: media.Episode, Season: 2, Episode: 3}},
		{Path: "Movie (tt7654321).mkv", Parsed: media.Parsed{Kind: media.Movie}},
		{Path: "Fallback [tmdb-nope].mkv", Parsed: media.Parsed{Kind: media.Movie, Query: "Fallback", Year: 2024}},
	}

	got := engine.Match(context.Background(), files, options)
	if got[0].Status != media.Ready || got[0].Candidate.ID != 123 {
		t.Fatalf("filename TMDB match = %#v", got[0])
	}
	if got[1].Status != media.Ready || got[1].Candidate.ID != 456 ||
		got[1].Candidate.Season != 2 || got[1].Candidate.Episode != 3 {
		t.Fatalf("parent TVDB match = %#v", got[1])
	}
	if got[2].Status != media.Ready || got[2].Candidate.ID != 321 {
		t.Fatalf("IMDb match = %#v", got[2])
	}
	if got[3].Status != media.Ready || searches.Load() != 1 {
		t.Fatalf("malformed fallback = %#v, searches=%d", got[3], searches.Load())
	}
}

func TestAmbiguousMovieRequiresReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[
			{"id":1,"title":"Crash","original_title":"Crash","release_date":"1996-07-17"},
			{"id":2,"title":"Crash","original_title":"Crash","release_date":"2004-09-10"}
		]}`))
	}))
	defer server.Close()

	options := settings.Defaults()
	options.TMDBToken = "token"
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	files := []media.File{{Path: "Crash.mkv", Parsed: media.Parse("Crash.mkv")}}
	got := engine.Match(context.Background(), files, options)
	if got[0].Status != media.Review || len(got[0].Candidates) != 2 {
		t.Fatalf("ambiguous result = %#v", got[0])
	}
}

func TestReviewGroupUsesParentHintAndKeepsPartialEpisodeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/tv/42/season/1" {
			_, _ = writer.Write([]byte(`{"episodes":[{"name":"Pilot","season_number":1,"episode_number":1}]}`))
			return
		}
		if request.URL.Path == "/tv/42/season/2" {
			_, _ = writer.Write([]byte(`{"episodes":[]}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	options := settings.Defaults()
	options.TMDBToken = "token"
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	files := []media.File{
		{Path: filepath.Join("Show Name (2019)", "Season 01", "Release.A.S01E01.mkv"), Parsed: media.Parsed{Kind: media.Episode, Query: "Release A", Season: 1, Episode: 1}},
		{Path: filepath.Join("Show Name (2019)", "Season 02", "Release.B.S02E02.mkv"), Parsed: media.Parsed{Kind: media.Episode, Query: "Release B", Season: 2, Episode: 2}},
		{Path: filepath.Join("Other Show", "Release.C.S01E01.mkv"), Parsed: media.Parsed{Kind: media.Episode, Query: "Release C", Season: 1, Episode: 1}},
	}

	indices := EpisodeGroupIndices(files, 0)
	if len(indices) != 2 {
		t.Fatalf("episode group = %v, want both files", indices)
	}
	got := engine.ResolveGroup(context.Background(), files, indices, media.Candidate{
		ID: 42, Kind: media.Episode, Title: "Show Name", SeriesYear: 2019,
	}, options)
	if got[0].Status != media.Ready || got[0].Candidate.Season != 1 || got[0].Candidate.Episode != 1 {
		t.Fatalf("resolved episode = %#v", got[0])
	}
	if got[1].Status != media.Error || got[1].Message != "TMDB season 2 has no episode 2" {
		t.Fatalf("unresolved episode = %#v", got[1])
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if cancelled := engine.ResolveGroup(ctx, files, indices, media.Candidate{ID: 42, Kind: media.Episode}, options); cancelled[0].Status != "" {
		t.Fatalf("cancelled group changed files: %#v", cancelled)
	}
}

func TestResolveEmbeddedMetadataEnrichmentIsBestEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/movie/1":
			_, _ = writer.Write([]byte(`{"id":1,"title":"Movie","genres":[{"id":1,"name":"Action"}]}`))
		case "/movie/1/release_dates":
			_, _ = writer.Write([]byte(`{"results":[]}`))
		case "/movie/1/credits":
			http.Error(writer, `{"status_message":"credits unavailable"}`, http.StatusBadGateway)
		case "/tv/2":
			_, _ = writer.Write([]byte(`{"id":2,"name":"Series","genres":[{"id":2,"name":"Drama"}]}`))
		case "/tv/2/content_ratings":
			http.Error(writer, `{"status_message":"ratings unavailable"}`, http.StatusBadGateway)
		case "/tv/2/credits":
			http.Error(writer, `{"status_message":"show credits unavailable"}`, http.StatusBadGateway)
		case "/tv/2/season/3":
			_, _ = writer.Write([]byte(`{"episodes":[{
				"id":24,"name":"Pilot","original_name":"",
				"season_number":3,"episode_number":4,"air_date":"2025-01-06","overview":"Story"
			}]}`))
		case "/tv/2/season/3/episode/4":
			http.Error(writer, `{"status_message":"original title unavailable"}`, http.StatusBadGateway)
		case "/tv/2/season/3/episode/4/credits":
			http.Error(writer, `{"status_message":"episode credits unavailable"}`, http.StatusBadGateway)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := settings.Defaults()
	options.WriteEmbeddedMetadata = true
	options.Language = "fr-FR"
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))

	movie := engine.Resolve(context.Background(), media.File{
		Path: "movie.mkv", Parsed: media.Parsed{Kind: media.Movie},
	}, media.Candidate{
		ID: 1, Kind: media.Movie, Title: "Movie", ReleaseDate: "2024-05-01", OriginalLanguage: "en",
	}, options)
	if movie.Status != media.Ready || movie.Proposed == "" {
		t.Fatalf("movie should stay ready when credits fail: %#v", movie)
	}
	if movie.Candidate.Genre != "Action" || len(movie.Candidate.Directors) != 0 {
		t.Fatalf("movie should keep genre and skip failed credits: %#v", movie.Candidate)
	}

	episode := engine.Resolve(context.Background(), media.File{
		Path: "episode.mkv", Parsed: media.Parsed{Kind: media.Episode, Season: 3, Episode: 4},
	}, media.Candidate{
		ID: 2, Kind: media.Episode, Title: "Series", OriginalLanguage: "en",
	}, options)
	if episode.Status != media.Ready || episode.Candidate.EpisodeTitle != "Pilot" {
		t.Fatalf("episode should stay ready when enrich calls fail: %#v", episode)
	}
	if episode.Candidate.Genre != "Drama" || episode.Candidate.LawRating != "" ||
		len(episode.Candidate.Directors) != 0 || len(episode.Candidate.Actors) != 0 ||
		episode.Candidate.OriginalEpisodeTitle != "" {
		t.Fatalf("episode should keep core fields and skip failed enrichments: %#v", episode.Candidate)
	}
}

func TestResolveLoadsEmbeddedMetadataDatesAndEpisodeDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/movie/1":
			_, _ = writer.Write([]byte(`{"id":1,"title":"Movie","genres":[{"id":1,"name":"Action"}]}`))
		case "/movie/1/release_dates":
			_, _ = writer.Write([]byte(`{"results":[{
				"iso_3166_1":"US","release_dates":[
					{"release_date":"2024-03-02T00:00:00.000Z","certification":"PG-13"},
					{"release_date":"2024-02-01T00:00:00.000Z","certification":"PG-13"}
				]
			}]}`))
		case "/movie/1/credits":
			_, _ = writer.Write([]byte(`{"cast":[{"name":"Actor One","order":0}],"crew":[
				{"name":"Dir","job":"Director"},{"name":"Write","job":"Writer"}
			]}`))
		case "/tv/2":
			_, _ = writer.Write([]byte(`{"id":2,"name":"Series","genres":[{"id":2,"name":"Drama"}]}`))
		case "/tv/2/content_ratings":
			_, _ = writer.Write([]byte(`{"results":[{"iso_3166_1":"US","rating":"TV-14"}]}`))
		case "/tv/2/credits":
			_, _ = writer.Write([]byte(`{"cast":[{"name":"Star","order":0}],"crew":[]}`))
		case "/tv/2/season/3":
			_, _ = writer.Write([]byte(`{"episodes":[{
				"id":24,"name":"Localized episode","original_name":"Original episode",
				"season_number":3,"episode_number":4,"air_date":"2025-01-06","overview":"Episode overview"
			}]}`))
		case "/tv/2/season/3/episode/4/credits":
			_, _ = writer.Write([]byte(`{"cast":[],"crew":[{"name":"Ep Dir","job":"Director"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := settings.Defaults()
	options.WriteEmbeddedMetadata = true
	engine := New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	movie := engine.Resolve(context.Background(), media.File{
		Path: "movie.mkv", Parsed: media.Parsed{Kind: media.Movie},
	}, media.Candidate{ID: 1, Kind: media.Movie, Title: "Movie", ReleaseDate: "2024-05-01"}, options)
	if movie.Status != media.Ready || movie.Candidate.ReleaseDate != "2024-02-01" ||
		movie.Candidate.Genre != "Action" || movie.Candidate.LawRating != "PG-13" ||
		len(movie.Candidate.Directors) != 1 || movie.Candidate.Directors[0] != "Dir" ||
		len(movie.Candidate.Actors) != 1 || movie.Candidate.Actors[0] != "Actor One" {
		t.Fatalf("movie metadata = %#v", movie.Candidate)
	}
	episode := engine.Resolve(context.Background(), media.File{
		Path: "episode.mkv", Parsed: media.Parsed{Kind: media.Episode, Season: 3, Episode: 4},
	}, media.Candidate{ID: 2, Kind: media.Episode, Title: "Series"}, options)
	if episode.Status != media.Ready || episode.Candidate.AirDate != "2025-01-06" ||
		episode.Candidate.Overview != "Episode overview" ||
		episode.Candidate.OriginalEpisodeTitle != "Original episode" ||
		episode.Candidate.Genre != "Drama" || episode.Candidate.LawRating != "TV-14" ||
		len(episode.Candidate.Directors) != 1 || episode.Candidate.Directors[0] != "Ep Dir" ||
		len(episode.Candidate.Actors) != 1 || episode.Candidate.Actors[0] != "Star" {
		t.Fatalf("episode metadata = %#v", episode.Candidate)
	}
}
