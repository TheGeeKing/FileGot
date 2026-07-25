package ui

import (
	"context"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/thegeeking/FileGot/internal/media"
	"github.com/thegeeking/FileGot/internal/settings"
	"github.com/thegeeking/FileGot/internal/tmdb"
)

var fallbackLanguages = []string{"de-DE", "en-GB", "en-US", "es-ES", "fr-FR", "it-IT", "ja-JP", "pt-BR"}

func ShowSettings(app fyne.App, store *settings.Store, onSaved func()) {
	window := app.NewWindow("FileGot Settings")
	current := store.Load()

	tokenEntry := widget.NewEntry()
	tokenEntry.Password = true
	tokenEntry.SetText(current.TMDBToken)
	languages := append([]string(nil), fallbackLanguages...)
	if !slices.Contains(languages, current.Language) {
		languages = append(languages, current.Language)
		slices.Sort(languages)
	}
	languageSelect := widget.NewSelect(languages, nil)
	languageSelect.SetSelected(current.Language)
	preferOriginal := widget.NewCheck("Prefer original titles", nil)
	preferOriginal.SetChecked(current.PreferOriginalTitle)
	includeAdult := widget.NewCheck("Include adult search results", nil)
	includeAdult.SetChecked(current.IncludeAdult)
	connectionStatus := widget.NewLabel("")
	connectionStatus.Wrapping = fyne.TextWrapWord

	moviePattern := widget.NewEntry()
	moviePattern.SetText(current.MoviePattern)
	episodePattern := widget.NewEntry()
	episodePattern.SetText(current.EpisodePattern)
	presetSelect := widget.NewSelect(
		[]string{settings.PresetClean, settings.PresetCompact, settings.PresetIDSafe, settings.PresetCustom},
		nil,
	)
	presetSelect.SetSelected(settings.PresetName(current))
	examples := widget.NewLabel("")
	examples.Wrapping = fyne.TextWrapWord

	scanSubfolders := widget.NewCheck("Scan subfolders", nil)
	scanSubfolders.SetChecked(current.ScanSubfolders)
	autoMatch := widget.NewCheck("Match automatically after adding files", nil)
	autoMatch.SetChecked(current.AutoMatch)
	confirmRename := widget.NewCheck("Confirm before renaming", nil)
	confirmRename.SetChecked(current.ConfirmRename)
	ignoreHidden := widget.NewCheck("Ignore hidden files and folders", nil)
	ignoreHidden.SetChecked(current.IgnoreHidden)

	validation := widget.NewLabel("")
	validation.Wrapping = fyne.TextWrapWord
	saveButton := widget.NewButton("Save", nil)
	var applyingPreset bool

	value := func() settings.Settings {
		return settings.Settings{
			TMDBToken: tokenEntry.Text, Language: languageSelect.Selected,
			PreferOriginalTitle: preferOriginal.Checked, IncludeAdult: includeAdult.Checked,
			MoviePattern: moviePattern.Text, EpisodePattern: episodePattern.Text,
			ScanSubfolders: scanSubfolders.Checked, AutoMatch: autoMatch.Checked,
			ConfirmRename: confirmRename.Checked, IgnoreHidden: ignoreHidden.Checked,
		}
	}

	refreshValidation := func() {
		candidate := value()
		if err := store.Validate(candidate); err != nil {
			validation.SetText(err.Error())
			saveButton.Disable()
		} else {
			validation.SetText("Settings are valid.")
			saveButton.Enable()
		}

		movieExample, movieErr := media.Format(candidate.MoviePattern, "movie.mkv", media.Candidate{
			ID: 438631, Kind: media.Movie, Title: "Dune: Part Two", Year: 2024,
		})
		episodeExample, episodeErr := media.Format(candidate.EpisodePattern, "episode.mkv", media.Candidate{
			ID: 100088, Kind: media.Episode, Title: "The Last of Us", SeriesYear: 2023,
			Season: 1, Episode: 3, EpisodeTitle: "Long, Long Time",
		})
		if movieErr == nil && episodeErr == nil {
			examples.SetText("Movie: " + movieExample + "\nEpisode: " + episodeExample)
		} else {
			examples.SetText("Fix the pattern errors to see examples.")
		}
	}

	patternChanged := func(string) {
		if !applyingPreset {
			presetSelect.SetSelected(settings.PresetCustom)
		}
		refreshValidation()
	}
	moviePattern.OnChanged = patternChanged
	episodePattern.OnChanged = patternChanged
	presetSelect.OnChanged = func(name string) {
		if name == "" || name == settings.PresetCustom {
			return
		}
		applyingPreset = true
		movie, episode := settings.Preset(name)
		moviePattern.SetText(movie)
		episodePattern.SetText(episode)
		applyingPreset = false
		refreshValidation()
	}
	tokenEntry.OnChanged = func(string) { refreshValidation() }
	languageSelect.OnChanged = func(string) { refreshValidation() }
	preferOriginal.OnChanged = func(bool) { refreshValidation() }
	includeAdult.OnChanged = func(bool) { refreshValidation() }
	scanSubfolders.OnChanged = func(bool) { refreshValidation() }
	autoMatch.OnChanged = func(bool) { refreshValidation() }
	confirmRename.OnChanged = func(bool) { refreshValidation() }
	ignoreHidden.OnChanged = func(bool) { refreshValidation() }

	testButton := widget.NewButton("Test Connection", nil)
	testButton.OnTapped = func() {
		testButton.Disable()
		connectionStatus.SetText("Connecting to TMDB…")
		token := tokenEntry.Text
		go func() {
			translations, err := tmdb.New(token).Translations(context.Background())
			fyne.Do(func() {
				testButton.Enable()
				if err != nil {
					connectionStatus.SetText(err.Error())
					return
				}
				selectedLanguage := languageSelect.Selected
				slices.Sort(translations)
				languageSelect.SetOptions(translations)
				if slices.Contains(translations, selectedLanguage) {
					languageSelect.SetSelected(selectedLanguage)
				} else {
					languageSelect.SetSelected("en-US")
				}
				connectionStatus.SetText(fmt.Sprintf("Connected. TMDB returned %d supported locales.", len(translations)))
			})
		}()
	}

	providerTab := container.NewTabItem("TMDB", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Read Access Token", tokenEntry),
			widget.NewFormItem("Language", languageSelect),
		),
		preferOriginal,
		includeAdult,
		container.NewHBox(testButton),
		connectionStatus,
	))
	namingTab := container.NewTabItem("Naming", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Preset", presetSelect),
			widget.NewFormItem("Movie pattern", moviePattern),
			widget.NewFormItem("Episode pattern", episodePattern),
		),
		widget.NewLabel("Movie tokens: {title}, {year}, {tmdb_id}"),
		widget.NewLabel("Episode tokens: {series}, {series_year}, {season}, {episode}, {episode_title}, {tmdb_id}"),
		examples,
	))
	behaviorTab := container.NewTabItem("Behavior", container.NewVBox(
		scanSubfolders,
		autoMatch,
		confirmRename,
		ignoreHidden,
	))

	saveButton.OnTapped = func() {
		if err := store.Save(value()); err != nil {
			validation.SetText(err.Error())
			return
		}
		window.Close()
		if onSaved != nil {
			onSaved()
		}
	}
	cancelButton := widget.NewButton("Cancel", window.Close)
	restoreButton := widget.NewButton("Restore Defaults", func() {
		defaults := settings.Defaults()
		tokenEntry.SetText(defaults.TMDBToken)
		languageSelect.SetSelected(defaults.Language)
		preferOriginal.SetChecked(defaults.PreferOriginalTitle)
		includeAdult.SetChecked(defaults.IncludeAdult)
		moviePattern.SetText(defaults.MoviePattern)
		episodePattern.SetText(defaults.EpisodePattern)
		scanSubfolders.SetChecked(defaults.ScanSubfolders)
		autoMatch.SetChecked(defaults.AutoMatch)
		confirmRename.SetChecked(defaults.ConfirmRename)
		ignoreHidden.SetChecked(defaults.IgnoreHidden)
		presetSelect.SetSelected(settings.PresetClean)
		refreshValidation()
	})

	tabs := container.NewAppTabs(providerTab, namingTab, behaviorTab)
	footer := container.NewBorder(nil, nil, restoreButton, container.NewHBox(cancelButton, saveButton), validation)
	window.SetContent(container.NewBorder(nil, footer, nil, nil, tabs))
	window.Resize(fyne.NewSize(760, 560))
	window.CenterOnScreen()
	refreshValidation()
	window.Show()
}
