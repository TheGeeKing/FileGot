package metadata

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeMatroskaTagsUsesSpecTargetsAndPreservesExistingTags(t *testing.T) {
	tags := matroskaTags{Tags: []matroskaTag{{
		Targets: matroskaTargets{TypeValue: 30},
		Simple:  []matroskaSimple{{Name: "COMMENT", String: "keep"}},
	}}}
	mergeMatroskaTags(&tags, Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
		Genre: "Drama", LawRating: "TV-14",
		Directors: []string{"Dir"}, Writers: []string{"Writer"}, Actors: []string{"A", "B"},
	})
	got := matroskaTagMap(tags)
	for key, want := range map[string]string{
		"30:COMMENT": "keep",
		"70:TITLE":   "Show", "60:PART_NUMBER": "1",
		"50:TITLE": "Pilot", "50:ORIGINAL_TITLE": "Original Pilot",
		"50:COMMENT": "Story", "50:DATE_RELEASED": "2024-01-02",
		"50:GENRE": "Drama", "50:LAW_RATING": "TV-14", "50:TMDB_ID": "42",
		"50:PART_NUMBER":  "2",
		"50:DIRECTOR:Dir": "Dir", "50:WRITTEN_BY:Writer": "Writer",
		"50:ACTOR:A": "A", "50:ACTOR:B": "B",
	} {
		if got[key] != want {
			t.Errorf("%s = %q, want %q", key, got[key], want)
		}
	}
	if _, ok := got["30:DESCRIPTION"]; ok {
		t.Fatal("level-30 DESCRIPTION should not be written")
	}
	if _, ok := got["30:TVSHOW"]; ok {
		t.Fatal("level-30 TVSHOW should not be written")
	}
	byLevel := map[int]string{}
	for _, tag := range tags.Tags {
		level := targetValue(tag.Targets)
		if tag.Targets.TrackUID != 0 {
			continue
		}
		byLevel[level] = tag.Targets.Type
		if level == 30 {
			continue
		}
		for _, simple := range tag.Simple {
			if simple.LanguageIETF != "und" {
				t.Errorf("%s language = %q, want und", simple.Name, simple.LanguageIETF)
			}
		}
	}
	if byLevel[70] != "COLLECTION" || byLevel[60] != "SEASON" || byLevel[50] != "EPISODE" {
		t.Fatalf("target types = %#v", byLevel)
	}
}

func TestMergeMatroskaTagsMovieUsesLevel50Only(t *testing.T) {
	tags := matroskaTags{}
	mergeMatroskaTags(&tags, Values{
		Title: "Dune", Date: "2021-10-22", Overview: "Sand", Genre: "Sci-Fi", TMDBID: 7,
	})
	got := matroskaTagMap(tags)
	if got["50:TITLE"] != "Dune" || got["50:COMMENT"] != "Sand" || got["50:GENRE"] != "Sci-Fi" {
		t.Fatalf("movie tags = %#v", got)
	}
	if _, ok := got["70:TITLE"]; ok {
		t.Fatal("movies should not write collection TITLE")
	}
	if len(tags.Tags) != 1 || tags.Tags[0].Targets.Type != "MOVIE" {
		t.Fatalf("movie target = %#v", tags.Tags[0].Targets)
	}
}

func TestMergeMatroskaTagsUpgradesEmptyTargetsWithMovieType(t *testing.T) {
	tags := matroskaTags{Tags: []matroskaTag{{
		Targets: matroskaTargets{},
		Simple:  []matroskaSimple{{Name: "TITLE", String: "Old", LanguageIETF: "und"}},
	}}}
	mergeMatroskaTags(&tags, Values{Title: "New", Overview: "Body"})
	if len(tags.Tags) != 1 {
		t.Fatalf("tags = %#v", tags.Tags)
	}
	if tags.Tags[0].Targets.Type != "MOVIE" || tags.Tags[0].Targets.TypeValue != 50 {
		t.Fatalf("upgraded targets = %#v", tags.Tags[0].Targets)
	}
	got := matroskaTagMap(tags)
	if got["50:TITLE"] != "New" || got["50:COMMENT"] != "Body" {
		t.Fatalf("values = %#v", got)
	}
}

func TestWriteMKVInPlaceIntegration(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "mkvextract", "mkvmerge", "mkvpropedit"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	path := filepath.Join(t.TempDir(), "episode.mkv")
	if output, err := exec.Command(
		"ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=size=16x16:rate=1",
		"-t", "1", "-c:v", "ffv1", path,
	).CombinedOutput(); err != nil {
		t.Fatalf("create MKV: %v: %s", err, output)
	}
	values := Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
		Genre: "Drama", LawRating: "TV-14",
		Directors: []string{"Dir"}, Actors: []string{"Actor"},
	}
	writer := NewWriter()
	if different, err := writer.Differs(path, values); err != nil || !different {
		t.Fatalf("untagged MKV: different=%v err=%v", different, err)
	}
	if err := writer.WriteMKVInPlace(path, values); err != nil {
		t.Fatal(err)
	}
	if different, err := writer.Differs(path, values); err != nil || different {
		t.Fatalf("tagged MKV: different=%v err=%v", different, err)
	}
	extracted := filepath.Join(t.TempDir(), "tags.xml")
	if output, err := exec.Command("mkvextract", path, "tags", extracted).CombinedOutput(); err != nil {
		t.Fatalf("extract tags: %v: %s", err, output)
	}
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatal(err)
	}
	var tags matroskaTags
	if err := xml.Unmarshal(content, &tags); err != nil {
		t.Fatal(err)
	}
	got := matroskaTagMap(tags)
	if got["70:TITLE"] != "Show" || got["50:TITLE"] != "Pilot" ||
		got["60:PART_NUMBER"] != "1" || got["50:PART_NUMBER"] != "2" ||
		got["50:COMMENT"] != "Story" || got["50:GENRE"] != "Drama" ||
		got["50:LAW_RATING"] != "TV-14" || got["50:ACTOR:Actor"] != "Actor" {
		t.Fatalf("written tags = %#v", got)
	}
	foundEpisodeTarget := false
	for _, tag := range tags.Tags {
		if tag.Targets.TrackUID != 0 {
			continue
		}
		if targetValue(tag.Targets) == 50 {
			foundEpisodeTarget = true
			if tag.Targets.Type != "EPISODE" {
				t.Fatalf("level-50 TargetType = %q, want EPISODE (mkvpropedit strips value 50)", tag.Targets.Type)
			}
		}
		for _, simple := range tag.Simple {
			if strings.HasPrefix(simple.Name, "_STATISTICS_") || simple.Name == "BPS" ||
				simple.Name == "DURATION" || simple.Name == "NUMBER_OF_FRAMES" ||
				simple.Name == "NUMBER_OF_BYTES" {
				continue
			}
			if simple.LanguageIETF != "und" {
				t.Errorf("%s language = %q, want und", simple.Name, simple.LanguageIETF)
			}
		}
	}
	if !foundEpisodeTarget {
		t.Fatal("missing level-50 episode tags after extract")
	}
	identify, err := exec.Command("mkvmerge", "-J", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Container struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"container"`
	}
	if err := json.Unmarshal(identify, &info); err != nil {
		t.Fatal(err)
	}
	if info.Container.Properties.Title != "Pilot" {
		t.Fatalf("segment title = %q", info.Container.Properties.Title)
	}
}
