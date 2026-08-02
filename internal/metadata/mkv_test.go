package metadata

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMKVDiffersDetectsSegmentTitleMismatch(t *testing.T) {
	writer := &Writer{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "mkvextract":
			path := args[2]
			content := []byte(`<?xml version="1.0"?><Tags><Tag><Targets><TargetTypeValue>50</TargetTypeValue><TargetType>EPISODE</TargetType></Targets><Simple><Name>TITLE</Name><String>Pilot</String><TagLanguageIETF>und</TagLanguageIETF></Simple></Tag></Tags>`)
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return nil, err
			}
			return nil, nil
		case "mkvmerge":
			return []byte(`{"container":{"properties":{"title":"Old Title"}}}`), nil
		default:
			t.Fatalf("unexpected tool %s", name)
			return nil, nil
		}
	}}
	different, err := writer.Differs("episode.mkv", Values{Title: "Pilot", IsEpisode: true})
	if err != nil || !different {
		t.Fatalf("segment title mismatch: different=%v err=%v", different, err)
	}
}

func TestExpectedMatroskaTagsIgnoresSeriesOnMovies(t *testing.T) {
	got := expectedMatroskaTags(Values{Title: "Dune", Series: "ShouldNotMatter", Season: 1, Episode: 2})
	if _, ok := got["70:TITLE"]; ok {
		t.Fatalf("movie expectations = %#v, series level must stay episode-only", got)
	}
	if _, ok := got["60:PART_NUMBER"]; ok {
		t.Fatalf("movie expectations = %#v, season must stay episode-only", got)
	}
	episode := expectedMatroskaTags(Values{
		Title: "Pilot", Series: "Show", Season: 1, Episode: 2, IsEpisode: true,
	})
	if episode["70:TITLE"] != "Show" || episode["60:PART_NUMBER"] != "1" || episode["50:PART_NUMBER"] != "2" {
		t.Fatalf("episode expectations = %#v", episode)
	}
	special := expectedMatroskaTags(Values{
		Title: "Special", Series: "Show", Season: 0, Episode: 1, IsEpisode: true,
	})
	if special["60:PART_NUMBER"] != "0" {
		t.Fatalf("specials season = %#v, want PART_NUMBER 0", special)
	}
}

func TestMergeMatroskaTagsUsesSpecTargetsAndPreservesExistingTags(t *testing.T) {
	tags := matroskaTags{Tags: []matroskaTag{{
		Targets: matroskaTargets{TypeValue: 30},
		Simple:  []matroskaSimple{{Name: "COMMENT", String: "keep"}},
	}}}
	mergeMatroskaTags(&tags, Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
		Genre: "Drama", LawRating: "TV-14", IsEpisode: true,
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

func TestMergeMatroskaTagsStripsTrackUIDTags(t *testing.T) {
	tags := matroskaTags{Tags: []matroskaTag{{
		Targets: matroskaTargets{TrackUID: 99},
		Simple:  []matroskaSimple{{Name: "BPS", String: "1000"}, {Name: "_STATISTICS_TAGS", String: "BPS"}},
	}, {
		Targets: matroskaTargets{TypeValue: 30},
		Simple:  []matroskaSimple{{Name: "COMMENT", String: "keep"}},
	}}}
	mergeMatroskaTags(&tags, Values{Title: "Pilot", Genre: "Drama", IsEpisode: true, Series: "Show", Season: 1, Episode: 1})
	for _, tag := range tags.Tags {
		if tag.Targets.TrackUID != 0 {
			t.Fatalf("TrackUID tag kept: %#v", tag)
		}
	}
	got := matroskaTagMap(tags)
	if got["30:COMMENT"] != "keep" || got["50:GENRE"] != "Drama" {
		t.Fatalf("tags = %#v", got)
	}
}

func TestMergeMatroskaTagsKeepsTrackUIDWhenWindowsFieldsAbsent(t *testing.T) {
	tags := matroskaTags{Tags: []matroskaTag{{
		Targets: matroskaTargets{TrackUID: 99},
		Simple:  []matroskaSimple{{Name: "BPS", String: "1000"}},
	}}}
	mergeMatroskaTags(&tags, Values{Title: "Pilot", TMDBID: 42, IsEpisode: true, Series: "Show", Season: 1, Episode: 1})
	found := false
	for _, tag := range tags.Tags {
		if tag.Targets.TrackUID == 99 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("TrackUID statistics should remain when Genre/Comment/Date are not written")
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
	// Seed TrackUID statistics like mkvmerge leaves behind; Windows Explorer ignores
	// GENRE/COMMENT/DATE_RELEASED while these remain, so WriteMKVInPlace must strip them.
	seedTags := filepath.Join(t.TempDir(), "seed-tags.xml")
	if err := os.WriteFile(seedTags, []byte(`<?xml version="1.0"?>
<Tags>
  <Tag>
    <Targets><TrackUID>1</TrackUID></Targets>
    <Simple><Name>BPS</Name><String>1000</String></Simple>
  </Tag>
</Tags>
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("mkvpropedit", path, "--tags", "all:"+seedTags).CombinedOutput(); err != nil {
		t.Fatalf("seed track tags: %v: %s", err, output)
	}
	values := Values{
		Title: "Pilot", OriginalTitle: "Original Pilot", Date: "2024-01-02",
		Series: "Show", Season: 1, Episode: 2, TMDBID: 42, Overview: "Story",
		Genre: "Drama", LawRating: "TV-14", IsEpisode: true,
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
			t.Fatalf("TrackUID tag should be stripped for Windows Explorer: %#v", tag.Targets)
		}
		if targetValue(tag.Targets) == 50 {
			foundEpisodeTarget = true
			if tag.Targets.Type != "EPISODE" {
				t.Fatalf("level-50 TargetType = %q, want EPISODE (mkvpropedit strips value 50)", tag.Targets.Type)
			}
		}
		for _, simple := range tag.Simple {
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
