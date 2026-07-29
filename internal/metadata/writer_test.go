package metadata

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestWriterUsesStreamCopyAndMetadata(t *testing.T) {
	var got []string
	writer := &Writer{run: func(_ string, args ...string) ([]byte, error) {
		got = append([]string(nil), args...)
		return nil, os.WriteFile(args[len(args)-1], []byte("tagged"), 0o600)
	}}
	output := t.TempDir() + "/tagged.mp4"
	err := writer.Write("episode.mp4", output, Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
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
	if !WritesInPlace("movie.mkv") || WritesInPlace("movie.mp4") {
		t.Fatal("only MKV should write metadata in place")
	}
}

func TestWriteRejectsMKVFullCopy(t *testing.T) {
	writer := NewWriter()
	err := writer.Write("in.mkv", "out.mkv", Values{Title: "T"})
	if err == nil || !strings.Contains(err.Error(), "WriteMKVInPlace") {
		t.Fatalf("Write MKV = %v", err)
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
	writer := &Writer{run: func(_ string, _ ...string) ([]byte, error) {
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
