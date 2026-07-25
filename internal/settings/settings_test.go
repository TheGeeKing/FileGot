package settings

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestStoreRoundTrip(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)

	store := NewStore(app.Preferences())
	value := Defaults()
	value.TMDBToken = "token"
	value.Language = "fr-FR"
	value.MoviePattern = "{title}.{year}"
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
