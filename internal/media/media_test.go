package media

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/TheGeeKing/FileGot/internal/mediainfo"
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
			name: "language tag stripped when uppercase",
			path: "La.Petite.Derniere.FRENCH.1080p.mkv",
			want: Parsed{Kind: Movie, Query: "La Petite Derniere"},
		},
		{
			name: "language word kept when title case",
			path: "The.French.Connection.1971.mkv",
			want: Parsed{Kind: Movie, Query: "The French Connection", Year: 1971},
		},
		{
			name: "empty parentheses after year",
			path: "Astérix et les Indiens (1994).mkv",
			want: Parsed{Kind: Movie, Query: "Astérix et les Indiens", Year: 1994},
		},
		{
			name: "nfd accents normalized for TMDB search",
			path: "Aste\u0301rix et les Indiens (1994).mkv",
			want: Parsed{Kind: Movie, Query: "Astérix et les Indiens", Year: 1994},
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

func TestIDHint(t *testing.T) {
	tests := []struct {
		path string
		want Identifier
		ok   bool
	}{
		{
			path: filepath.Join("Movies [tmdb-999]", "Film [tmdb-123].mkv"),
			want: Identifier{Source: TMDB, Value: "123"}, ok: true,
		},
		{
			path: filepath.Join("Library {tvdb-456}", "Season 1", "Show.S01E01.mkv"),
			want: Identifier{Source: TVDB, Value: "456"}, ok: true,
		},
		{
			path: filepath.Join("Movies (imdb-tt1234567)", "Film.mkv"),
			want: Identifier{Source: IMDB, Value: "tt1234567"}, ok: true,
		},
		{
			path: "Film (tt7654321).mkv",
			want: Identifier{Source: IMDB, Value: "tt7654321"}, ok: true,
		},
		{path: filepath.Join("Movies [tmdb-999]", "Film [tmdb-0].mkv"), want: Identifier{Source: TMDB, Value: "999"}, ok: true},
		{path: "Film [tmdb-nope].mkv"},
		{path: "Film [tvdb--1].mkv"},
		{path: "Film [imdb-123].mkv"},
		{path: "Film [tmdb-123).mkv"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, ok := IDHint(test.path)
			if ok != test.ok || got != test.want {
				t.Fatalf("IDHint(%q) = %#v, %t; want %#v, %t", test.path, got, ok, test.want, test.ok)
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

func TestFormatAdvancedOptionalMovieMetadata(t *testing.T) {
	pattern := `{n}{" ($y)"}{" [$primaryTitle]"}`
	movie := Candidate{
		ID: 438631, Kind: Movie, Title: `Dune: Part Two`,
		OriginalTitle: "Dune: Deuxième Partie", Year: 2024,
	}

	got, err := FormatAdvanced(pattern, "release.MKV", movie)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two (2024) [Dune Deuxième Partie].MKV"; got != want {
		t.Fatalf("FormatAdvanced() = %q, want %q", got, want)
	}

	movie.Year = 0
	movie.OriginalTitle = ""
	got, err = FormatAdvanced(pattern, "release.MKV", movie)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two.MKV"; got != want {
		t.Fatalf("FormatAdvanced() without optional metadata = %q, want %q", got, want)
	}
}

func TestFormatAdvancedEpisodeData(t *testing.T) {
	episode := Candidate{
		ID: 100088, Kind: Episode, Title: "The Last of Us", OriginalTitle: "The Last of Us",
		SeriesYear: 2023, Season: 1, Episode: 3, EpisodeTitle: "Long, Long Time",
	}
	pattern := `{n} ({primaryTitle}) {y} - {s00e00} - {t} [tmdb-{tmdbid}]`

	got, err := FormatAdvanced(pattern, "episode.mp4", episode)
	if err != nil {
		t.Fatal(err)
	}
	want := "The Last of Us (The Last of Us) 2023 - S01E03 - Long, Long Time [tmdb-100088].mp4"
	if got != want {
		t.Fatalf("FormatAdvanced() = %q, want %q", got, want)
	}
}

func TestFormatAdvancedRejectsMissingRequiredData(t *testing.T) {
	_, err := FormatAdvanced(`{n} ({y})`, "movie.mkv", Candidate{
		Kind: Movie, Year: 2024,
	})
	if err == nil {
		t.Fatal("missing movie title should be a row error")
	}
}

func TestFormatAdvancedRejectsMissingReferencedData(t *testing.T) {
	_, err := FormatAdvanced(`{n} ({y})`, "movie.mkv", Candidate{
		Kind: Movie, Title: "Dune",
	})
	if err == nil {
		t.Fatal("missing referenced year should be a row error")
	}
}

func TestFormatAdvancedAllowsPresentBindingToTransformEmpty(t *testing.T) {
	got, err := FormatAdvanced(`{n.removeAll(/.*/)}suffix`, "movie.mkv", Candidate{
		Kind: Movie, Title: "Dune",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "suffix.mkv"; got != want {
		t.Fatalf("FormatAdvanced() = %q, want %q", got, want)
	}
}

func TestFormatAdvancedConditionals(t *testing.T) {
	pattern := `{n}{(y == 2024 && n != "") || !primaryTitle ? " [$y]" : ""}`
	movie := Candidate{Kind: Movie, Title: "Dune", Year: 2024}

	got, err := FormatAdvanced(pattern, "movie.mkv", movie)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune [2024].mkv"; got != want {
		t.Fatalf("FormatAdvanced() = %q, want %q", got, want)
	}

	movie.Year = 0
	got, err = FormatAdvanced(pattern, "movie.mkv", movie)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune.mkv"; got != want {
		t.Fatalf("FormatAdvanced() without year = %q, want %q", got, want)
	}
}

func TestFormatAdvancedInterpolationMethods(t *testing.T) {
	got, err := FormatAdvanced(`{"Year $y.replace('2', '3')"}`, "movie.mkv", Candidate{
		Kind: Movie, Title: "Dune", Year: 2024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Year 3034.mkv"; got != want {
		t.Fatalf("FormatAdvanced() = %q, want %q", got, want)
	}
}

func TestAdvancedTemplateHelpers(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		pattern string
		want    string
	}{
		{name: "lower", title: "Dune Part Two", pattern: `{n.lower()}`, want: "dune part two.mkv"},
		{name: "upper", title: "Dune Part Two", pattern: `{n.upper()}`, want: "DUNE PART TWO.mkv"},
		{name: "trim", title: "  Dune Part Two  ", pattern: `{n.trim()}`, want: "Dune Part Two.mkv"},
		{name: "space", title: "Dune Part Two", pattern: `{n.space('.')}`, want: "Dune.Part.Two.mkv"},
		{name: "pad", title: "Dune", pattern: `{y.pad(4)}`, want: "0007.mkv"},
		{name: "replace", title: "Dune Part Two", pattern: `{n.replace('Two', '2')}`, want: "Dune Part 2.mkv"},
		{name: "default", title: "Dune", pattern: `{primaryTitle.default('Unknown')}`, want: "Unknown.mkv"},
		{name: "colon", title: "Dune: Part Two", pattern: `{n.colon(' - ')}`, want: "Dune - Part Two.mkv"},
		{name: "slash", title: `Dune/Part\Two`, pattern: `{n.slash('.')}`, want: "Dune.Part.Two.mkv"},
		{name: "before", title: "Dune - Part Two", pattern: `{n.before(' - ')}`, want: "Dune.mkv"},
		{name: "after", title: "Dune - Part Two", pattern: `{n.after(' - ')}`, want: "Part Two.mkv"},
		{name: "removeAll", title: "Dune!", pattern: `{n.removeAll(/[!?.]+$/)}`, want: "Dune.mkv"},
		{name: "replaceAll", title: "Dune   Part Two", pattern: `{n.replaceAll(/\s+/, '.')}`, want: "Dune.Part.Two.mkv"},
		{name: "regex quantifier", title: "Dune 2024", pattern: `{n.removeAll(/\s\d{4}$/)}`, want: "Dune.mkv"},
		{name: "upperInitial", title: "the day a demon was born", pattern: `{n.upperInitial()}`, want: "The Day A Demon Was Born.mkv"},
		{name: "lowerTrail", title: "Gundam SEED", pattern: `{n.lowerTrail()}`, want: "Gundam Seed.mkv"},
		{name: "sortName", title: "The Walking Dead", pattern: `{n.sortName()}`, want: "Walking Dead.mkv"},
		{name: "initialName", title: "James Cameron", pattern: `{n.initialName()}`, want: "J. Cameron.mkv"},
		{name: "acronym", title: "Deep Space 9", pattern: `{n.acronym()}`, want: "DS9.mkv"},
		{name: "roman", title: "Star Wars Episode 4", pattern: `{n.roman()}`, want: "Star Wars Episode IV.mkv"},
		{name: "clean", title: "Dune: Part Two?", pattern: `{n.clean()}`, want: "Dune Part Two.mkv"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FormatAdvanced(test.pattern, "movie.mkv", Candidate{
				Kind: Movie, Title: test.title, Year: 7,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("FormatAdvanced() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateAdvancedTemplate(t *testing.T) {
	valid := `{n.trim().space('.').lower()}{" ($y)"}`
	if err := ValidateAdvancedPattern(Movie, valid); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	if err := ValidateAdvancedPattern(Movie, `{y.replace('2', '3')}`); err != nil {
		t.Fatalf("valid integer replacement rejected: %v", err)
	}

	invalid := []string{
		`{{range .Title}}{{.}}{{end}}`,
		`{id}`,
		`{n.unknown()}`,
		`{n.lower('extra')}`,
		`{n.replace('one')}`,
		`{y.acronym()}`,
		`{y == 2024 ? n : n.removeAll(/[invalid/)}`,
		`{n; env('HOME')}`,
		`{n`,
	}
	for _, pattern := range invalid {
		t.Run(pattern, func(t *testing.T) {
			if err := ValidateAdvancedPattern(Movie, pattern); err == nil {
				t.Fatal("unsafe or invalid template was accepted")
			}
		})
	}
}

func TestAdvancedTemplateCatalog(t *testing.T) {
	movie := AdvancedTemplateCatalog(Movie)
	episode := AdvancedTemplateCatalog(Episode)

	if !hasAdvancedSyntax(movie, "n") || hasAdvancedSyntax(movie, "t") {
		t.Fatalf("movie bindings = %#v", movie)
	}
	if !hasAdvancedSyntax(episode, "n") || !hasAdvancedSyntax(episode, "t") {
		t.Fatalf("episode bindings = %#v", episode)
	}

	replace, ok := advancedSyntax(movie, "replace")
	if !ok || replace.Description == "" || replace.ReturnType != "String" ||
		replace.Syntax == "" || replace.Example == "" || len(replace.Parameters) != 2 {
		t.Fatalf("replace syntax = %#v", replace)
	}
	if replace.Parameters[0].Name != "old" || replace.Parameters[0].Type != AdvancedTemplateString ||
		!replace.Parameters[0].Required || replace.Parameters[1].Name != "new" ||
		replace.Parameters[1].Type != AdvancedTemplateString || !replace.Parameters[1].Required {
		t.Fatalf("replace parameters = %#v", replace.Parameters)
	}

	for _, method := range AdvancedTemplateMethods() {
		if !hasAdvancedSyntax(movie, method) || !hasAdvancedSyntax(episode, method) {
			t.Errorf("catalog is missing method %q", method)
		}
	}
}

func TestAdvancedTemplateCompletions(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		pattern     string
		cursor      int
		want        []string
		replacement [2]int
	}{
		{
			name: "movie bindings", kind: Movie, pattern: "{", cursor: 1,
			want: []string{
				"n", "ny", "primaryTitle", "tmdbid", "y",
				"vcf", "vc", "ac", "cf", "vf", "hpi", "vk", "aco", "acf", "af", "channels",
				"resolution", "width", "height", "bitdepth", "hdr", "dovi", "bitrate", "vbr",
				"abr", "fps", "khz", "ar", "ws", "hd", "s3d", "mediaTitle", "audioLanguages",
				"textLanguages", "duration", "seconds", "minutes", "hours",
				"media", "video", "audio", "text", "chapters", "image", "menu",
			},
			replacement: [2]int{1, 1},
		},
		{
			name: "episode-only binding", kind: Episode, pattern: "{t", cursor: 2,
			want: []string{"t", "tmdbid", "textLanguages", "text"}, replacement: [2]int{1, 2},
		},
		{
			name: "binding prefix", kind: Movie, pattern: "prefix {pr} suffix", cursor: 10,
			want: []string{"primaryTitle"}, replacement: [2]int{8, 10},
		},
		{
			name: "binding suffix", kind: Movie, pattern: "{primaryTitle}", cursor: 4,
			want: []string{"primaryTitle"}, replacement: [2]int{1, 13},
		},
		{
			name: "direct methods", kind: Movie, pattern: "{n.", cursor: 3,
			want: AdvancedTemplateMethods(), replacement: [2]int{3, 3},
		},
		{
			name: "integer methods", kind: Movie, pattern: "{y.", cursor: 3,
			want:        []string{"default", "pad", "removeAll", "replace", "replaceAll", "roman"},
			replacement: [2]int{3, 3},
		},
		{
			name: "interpolation methods", kind: Movie, pattern: `{" ($y.`, cursor: len([]rune(`{" ($y.`)),
			want:        []string{"default", "pad", "removeAll", "replace", "replaceAll", "roman"},
			replacement: [2]int{len([]rune(`{" ($y.`)), len([]rune(`{" ($y.`))},
		},
		{
			name: "chained method prefix", kind: Movie, pattern: `{n.space('.').re`, cursor: 16,
			want: []string{"removeAll", "replace", "replaceAll"}, replacement: [2]int{14, 16},
		},
		{
			name: "string literal", kind: Movie, pattern: `{n.replace('re`, cursor: 14,
		},
		{
			name: "regular expression literal", kind: Movie, pattern: `{n.removeAll(/re`, cursor: 16,
		},
		{
			name: "outside expression", kind: Movie, pattern: "movie", cursor: 5,
		},
		{
			name: "invalid chain", kind: Movie, pattern: "{n.unknown().", cursor: 13,
		},
		{
			name: "incompatible binding", kind: Movie, pattern: "{t.", cursor: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := AdvancedTemplateCompletions(test.kind, test.pattern, test.cursor)
			names := make([]string, len(got))
			for index, completion := range got {
				names[index] = completion.Name
				if completion.Description == "" || completion.ReturnType == "" || completion.Example == "" {
					t.Errorf("completion metadata = %#v", completion)
				}
				if completion.ReplaceStart != test.replacement[0] ||
					completion.ReplaceEnd != test.replacement[1] {
					t.Errorf("replacement = %d:%d, want %d:%d",
						completion.ReplaceStart, completion.ReplaceEnd,
						test.replacement[0], test.replacement[1])
				}
			}
			if !slices.Equal(names, test.want) {
				t.Fatalf("completions = %v, want %v", names, test.want)
			}
			if test.name == "integer methods" || test.name == "interpolation methods" {
				prefix := "{y."
				if test.name == "interpolation methods" {
					prefix = `{"$y.`
				}
				for _, completion := range got {
					if !strings.HasPrefix(completion.Syntax, prefix) {
						t.Fatalf("completion example = %q, want prefix %q", completion.Syntax, prefix)
					}
				}
			}
		})
	}
}

func TestAdvancedTemplateCompletesMediaInfoObjectFields(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{pattern: `{media.CompleteN`, want: "CompleteName"},
		{pattern: `{video[0].Wid`, want: "Width"},
		{pattern: `{audio[0].ChannelL`, want: "ChannelLayout"},
		{pattern: `{text[0].Lang`, want: "Language"},
		{pattern: `{image.For`, want: "Format"},
		{pattern: `{menu.Dur`, want: "Duration"},
	}
	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			completions := AdvancedTemplateCompletions(Movie, test.pattern, len([]rune(test.pattern)))
			for _, completion := range completions {
				if completion.Name == test.want {
					if completion.InsertText != test.want || completion.Description == "" ||
						completion.Example == "" || completion.ReturnType != "String" {
						t.Fatalf("completion = %#v", completion)
					}
					return
				}
			}
			t.Fatalf("completions = %#v, want %q", completions, test.want)
		})
	}
}

func TestAdvancedTemplateSignatureHelp(t *testing.T) {
	tests := []struct {
		name      string
		kind      Kind
		pattern   string
		cursor    int
		method    string
		parameter int
	}{
		{
			name: "first replace parameter", kind: Movie, pattern: `{n.replace(`, cursor: 11,
			method: "replace", parameter: 0,
		},
		{
			name: "second replace parameter", kind: Movie, pattern: `{n.replace('old', `, cursor: 18,
			method: "replace", parameter: 1,
		},
		{
			name: "optional pad parameter", kind: Movie, pattern: `{n.pad(20, `, cursor: 11,
			method: "pad", parameter: 1,
		},
		{
			name: "chained call", kind: Movie, pattern: `{n.trim().replace(`, cursor: 18,
			method: "replace", parameter: 0,
		},
		{
			name: "integer receiver", kind: Movie, pattern: `{y.replace(`, cursor: 11,
			method: "replace", parameter: 0,
		},
		{
			name: "interpolation receiver", kind: Movie,
			pattern: `{" ($y.replace(`, cursor: len([]rune(`{" ($y.replace(`)),
			method: "replace", parameter: 0,
		},
		{
			name: "cursor inside string", kind: Movie, pattern: `{n.replace('old', 'new')}`, cursor: 22,
			method: "replace", parameter: 1,
		},
		{name: "closed call", kind: Movie, pattern: `{n.replace('old', 'new')}`, cursor: 25},
		{name: "grouping parentheses", kind: Movie, pattern: `{(n != "")`, cursor: 10},
		{name: "unavailable receiver", kind: Movie, pattern: `{t.replace(`, cursor: 11},
		{name: "unknown receiver", kind: Movie, pattern: `{bogus.replace(`, cursor: 15},
		{name: "incompatible receiver", kind: Movie, pattern: `{y.acronym(`, cursor: 11},
		{name: "too many arguments", kind: Movie, pattern: `{n.replace('a', 'b', `, cursor: 21},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := AdvancedTemplateSignatureHelp(test.kind, test.pattern, test.cursor)
			if test.method == "" {
				if got != nil {
					t.Fatalf("signature = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Name != test.method || got.ActiveParameter != test.parameter {
				t.Fatalf("signature = %#v, want %s parameter %d", got, test.method, test.parameter)
			}
			if got.Parameters[got.ActiveParameter].Description == "" {
				t.Fatalf("active parameter has no description: %#v", got)
			}
		})
	}
}

func hasAdvancedSyntax(items []AdvancedTemplateSyntax, name string) bool {
	_, ok := advancedSyntax(items, name)
	return ok
}

func advancedSyntax(items []AdvancedTemplateSyntax, name string) (AdvancedTemplateSyntax, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return AdvancedTemplateSyntax{}, false
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

func TestAdvancedNamingUsesTechnicalBindingsAndRawObjects(t *testing.T) {
	input, err := os.Open(filepath.Join("..", "mediainfo", "testdata", "movie.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	technical, err := mediainfo.Decode(input)
	if err != nil {
		t.Fatal(err)
	}

	got, err := FormatAdvancedWithMetadata(
		`{n} [{vf} {vc} {aco}] [{video[0].Format_Profile}]`,
		"movie.mkv",
		Candidate{Kind: Movie, Title: "Dune Part Two"},
		technical,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two [2160p HEVC TrueHD+Atmos] [Main 10].mkv"; got != want {
		t.Fatalf("FormatAdvancedWithMetadata() = %q, want %q", got, want)
	}
}

func TestAdvancedTechnicalBindingRequiresMetadata(t *testing.T) {
	_, err := FormatAdvancedWithMetadata(
		`{n} [{hdr}]`,
		"movie.mkv",
		Candidate{Kind: Movie, Title: "Dune Part Two"},
		mediainfo.Metadata{},
	)
	if err == nil || !strings.Contains(err.Error(), "binding hdr is unavailable") {
		t.Fatalf("missing metadata error = %v", err)
	}
}

func TestExampleTechnicalMetadataProvidesDocumentedBindings(t *testing.T) {
	technical := ExampleTechnicalMetadata()
	for _, name := range technicalBindingNames {
		if technical.Bindings[name] == "" {
			t.Errorf("example metadata has no value for %q", name)
		}
		if technicalDescription(name) == "" {
			t.Errorf("technical binding %q has no description", name)
		}
		if technicalExample(name) == "" {
			t.Errorf("technical binding %q has no example", name)
		}
	}

	got, err := FormatAdvancedWithMetadata(
		`{n} [{width}x{height}]`,
		"movie.mkv",
		Candidate{Kind: Movie, Title: "Dune Part Two"},
		technical,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two [3840x2160].mkv"; got != want {
		t.Fatalf("FormatAdvancedWithMetadata() = %q, want %q", got, want)
	}

	for _, expression := range []string{
		`{media}`, `{video[0]}`, `{audio[0]}`, `{text[0]}`, `{chapters}`, `{image}`, `{menu}`,
	} {
		if _, err := FormatAdvancedWithMetadata(
			expression,
			"movie.mkv",
			Candidate{Kind: Movie, Title: "Dune Part Two"},
			technical,
		); err != nil {
			t.Errorf("%s: %v", expression, err)
		}
	}
	if technical.Media["CompleteName"] == "" || technical.Audio[0]["ChannelLayout"] == "" {
		t.Fatal("example raw MediaInfo objects are not populated")
	}
}

func TestTechnicalMetadataDetectionIgnoresLiterals(t *testing.T) {
	for _, pattern := range []string{
		`{n.replace('video', 'film')}`,
		`{n}{" HDR"}`,
		`Movie ar`,
	} {
		if AdvancedTemplateUsesTechnicalMetadata(pattern) {
			t.Errorf("ordinary pattern %q requires MediaInfo", pattern)
		}
	}
	for _, pattern := range []string{`{vf}`, `{" [$hdr]"}`, `{video[0]["Format Profile"]}`} {
		if !AdvancedTemplateUsesTechnicalMetadata(pattern) {
			t.Errorf("technical pattern %q does not require MediaInfo", pattern)
		}
	}
}
