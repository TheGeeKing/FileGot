package settings

import (
	"fmt"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/mediainfo"
	"github.com/TheGeeKing/FileGot/internal/metadata"
)

const (
	PresetClean   = "Clean"
	PresetCompact = "Compact"
	PresetIDSafe  = "ID-safe"
	PresetCustom  = "Custom"

	NamingSimple   = "Simple"
	NamingAdvanced = "Advanced"

	defaultMovieTemplate   = `{n}{" ($y)"}`
	defaultEpisodeTemplate = `{n} - {s00e00}{" - $t"}`
)

var languagePattern = regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2}$`)

type Settings struct {
	TMDBToken              string
	Language               string
	PreferOriginalTitle    bool
	IncludeAdult           bool
	MediaInfoExecutable    string
	NamingMode             string
	MoviePattern           string
	EpisodePattern         string
	MovieTemplate          string
	EpisodeTemplate        string
	ScanSubfolders         bool
	AutoMatch              bool
	IncludeSpecials        bool
	ConfirmRename          bool
	IgnoreHidden           bool
	SortMatchedByStatus    bool
	WriteEmbeddedMetadata  bool
	EmbeddedMetadataFields metadata.WriteFields
}

type Store struct {
	preferences fyne.Preferences
}

func NewStore(preferences fyne.Preferences) *Store {
	return &Store{preferences: preferences}
}

func Defaults() Settings {
	movie, episode := Preset(PresetClean)
	return Settings{
		Language:               "en-US",
		MediaInfoExecutable:    "",
		NamingMode:             NamingSimple,
		MoviePattern:           movie,
		EpisodePattern:         episode,
		MovieTemplate:          defaultMovieTemplate,
		EpisodeTemplate:        defaultEpisodeTemplate,
		ScanSubfolders:         true,
		AutoMatch:              true,
		IncludeSpecials:        true,
		ConfirmRename:          true,
		IgnoreHidden:           true,
		EmbeddedMetadataFields: metadata.AllWriteFields(),
	}
}

func Preset(name string) (movie, episode string) {
	switch name {
	case PresetCompact:
		return "{title}.{year}", "{series}.S{season}E{episode}.{episode_title}"
	case PresetIDSafe:
		return "{title} ({year}) [tmdb-{tmdb_id}]", "{series} - S{season}E{episode} - {episode_title} [tmdb-{tmdb_id}]"
	default:
		return "{title} ({year})", "{series} - S{season}E{episode} - {episode_title}"
	}
}

func PresetName(settings Settings) string {
	for _, name := range []string{PresetClean, PresetCompact, PresetIDSafe} {
		movie, episode := Preset(name)
		if settings.MoviePattern == movie && settings.EpisodePattern == episode {
			return name
		}
	}
	return PresetCustom
}

func (store *Store) Load() Settings {
	defaults := Defaults()
	prefs := store.preferences
	return Settings{
		TMDBToken:              prefs.StringWithFallback("tmdb.token", defaults.TMDBToken),
		Language:               prefs.StringWithFallback("tmdb.language", defaults.Language),
		PreferOriginalTitle:    prefs.BoolWithFallback("tmdb.prefer_original_title", defaults.PreferOriginalTitle),
		IncludeAdult:           prefs.BoolWithFallback("tmdb.include_adult", defaults.IncludeAdult),
		MediaInfoExecutable:    configuredMediaInfo(prefs.StringWithFallback("mediainfo.executable", defaults.MediaInfoExecutable)),
		NamingMode:             prefs.StringWithFallback("naming.mode", defaults.NamingMode),
		MoviePattern:           prefs.StringWithFallback("naming.movie", defaults.MoviePattern),
		EpisodePattern:         prefs.StringWithFallback("naming.episode", defaults.EpisodePattern),
		MovieTemplate:          prefs.StringWithFallback("naming.advanced.movie", defaults.MovieTemplate),
		EpisodeTemplate:        prefs.StringWithFallback("naming.advanced.episode", defaults.EpisodeTemplate),
		ScanSubfolders:         prefs.BoolWithFallback("behavior.scan_subfolders", defaults.ScanSubfolders),
		AutoMatch:              prefs.BoolWithFallback("behavior.auto_match", defaults.AutoMatch),
		IncludeSpecials:        prefs.BoolWithFallback("behavior.include_specials", defaults.IncludeSpecials),
		ConfirmRename:          prefs.BoolWithFallback("behavior.confirm_rename", defaults.ConfirmRename),
		IgnoreHidden:           prefs.BoolWithFallback("behavior.ignore_hidden", defaults.IgnoreHidden),
		SortMatchedByStatus:    prefs.BoolWithFallback("behavior.sort_matched_by_status", defaults.SortMatchedByStatus),
		WriteEmbeddedMetadata:  prefs.BoolWithFallback("behavior.write_embedded_metadata", defaults.WriteEmbeddedMetadata),
		EmbeddedMetadataFields: loadEmbeddedMetadataFields(prefs, defaults.EmbeddedMetadataFields),
	}
}

func loadEmbeddedMetadataFields(prefs fyne.Preferences, defaults metadata.WriteFields) metadata.WriteFields {
	return metadata.WriteFields{
		Title:         prefs.BoolWithFallback("behavior.embedded_metadata.title", defaults.Title),
		OriginalTitle: prefs.BoolWithFallback("behavior.embedded_metadata.original_title", defaults.OriginalTitle),
		Comment:       prefs.BoolWithFallback("behavior.embedded_metadata.comment", defaults.Comment),
		DateReleased:  prefs.BoolWithFallback("behavior.embedded_metadata.date_released", defaults.DateReleased),
		Genre:         prefs.BoolWithFallback("behavior.embedded_metadata.genre", defaults.Genre),
		LawRating:     prefs.BoolWithFallback("behavior.embedded_metadata.law_rating", defaults.LawRating),
		Directors:     prefs.BoolWithFallback("behavior.embedded_metadata.directors", defaults.Directors),
		Writers:       prefs.BoolWithFallback("behavior.embedded_metadata.writers", defaults.Writers),
		Actors:        prefs.BoolWithFallback("behavior.embedded_metadata.actors", defaults.Actors),
		TMDBID:        prefs.BoolWithFallback("behavior.embedded_metadata.tmdb_id", defaults.TMDBID),
		SeriesInfo:    prefs.BoolWithFallback("behavior.embedded_metadata.series_info", defaults.SeriesInfo),
	}
}

func configuredMediaInfo(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "mediainfo") {
		return ""
	}
	return value
}

func (store *Store) Validate(value Settings) error {
	if strings.TrimSpace(value.TMDBToken) == "" {
		return fmt.Errorf("TMDB Read Access Token is required")
	}
	if !languagePattern.MatchString(value.Language) {
		return fmt.Errorf("language must be an IETF locale such as en-US")
	}
	switch value.NamingMode {
	case NamingSimple:
		if err := media.ValidatePattern(media.Movie, value.MoviePattern); err != nil {
			return err
		}
		return media.ValidatePattern(media.Episode, value.EpisodePattern)
	case NamingAdvanced:
		if err := media.ValidateAdvancedPattern(media.Movie, value.MovieTemplate); err != nil {
			return err
		}
		return media.ValidateAdvancedPattern(media.Episode, value.EpisodeTemplate)
	default:
		return fmt.Errorf("unsupported naming mode %q", value.NamingMode)
	}
}

func (store *Store) Save(value Settings) error {
	if err := store.Validate(value); err != nil {
		return err
	}
	prefs := store.preferences
	prefs.SetString("tmdb.token", strings.TrimSpace(value.TMDBToken))
	prefs.SetString("tmdb.language", value.Language)
	prefs.SetBool("tmdb.prefer_original_title", value.PreferOriginalTitle)
	prefs.SetBool("tmdb.include_adult", value.IncludeAdult)
	prefs.SetString("mediainfo.executable", strings.TrimSpace(value.MediaInfoExecutable))
	prefs.SetString("naming.mode", value.NamingMode)
	prefs.SetString("naming.movie", value.MoviePattern)
	prefs.SetString("naming.episode", value.EpisodePattern)
	prefs.SetString("naming.advanced.movie", value.MovieTemplate)
	prefs.SetString("naming.advanced.episode", value.EpisodeTemplate)
	prefs.SetBool("behavior.scan_subfolders", value.ScanSubfolders)
	prefs.SetBool("behavior.auto_match", value.AutoMatch)
	prefs.SetBool("behavior.include_specials", value.IncludeSpecials)
	prefs.SetBool("behavior.confirm_rename", value.ConfirmRename)
	prefs.SetBool("behavior.ignore_hidden", value.IgnoreHidden)
	prefs.SetBool("behavior.sort_matched_by_status", value.SortMatchedByStatus)
	prefs.SetBool("behavior.write_embedded_metadata", value.WriteEmbeddedMetadata)
	fields := value.EmbeddedMetadataFields
	prefs.SetBool("behavior.embedded_metadata.title", fields.Title)
	prefs.SetBool("behavior.embedded_metadata.original_title", fields.OriginalTitle)
	prefs.SetBool("behavior.embedded_metadata.comment", fields.Comment)
	prefs.SetBool("behavior.embedded_metadata.date_released", fields.DateReleased)
	prefs.SetBool("behavior.embedded_metadata.genre", fields.Genre)
	prefs.SetBool("behavior.embedded_metadata.law_rating", fields.LawRating)
	prefs.SetBool("behavior.embedded_metadata.directors", fields.Directors)
	prefs.SetBool("behavior.embedded_metadata.writers", fields.Writers)
	prefs.SetBool("behavior.embedded_metadata.actors", fields.Actors)
	prefs.SetBool("behavior.embedded_metadata.tmdb_id", fields.TMDBID)
	prefs.SetBool("behavior.embedded_metadata.series_info", fields.SeriesInfo)
	return nil
}

func (value Settings) FormatName(originalPath string, candidate media.Candidate) (string, error) {
	return value.FormatNameWithMetadata(originalPath, candidate, mediainfo.Metadata{})
}

func (value Settings) FormatNameWithMetadata(
	originalPath string,
	candidate media.Candidate,
	technical mediainfo.Metadata,
) (string, error) {
	pattern := value.MoviePattern
	template := value.MovieTemplate
	if candidate.Kind == media.Episode {
		pattern = value.EpisodePattern
		template = value.EpisodeTemplate
	}
	switch value.NamingMode {
	case NamingSimple:
		return media.Format(pattern, originalPath, candidate)
	case NamingAdvanced:
		return media.FormatAdvancedWithMetadata(template, originalPath, candidate, technical)
	default:
		return "", fmt.Errorf("unsupported naming mode %q", value.NamingMode)
	}
}
