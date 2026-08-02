package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Values struct {
	Title         string
	OriginalTitle string
	Date          string
	Series        string
	Season        int
	Episode       int
	TMDBID        int
	Overview      string
	Genre         string
	LawRating     string
	Directors     []string
	Writers       []string
	Actors        []string
	IsEpisode     bool
}

// WriteFields selects which Values are written into a container.
type WriteFields struct {
	Title         bool
	OriginalTitle bool
	Comment       bool
	DateReleased  bool
	Genre         bool
	LawRating     bool
	Directors     bool
	Writers       bool
	Actors        bool
	TMDBID        bool
	SeriesInfo    bool
}

func AllWriteFields() WriteFields {
	return WriteFields{
		Title: true, OriginalTitle: true, Comment: true, DateReleased: true,
		Genre: true, LawRating: true, Directors: true, Writers: true,
		Actors: true, TMDBID: true, SeriesInfo: true,
	}
}

func (values Values) Filtered(fields WriteFields) Values {
	filtered := Values{IsEpisode: values.IsEpisode}
	if fields.Title {
		filtered.Title = values.Title
	}
	if fields.OriginalTitle {
		filtered.OriginalTitle = values.OriginalTitle
	}
	if fields.Comment {
		filtered.Overview = values.Overview
	}
	if fields.DateReleased {
		filtered.Date = values.Date
	}
	if fields.Genre {
		filtered.Genre = values.Genre
	}
	if fields.LawRating {
		filtered.LawRating = values.LawRating
	}
	if fields.Directors {
		filtered.Directors = append([]string(nil), values.Directors...)
	}
	if fields.Writers {
		filtered.Writers = append([]string(nil), values.Writers...)
	}
	if fields.Actors {
		filtered.Actors = append([]string(nil), values.Actors...)
	}
	if fields.TMDBID {
		filtered.TMDBID = values.TMDBID
	}
	if fields.SeriesInfo {
		filtered.Series = values.Series
		filtered.Season = values.Season
		filtered.Episode = values.Episode
	}
	return filtered
}

type Writer struct {
	run func(name string, args ...string) ([]byte, error)
}

func NewWriter() *Writer {
	return &Writer{run: func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}}
}

func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv", ".mp4", ".m4v", ".mov":
		return true
	default:
		return false
	}
}


func (writer *Writer) Differs(path string, values Values) (bool, error) {
	if strings.EqualFold(filepath.Ext(path), ".mkv") {
		return writer.mkvDiffers(path, values)
	}
	output, err := writer.run("ffprobe", "-v", "error", "-show_entries", "format_tags", "-of", "json", path)
	if err != nil {
		return false, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var probe struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return false, fmt.Errorf("parse ffprobe metadata: %w", err)
	}
	actual := make(map[string]string, len(probe.Format.Tags))
	for key, value := range probe.Format.Tags {
		actual[strings.ToLower(key)] = value
	}
	for _, entry := range ffmpegMetadataEntries(values) {
		if entry.value != "" && actual[entry.key] != entry.value {
			return true, nil
		}
	}
	return false, nil
}

// WriteInPlace writes metadata into the container at path without creating a
// second copy. MKV uses mkvpropedit; MP4/MOV/M4V use ffmpeg stream-copy to a
// temp file that replaces the original atomically.
func (writer *Writer) WriteInPlace(path string, values Values) error {
	if strings.EqualFold(filepath.Ext(path), ".mkv") {
		return writer.WriteMKVInPlace(path, values)
	}
	return writer.writeMP4InPlace(path, values)
}

func (writer *Writer) writeMP4InPlace(path string, values Values) error {
	tmp := path + ".filegot-tmp" + filepath.Ext(path)
	args := []string{"-v", "error", "-y", "-i", path, "-map", "0", "-c", "copy", "-map_metadata", "0"}
	args = append(args, "-movflags", "use_metadata_tags")
	for _, entry := range ffmpegMetadataEntries(values) {
		if entry.value != "" {
			args = append(args, "-metadata", entry.key+"="+entry.value)
		}
	}
	args = append(args, tmp)
	if output, err := writer.run("ffmpeg", args...); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace original: %w", err)
	}
	return nil
}

func ffmpegMetadataEntries(values Values) []struct{ key, value string } {
	return []struct{ key, value string }{
		{"title", values.Title},
		{"original_title", values.OriginalTitle},
		{"date", values.Date},
		{"show", values.Series},
		{"season_number", optionalInt(values.Season)},
		{"episode_sort", optionalInt(values.Episode)},
		{"tmdb_id", optionalInt(values.TMDBID)},
		{"synopsis", values.Overview},
		{"genre", values.Genre},
		{"rating", values.LawRating},
		{"director", strings.Join(values.Directors, "; ")},
		{"writer", strings.Join(values.Writers, "; ")},
		{"artist", strings.Join(values.Actors, "; ")},
	}
}

func optionalInt(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}
