package metadata

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type matroskaTags struct {
	XMLName xml.Name      `xml:"Tags"`
	Tags    []matroskaTag `xml:"Tag"`
}

type matroskaTag struct {
	Targets matroskaTargets  `xml:"Targets"`
	Simple  []matroskaSimple `xml:"Simple"`
}

type matroskaTargets struct {
	TypeValue     int    `xml:"TargetTypeValue,omitempty"`
	Type          string `xml:"TargetType,omitempty"`
	TrackUID      uint64 `xml:"TrackUID,omitempty"`
	EditionUID    uint64 `xml:"EditionUID,omitempty"`
	ChapterUID    uint64 `xml:"ChapterUID,omitempty"`
	AttachmentUID uint64 `xml:"AttachmentUID,omitempty"`
}

type matroskaSimple struct {
	Name            string           `xml:"Name"`
	String          string           `xml:"String,omitempty"`
	Binary          string           `xml:"Binary,omitempty"`
	Language        string           `xml:"TagLanguage,omitempty"`
	LanguageIETF    string           `xml:"TagLanguageIETF,omitempty"`
	DefaultLanguage string           `xml:"TagDefault,omitempty"`
	Simple          []matroskaSimple `xml:"Simple,omitempty"`
}

func (writer *Writer) WriteMKVInPlace(path string, values Values) error {
	backup, err := os.CreateTemp("", "filegot-mkv-tags-backup-*.xml")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	_ = backup.Close()
	_ = os.Remove(backupPath)
	defer os.Remove(backupPath)

	hadTags := true
	if output, extractErr := writer.run("mkvextract", path, "tags", backupPath); extractErr != nil {
		return fmt.Errorf("mkvextract: %w: %s", extractErr, strings.TrimSpace(string(output)))
	} else if _, statErr := os.Stat(backupPath); os.IsNotExist(statErr) {
		hadTags = false
	}

	current := matroskaTags{}
	if hadTags {
		content, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return readErr
		}
		if unmarshalErr := xml.Unmarshal(content, &current); unmarshalErr != nil {
			return fmt.Errorf("parse existing Matroska tags: %w", unmarshalErr)
		}
	}
	mergeMatroskaTags(&current, values)

	update, err := os.CreateTemp("", "filegot-mkv-tags-update-*.xml")
	if err != nil {
		return err
	}
	updatePath := update.Name()
	defer os.Remove(updatePath)
	content, err := xml.MarshalIndent(current, "", "  ")
	if err == nil {
		_, err = update.Write(append([]byte(xml.Header), content...))
	}
	closeErr := update.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	if output, editErr := writer.run("mkvpropedit", path, "--tags", "all:"+updatePath); editErr != nil {
		restore := "all:"
		if hadTags {
			restore += backupPath
		}
		restoreOutput, restoreErr := writer.run("mkvpropedit", path, "--tags", restore)
		return fmt.Errorf(
			"mkvpropedit: %w: %s; restore: %v: %s",
			editErr, strings.TrimSpace(string(output)), restoreErr, strings.TrimSpace(string(restoreOutput)),
		)
	}
	if values.Title != "" {
		if output, editErr := writer.run("mkvpropedit", path, "--edit", "info", "--set", "title="+values.Title); editErr != nil {
			restore := "all:"
			if hadTags {
				restore += backupPath
			}
			restoreOutput, restoreErr := writer.run("mkvpropedit", path, "--tags", restore)
			return fmt.Errorf(
				"mkvpropedit title: %w: %s; restore tags: %v: %s",
				editErr, strings.TrimSpace(string(output)), restoreErr, strings.TrimSpace(string(restoreOutput)),
			)
		}
	}
	return nil
}

func (writer *Writer) mkvDiffers(path string, values Values) (bool, error) {
	extracted, err := os.CreateTemp("", "filegot-mkv-tags-read-*.xml")
	if err != nil {
		return false, err
	}
	extractedPath := extracted.Name()
	_ = extracted.Close()
	_ = os.Remove(extractedPath)
	defer os.Remove(extractedPath)
	if output, extractErr := writer.run("mkvextract", path, "tags", extractedPath); extractErr != nil {
		return false, fmt.Errorf("mkvextract: %w: %s", extractErr, strings.TrimSpace(string(output)))
	}
	tags := matroskaTags{}
	if content, readErr := os.ReadFile(extractedPath); readErr == nil {
		if unmarshalErr := xml.Unmarshal(content, &tags); unmarshalErr != nil {
			return false, fmt.Errorf("parse Matroska tags: %w", unmarshalErr)
		}
	} else if !os.IsNotExist(readErr) {
		return false, readErr
	}
	actual := matroskaTagMap(tags)
	for key, expected := range expectedMatroskaTags(values) {
		if expected != "" && actual[key] != expected {
			return true, nil
		}
	}
	return false, nil
}

func mergeMatroskaTags(tags *matroskaTags, values Values) {
	// Windows Explorer's MKV property handler ignores GENRE/COMMENT/DATE_RELEASED
	// when any TrackUID-targeted tags (usually mkvmerge statistics) remain.
	if needsTrackUIDStrip(values) {
		stripTrackTargetedTags(tags)
	}
	episode := values.IsEpisode
	if episode {
		upsertMatroska(tags, 70, "TITLE", values.Series, episode)
		upsertMatroska(tags, 60, "PART_NUMBER", optionalInt(values.Season), episode)
		upsertMatroska(tags, 50, "TITLE", values.Title, episode)
		upsertMatroska(tags, 50, "PART_NUMBER", optionalInt(values.Episode), episode)
	} else {
		upsertMatroska(tags, 50, "TITLE", values.Title, episode)
	}
	upsertMatroska(tags, 50, "ORIGINAL_TITLE", values.OriginalTitle, episode)
	upsertMatroska(tags, 50, "COMMENT", values.Overview, episode)
	upsertMatroska(tags, 50, "DATE_RELEASED", values.Date, episode)
	upsertMatroska(tags, 50, "GENRE", values.Genre, episode)
	upsertMatroska(tags, 50, "LAW_RATING", values.LawRating, episode)
	upsertMatroska(tags, 50, "TMDB_ID", optionalInt(values.TMDBID), episode)
	replaceMatroskaList(tags, 50, "DIRECTOR", values.Directors, episode)
	replaceMatroskaList(tags, 50, "WRITTEN_BY", values.Writers, episode)
	replaceMatroskaList(tags, 50, "ACTOR", values.Actors, episode)
}

func needsTrackUIDStrip(values Values) bool {
	return values.Genre != "" || values.Overview != "" || values.Date != ""
}

func expectedMatroskaTags(values Values) map[string]string {
	expected := map[string]string{
		"50:TITLE":          values.Title,
		"50:ORIGINAL_TITLE": values.OriginalTitle,
		"50:COMMENT":        values.Overview,
		"50:DATE_RELEASED":  values.Date,
		"50:GENRE":          values.Genre,
		"50:LAW_RATING":     values.LawRating,
		"50:TMDB_ID":        optionalInt(values.TMDBID),
	}
	if values.Series != "" {
		expected["70:TITLE"] = values.Series
		expected["60:PART_NUMBER"] = optionalInt(values.Season)
		expected["50:PART_NUMBER"] = optionalInt(values.Episode)
	} else if values.IsEpisode {
		expected["60:PART_NUMBER"] = optionalInt(values.Season)
		expected["50:PART_NUMBER"] = optionalInt(values.Episode)
	}
	for _, name := range values.Directors {
		if name != "" {
			expected["50:DIRECTOR:"+name] = name
		}
	}
	for _, name := range values.Writers {
		if name != "" {
			expected["50:WRITTEN_BY:"+name] = name
		}
	}
	for _, name := range values.Actors {
		if name != "" {
			expected["50:ACTOR:"+name] = name
		}
	}
	return expected
}

func stripTrackTargetedTags(tags *matroskaTags) {
	kept := tags.Tags[:0]
	for _, tag := range tags.Tags {
		if tag.Targets.TrackUID != 0 {
			continue
		}
		kept = append(kept, tag)
	}
	tags.Tags = kept
}

func upsertMatroska(tags *matroskaTags, target int, name, value string, episode bool) {
	if value == "" {
		return
	}
	targetTag := ensureMatroskaTarget(tags, target, episode)
	for simpleIndex := range tags.Tags[targetTag].Simple {
		if strings.EqualFold(tags.Tags[targetTag].Simple[simpleIndex].Name, name) {
			tags.Tags[targetTag].Simple[simpleIndex].String = value
			tags.Tags[targetTag].Simple[simpleIndex].LanguageIETF = "und"
			return
		}
	}
	tags.Tags[targetTag].Simple = append(
		tags.Tags[targetTag].Simple,
		matroskaSimple{Name: name, String: value, LanguageIETF: "und"},
	)
}

func replaceMatroskaList(tags *matroskaTags, target int, name string, values []string, episode bool) {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			filtered = append(filtered, strings.TrimSpace(value))
		}
	}
	if len(filtered) == 0 {
		return
	}
	targetTag := ensureMatroskaTarget(tags, target, episode)
	kept := tags.Tags[targetTag].Simple[:0]
	for _, simple := range tags.Tags[targetTag].Simple {
		if !strings.EqualFold(simple.Name, name) {
			kept = append(kept, simple)
		}
	}
	for _, value := range filtered {
		kept = append(kept, matroskaSimple{Name: name, String: value, LanguageIETF: "und"})
	}
	tags.Tags[targetTag].Simple = kept
}

// ensureMatroskaTarget finds or creates a non-UID tag group for the Matroska level.
// TargetType is always set: mkvpropedit strips TargetTypeValue 50 (the default), and
// Windows Explorer needs an explicit TargetType (e.g. MOVIE) to surface COMMENT/GENRE.
func ensureMatroskaTarget(tags *matroskaTags, target int, episode bool) int {
	typeName := matroskaTargetType(target, episode)
	for tagIndex := range tags.Tags {
		tag := &tags.Tags[tagIndex]
		if targetValue(tag.Targets) != target || tag.Targets.TrackUID != 0 ||
			tag.Targets.EditionUID != 0 || tag.Targets.ChapterUID != 0 || tag.Targets.AttachmentUID != 0 {
			continue
		}
		tag.Targets.TypeValue = target
		tag.Targets.Type = typeName
		return tagIndex
	}
	tags.Tags = append(tags.Tags, matroskaTag{
		Targets: matroskaTargets{TypeValue: target, Type: typeName},
	})
	return len(tags.Tags) - 1
}

func matroskaTargetType(target int, episode bool) string {
	switch target {
	case 70:
		return "COLLECTION"
	case 60:
		return "SEASON"
	case 50:
		if episode {
			return "EPISODE"
		}
		return "MOVIE"
	default:
		return ""
	}
}

func targetValue(targets matroskaTargets) int {
	if targets.TypeValue != 0 {
		return targets.TypeValue
	}
	switch strings.ToUpper(targets.Type) {
	case "COLLECTION":
		return 70
	case "SEASON", "EDITION":
		return 60
	case "MOVIE", "EPISODE", "ALBUM", "CONCERT":
		return 50
	default:
		return 50
	}
}

func matroskaTagMap(tags matroskaTags) map[string]string {
	result := make(map[string]string)
	for _, tag := range tags.Tags {
		counts := map[string]int{}
		for _, simple := range tag.Simple {
			key := strconv.Itoa(targetValue(tag.Targets)) + ":" + strings.ToUpper(simple.Name)
			switch strings.ToUpper(simple.Name) {
			case "DIRECTOR", "WRITTEN_BY", "ACTOR":
				result[key+":"+simple.String] = simple.String
			default:
				if counts[key] == 0 {
					result[key] = simple.String
				}
				counts[key]++
			}
		}
	}
	return result
}
