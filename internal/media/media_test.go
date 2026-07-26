package media

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

	invalid := []string{
		`{{range .Title}}{{.}}{{end}}`,
		`{resolution}`,
		`{id}`,
		`{n.unknown()}`,
		`{n.lower('extra')}`,
		`{n.replace('one')}`,
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
	if replace.Parameters[0].Name != "old" || replace.Parameters[0].Type != "String" ||
		!replace.Parameters[0].Required || replace.Parameters[1].Name != "new" ||
		replace.Parameters[1].Type != "String" || !replace.Parameters[1].Required {
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
			want: []string{"n", "ny", "primaryTitle", "tmdbid", "y"}, replacement: [2]int{1, 1},
		},
		{
			name: "episode-only binding", kind: Episode, pattern: "{t", cursor: 2,
			want: []string{"t", "tmdbid"}, replacement: [2]int{1, 2},
		},
		{
			name: "binding prefix", kind: Movie, pattern: "prefix {pr} suffix", cursor: 10,
			want: []string{"primaryTitle"}, replacement: [2]int{8, 10},
		},
		{
			name: "direct methods", kind: Movie, pattern: "{n.", cursor: 3,
			want: AdvancedTemplateMethods(), replacement: [2]int{3, 3},
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
		})
	}
}

func TestAdvancedTemplateSignatureHelp(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		cursor    int
		method    string
		parameter int
	}{
		{
			name: "first replace parameter", pattern: `{n.replace(`, cursor: 11,
			method: "replace", parameter: 0,
		},
		{
			name: "second replace parameter", pattern: `{n.replace('old', `, cursor: 18,
			method: "replace", parameter: 1,
		},
		{
			name: "optional pad parameter", pattern: `{n.pad(20, `, cursor: 11,
			method: "pad", parameter: 1,
		},
		{
			name: "chained call", pattern: `{n.trim().replace(`, cursor: 18,
			method: "replace", parameter: 0,
		},
		{
			name: "cursor inside string", pattern: `{n.replace('old', 'new')}`, cursor: 22,
			method: "replace", parameter: 1,
		},
		{name: "closed call", pattern: `{n.replace('old', 'new')}`, cursor: 25},
		{name: "grouping parentheses", pattern: `{(n != "")`, cursor: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := AdvancedTemplateSignatureHelp(test.pattern, test.cursor)
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
