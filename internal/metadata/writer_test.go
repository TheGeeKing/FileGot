package metadata

import (
	"context"
	"errors"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestWriteInPlaceMP4UsesFFmpegAndReplacesOriginal(t *testing.T) {
	dir := t.TempDir()
	source := dir + "/movie.mp4"
	if err := os.WriteFile(source, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	var got []string
	writer := &Writer{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = append([]string(nil), args...)
		return nil, os.WriteFile(args[len(args)-1], []byte("tagged"), 0o600)
	}}
	err := writer.WriteInPlace(source, Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
		IsEpisode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"copy", "title=Pilot", "original_title=Original Pilot", "date=2024-01-02",
		"show=Show", "season_number=1", "episode_sort=2", "tmdb_id=42", "synopsis=Story",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("ffmpeg args missing %q: %v", want, got)
		}
	}
	content, _ := os.ReadFile(source)
	if string(content) != "tagged" {
		t.Fatalf("original not replaced: %q", content)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 0640 preserved from original", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestFFmpegMetadataEntriesKeepsSeasonZeroForSpecials(t *testing.T) {
	entries := ffmpegMetadataEntries(Values{
		Title: "Special", Series: "Show", Season: 0, Episode: 1, IsEpisode: true,
	})
	var season string
	for _, entry := range entries {
		if entry.key == "season_number" {
			season = entry.value
		}
	}
	if season != "0" {
		t.Fatalf("season_number = %q, want 0 for specials", season)
	}
	movie := ffmpegMetadataEntries(Values{Title: "Movie", Season: 0})
	for _, entry := range movie {
		if entry.key == "season_number" && entry.value != "" {
			t.Fatalf("movie season_number = %q, want empty", entry.value)
		}
	}
}

func TestSupportedContainers(t *testing.T) {
	for _, path := range []string{"movie.mkv", "movie.mp4", "movie.m4v", "movie.mov"} {
		if !Supported(path) {
			t.Errorf("%s should support metadata", path)
		}
	}
	if Supported("movie.avi") {
		t.Fatal("AVI should remain unchanged")
	}
}

func TestValuesFilteredKeepsOnlySelectedFields(t *testing.T) {
	values := Values{
		Title: "T", OriginalTitle: "O", Date: "2024-01-02", Overview: "Story",
		Genre: "Drama", LawRating: "PG", TMDBID: 7, Series: "Show", Season: 1, Episode: 2,
		Directors: []string{"D"}, Writers: []string{"W"}, Actors: []string{"A"}, IsEpisode: true,
	}
	fields := WriteFields{Title: true, Comment: true}
	got := values.Filtered(fields)
	if got.Title != "T" || got.Overview != "Story" || got.OriginalTitle != "" ||
		got.Date != "" || got.Genre != "" || got.Series != "" || got.TMDBID != 0 ||
		len(got.Directors) != 0 || !got.IsEpisode {
		t.Fatalf("filtered = %#v", got)
	}
}

func TestDiffersComparesRequestedMetadata(t *testing.T) {
	writer := &Writer{run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"format":{"tags":{"TITLE":"Pilot","tmdb_id":"42"}}}`), nil
	}}
	different, err := writer.Differs("episode.mp4", Values{Title: "Pilot", TMDBID: 42})
	if err != nil || different {
		t.Fatalf("matching metadata: different=%v err=%v", different, err)
	}
	different, err = writer.Differs("episode.mp4", Values{Title: "Another", TMDBID: 42})
	if err != nil || !different {
		t.Fatalf("changed metadata: different=%v err=%v", different, err)
	}
}

func TestRunToolTimesOutHungCommands(t *testing.T) {
	writer := &Writer{
		timeout: 20 * time.Millisecond,
		run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	_, err := writer.runTool("hung-tool")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestToolTimeoutUsesRemuxBudgetForFFmpeg(t *testing.T) {
	writer := &Writer{}
	if got := writer.toolTimeout("ffmpeg"); got != defaultRemuxTimeout {
		t.Fatalf("ffmpeg timeout = %v, want %v", got, defaultRemuxTimeout)
	}
	if got := writer.toolTimeout("ffprobe"); got != defaultProbeTimeout {
		t.Fatalf("ffprobe timeout = %v, want %v", got, defaultProbeTimeout)
	}
	if got := writer.toolTimeout("mkvpropedit"); got != defaultProbeTimeout {
		t.Fatalf("mkvpropedit timeout = %v, want %v", got, defaultProbeTimeout)
	}
}
