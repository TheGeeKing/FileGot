package media

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Parsed
	}{
		{
			name: "movie with year and release tags",
			path: "Dune.Part.Two.2024.2160p.WEB-DL.mkv",
			want: Parsed{Kind: Movie, Query: "Dune Part Two", Year: 2024},
		},
		{
			name: "season episode",
			path: "The.Last.of.Us.S01E03.1080p.mkv",
			want: Parsed{Kind: Episode, Query: "The Last of Us", Season: 1, Episode: 3},
		},
		{
			name: "show name from parent folders",
			path: filepath.Join("library", "13 Reasons Why", "Saison 1", "S01E03.mkv"),
			want: Parsed{Kind: Episode, Query: "13 Reasons Why", Season: 1, Episode: 3},
		},
		{
			name: "series year",
			path: "Doctor.Who.2005.S01E03.1080p.mkv",
			want: Parsed{Kind: Episode, Query: "Doctor Who", Year: 2005, Season: 1, Episode: 3},
		},
		{
			name: "x episode",
			path: "Spaced 2x04 HDTV.avi",
			want: Parsed{Kind: Episode, Query: "Spaced", Season: 2, Episode: 4},
		},
		{
			name: "multi episode",
			path: "Show.S01E01E02.mkv",
			want: Parsed{Kind: Episode, Query: "Show", Season: 1, Episode: 1, MultiEpisode: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Parse(test.path); got != test.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", test.path, got, test.want)
			}
		})
	}
}

func TestFormatAndSanitize(t *testing.T) {
	movie := Candidate{ID: 438631, Kind: Movie, Title: `Dune: Part Two`, Year: 2024}
	got, err := Format("{title} ({year}) [tmdb-{tmdb_id}]", "release.MKV", movie)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two (2024) [tmdb-438631].MKV"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}

	episode := Candidate{
		ID: 1399, Kind: Episode, Title: "Game of Thrones", SeriesYear: 2011,
		Season: 1, Episode: 2, EpisodeTitle: "The Kingsroad",
	}
	got, err = Format("{series} - S{season}E{episode} - {episode_title}", "episode.mkv", episode)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Game of Thrones - S01E02 - The Kingsroad.mkv"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}

	if got := Sanitize("CON", 240); got != "_CON" {
		t.Fatalf("Sanitize(CON) = %q", got)
	}
}

func TestValidatePattern(t *testing.T) {
	if err := ValidatePattern(Movie, "{title} ({season})"); err == nil {
		t.Fatal("movie pattern accepted episode token")
	}
	if err := ValidatePattern(Episode, "{series} S{season}E{episode}"); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "Season 01")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "Movie.2024.mkv"),
		filepath.Join(root, "notes.txt"),
		filepath.Join(nested, "Show.S01E01.mp4"),
	} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := Discover([]string{root}, DiscoverOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("Discover() found %d files, want 2", len(files))
	}

	files, err = Discover([]string{root}, DiscoverOptions{Recursive: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("non-recursive Discover() found %d files, want 1", len(files))
	}

	if runtime.GOOS == "windows" {
		files, err = Discover([]string{filepath.Join(root, "Movie.2024.mkv"), filepath.Join(root, "movie.2024.MKV")}, DiscoverOptions{})
		if err == nil && len(files) > 1 {
			t.Fatalf("Windows discovery should deduplicate case-insensitively")
		}
	}
}
