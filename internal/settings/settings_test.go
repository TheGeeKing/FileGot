package settings

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/TheGeeKing/FileGot/internal/media"
)

func TestStoreRoundTrip(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)

	store := NewStore(app.Preferences())
	value := Defaults()
	value.TMDBToken = "token"
	value.Language = "fr-FR"
	value.MoviePattern = "{title}.{year}"
	value.MediaInfoExecutable = `C:\Tools\MediaInfo.exe`
	value.IncludeSpecials = false
	value.SortMatchedByStatus = true
	value.WriteEmbeddedMetadata = true
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}

	got := store.Load()
	if got != value {
		t.Fatalf("Load() = %#v, want %#v", got, value)
	}
	if name := PresetName(got); name != PresetCustom {
		t.Fatalf("PresetName() = %q, want Custom", name)
	}
}

func TestDefaultsIncludeSpecials(t *testing.T) {
	if !Defaults().IncludeSpecials {
		t.Fatal("specials should be included by default")
	}
}

func TestValidation(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := NewStore(app.Preferences())

	value := Defaults()
	if err := store.Validate(value); err == nil {
		t.Fatal("empty token should fail validation")
	}
	value.TMDBToken = "token"
	value.Language = "english"
	if err := store.Validate(value); err == nil {
		t.Fatal("invalid locale should fail validation")
	}
}

func TestAdvancedNamingValidationAndRendering(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := NewStore(app.Preferences())

	value := Defaults()
	value.TMDBToken = "token"
	value.NamingMode = NamingAdvanced
	value.MovieTemplate = `{n}{" ($y)"}`
	value.EpisodeTemplate = `{n} - {s00e00}{" - $t"}`
	if err := store.Validate(value); err != nil {
		t.Fatal(err)
	}

	got, err := value.FormatName("movie.mkv", media.Candidate{
		Kind: media.Movie, Title: "Dune: Part Two", Year: 2024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dune Part Two (2024).mkv"; got != want {
		t.Fatalf("FormatName() = %q, want %q", got, want)
	}

	value.MovieTemplate = `{n.unknown()}`
	if err := store.Validate(value); err == nil {
		t.Fatal("unsupported template action should fail validation")
	}
}
