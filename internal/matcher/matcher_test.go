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
			_, _ = writer.Write([]byte(`{"results":[{"id":1,"name":"Release Notes","original_name":"Release Notes"}]}`))
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
