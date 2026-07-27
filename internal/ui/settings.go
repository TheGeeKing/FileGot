package ui

import (
	"context"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/mediainfo"
	"github.com/TheGeeKing/FileGot/internal/settings"
	"github.com/TheGeeKing/FileGot/internal/tmdb"
)

var fallbackLanguages = []string{"de-DE", "en-GB", "en-US", "es-ES", "fr-FR", "it-IT", "ja-JP", "pt-BR"}

type namingSyntaxRow struct {
	Syntax      string
	Description string
	Example     string
	Type        string
}

func namingReferenceRows(mode string, kind media.Kind) []namingSyntaxRow {
	if mode == settings.NamingSimple {
		if kind == media.Movie {
			return []namingSyntaxRow{
				{"{title}", "Movie title", "Dune: Part Two", "String"},
				{"{year}", "Release year", "2024", "Integer"},
				{"{tmdb_id}", "TMDB movie ID", "438631", "Integer"},
			}
		}
		return []namingSyntaxRow{
			{"{series}", "Series title", "The Last of Us", "String"},
			{"{series_year}", "Series premiere year", "2023", "Integer"},
			{"{season}", "Season number with two digits", "01", "String"},
			{"{episode}", "Episode number with two digits", "03", "String"},
			{"{episode_title}", "Episode title", "Long, Long Time", "String"},
			{"{tmdb_id}", "TMDB series ID", "100088", "Integer"},
		}
	}

	catalog := media.AdvancedTemplateCatalog(kind)
	rows := make([]namingSyntaxRow, len(catalog))
	for index, item := range catalog {
		rows[index] = namingSyntaxRow{
			Syntax: item.Syntax, Description: item.Description,
			Example: item.Example, Type: item.ReturnType,
		}
	}
	return rows
}

func newNamingReferenceTable(rows func() []namingSyntaxRow) *widget.Table {
	headers := [...]string{"Syntax", "Description", "Example", "Type"}
	table := widget.NewTable(
		func() (int, int) { return len(rows()), len(headers) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			row := rows()[id.Row]
			label.SetText([]string{row.Syntax, row.Description, row.Example, row.Type}[id.Col])
		},
	)
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}
	table.UpdateHeader = func(id widget.TableCellID, object fyne.CanvasObject) {
		object.(*widget.Label).SetText(headers[id.Col])
	}
	table.SetColumnWidth(0, 220)
	table.SetColumnWidth(1, 360)
	table.SetColumnWidth(2, 260)
	table.SetColumnWidth(3, 120)
	return table
}

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
	mediaInfoExecutable := widget.NewEntry()
	mediaInfoExecutable.SetText(current.MediaInfoExecutable)
	mediaInfoStatus := widget.NewLabel("")
	mediaInfoStatus.Wrapping = fyne.TextWrapWord

	mode := current.NamingMode
	simpleMovie, simpleEpisode := current.MoviePattern, current.EpisodePattern
	advancedMovie, advancedEpisode := current.MovieTemplate, current.EpisodeTemplate
	namingMode := widget.NewRadioGroup([]string{settings.NamingSimple, settings.NamingAdvanced}, nil)
	namingMode.Horizontal = true
	namingMode.Required = true
	namingMode.SetSelected(mode)
	moviePattern := newAdvancedTemplateEntry(media.Movie, func() bool {
		return mode == settings.NamingAdvanced
	})
	episodePattern := newAdvancedTemplateEntry(media.Episode, func() bool {
		return mode == settings.NamingAdvanced
	})
	if mode == settings.NamingAdvanced {
		moviePattern.SetText(advancedMovie)
		episodePattern.SetText(advancedEpisode)
	} else {
		moviePattern.SetText(simpleMovie)
		episodePattern.SetText(simpleEpisode)
	}
	presetSelect := widget.NewSelect(
		[]string{settings.PresetClean, settings.PresetCompact, settings.PresetIDSafe, settings.PresetCustom},
		nil,
	)
	presetSelect.SetSelected(settings.PresetName(current))
	if mode == settings.NamingAdvanced {
		presetSelect.Disable()
	}
	examples := widget.NewLabel("")
	examples.Wrapping = fyne.TextWrapWord
	movieReference := newNamingReferenceTable(func() []namingSyntaxRow {
		return namingReferenceRows(mode, media.Movie)
	})
	episodeReference := newNamingReferenceTable(func() []namingSyntaxRow {
		return namingReferenceRows(mode, media.Episode)
	})
	referenceTabs := container.NewAppTabs(
		container.NewTabItem("Movie", movieReference),
		container.NewTabItem("Episode", episodeReference),
	)

	scanSubfolders := widget.NewCheck("Scan subfolders", nil)
	scanSubfolders.SetChecked(current.ScanSubfolders)
	autoMatch := widget.NewCheck("Match automatically after adding files", nil)
	autoMatch.SetChecked(current.AutoMatch)
	includeSpecials := widget.NewCheck("Include specials when importing all seasons", nil)
	includeSpecials.SetChecked(current.IncludeSpecials)
	confirmRename := widget.NewCheck("Confirm before renaming", nil)
	confirmRename.SetChecked(current.ConfirmRename)
	ignoreHidden := widget.NewCheck("Ignore hidden files and folders", nil)
	ignoreHidden.SetChecked(current.IgnoreHidden)
	sortMatchedByStatus := widget.NewCheck("Sort matched files by status", nil)
	sortMatchedByStatus.SetChecked(current.SortMatchedByStatus)

	validation := widget.NewLabel("")
	validation.Wrapping = fyne.TextWrapWord
	saveButton := widget.NewButton("Save", nil)
	var applyingPreset bool
	var applyingMode bool

	value := func() settings.Settings {
		return settings.Settings{
			TMDBToken: tokenEntry.Text, Language: languageSelect.Selected,
			PreferOriginalTitle: preferOriginal.Checked, IncludeAdult: includeAdult.Checked,
			MediaInfoExecutable: mediaInfoExecutable.Text,
			NamingMode:          mode, MoviePattern: simpleMovie, EpisodePattern: simpleEpisode,
			MovieTemplate: advancedMovie, EpisodeTemplate: advancedEpisode,
			ScanSubfolders: scanSubfolders.Checked, AutoMatch: autoMatch.Checked,
			IncludeSpecials: includeSpecials.Checked,
			ConfirmRename:   confirmRename.Checked, IgnoreHidden: ignoreHidden.Checked,
			SortMatchedByStatus: sortMatchedByStatus.Checked,
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

		movieExample, movieErr := candidate.FormatName("movie.mkv", media.Candidate{
			ID: 438631, Kind: media.Movie, Title: "Dune: Part Two",
			OriginalTitle: "Dune: Part Two", Year: 2024,
		})
		episodeExample, episodeErr := candidate.FormatName("episode.mkv", media.Candidate{
			ID: 100088, Kind: media.Episode, Title: "The Last of Us",
			OriginalTitle: "The Last of Us", SeriesYear: 2023,
			Season: 1, Episode: 3, EpisodeTitle: "Long, Long Time",
		})
		movieLine := "Movie: " + movieExample
		if movieErr != nil {
			movieLine = "Movie error: " + movieErr.Error()
		}
		episodeLine := "Episode: " + episodeExample
		if episodeErr != nil {
			episodeLine = "Episode error: " + episodeErr.Error()
		}
		examples.SetText(movieLine + "\n" + episodeLine)
	}

	patternChanged := func(_ string) {
		if applyingMode {
			return
		}
		if mode == settings.NamingAdvanced {
			advancedMovie, advancedEpisode = moviePattern.Text, episodePattern.Text
		} else {
			simpleMovie, simpleEpisode = moviePattern.Text, episodePattern.Text
		}
		if mode == settings.NamingSimple && !applyingPreset {
			presetSelect.SetSelected(settings.PresetCustom)
		}
		refreshValidation()
	}
	moviePattern.setOnChanged(patternChanged)
	episodePattern.setOnChanged(patternChanged)
	presetSelect.OnChanged = func(name string) {
		if mode != settings.NamingSimple || name == "" || name == settings.PresetCustom {
			return
		}
		applyingPreset = true
		movie, episode := settings.Preset(name)
		moviePattern.SetText(movie)
		episodePattern.SetText(episode)
		applyingPreset = false
		refreshValidation()
	}
	refreshNamingReferences := func() {
		movieReference.Refresh()
		episodeReference.Refresh()
	}
	namingMode.OnChanged = func(selected string) {
		if selected == "" || selected == mode {
			return
		}
		mode = selected
		applyingMode = true
		if mode == settings.NamingAdvanced {
			moviePattern.SetText(advancedMovie)
			episodePattern.SetText(advancedEpisode)
			presetSelect.Disable()
		} else {
			moviePattern.SetText(simpleMovie)
			episodePattern.SetText(simpleEpisode)
			presetSelect.Enable()
			presetSelect.SetSelected(settings.PresetName(settings.Settings{
				MoviePattern: simpleMovie, EpisodePattern: simpleEpisode,
			}))
		}
		applyingMode = false
		refreshNamingReferences()
		refreshValidation()
	}
	tokenEntry.OnChanged = func(string) { refreshValidation() }
	languageSelect.OnChanged = func(string) { refreshValidation() }
	preferOriginal.OnChanged = func(bool) { refreshValidation() }
	includeAdult.OnChanged = func(bool) { refreshValidation() }
	mediaInfoExecutable.OnChanged = func(string) { refreshValidation() }
	scanSubfolders.OnChanged = func(bool) { refreshValidation() }
	autoMatch.OnChanged = func(bool) { refreshValidation() }
	includeSpecials.OnChanged = func(bool) { refreshValidation() }
	confirmRename.OnChanged = func(bool) { refreshValidation() }
	ignoreHidden.OnChanged = func(bool) { refreshValidation() }
	sortMatchedByStatus.OnChanged = func(bool) { refreshValidation() }

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

	testMediaInfoButton := widget.NewButton("Test MediaInfo", nil)
	testMediaInfoButton.OnTapped = func() {
		testMediaInfoButton.Disable()
		mediaInfoStatus.SetText("Starting MediaInfo…")
		executable := mediaInfoExecutable.Text
		go func() {
			err := mediainfo.TestExecutable(context.Background(), executable)
			fyne.Do(func() {
				testMediaInfoButton.Enable()
				if err != nil {
					mediaInfoStatus.SetText(err.Error())
					return
				}
				mediaInfoStatus.SetText("MediaInfo is available.")
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
	mediaInfoTab := container.NewTabItem("MediaInfo", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Executable", mediaInfoExecutable)),
		container.NewHBox(testMediaInfoButton),
		mediaInfoStatus,
	))
	namingControls := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Mode", namingMode),
			widget.NewFormItem("Preset", presetSelect),
			widget.NewFormItem("Movie name", moviePattern),
			widget.NewFormItem("Episode name", episodePattern),
		),
		examples,
	)
	namingTab := container.NewTabItem(
		"Naming",
		container.NewBorder(namingControls, nil, nil, nil, referenceTabs),
	)
	behaviorTab := container.NewTabItem("Behavior", container.NewVBox(
		scanSubfolders,
		autoMatch,
		includeSpecials,
		confirmRename,
		ignoreHidden,
		sortMatchedByStatus,
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
		mode = defaults.NamingMode
		simpleMovie, simpleEpisode = defaults.MoviePattern, defaults.EpisodePattern
		advancedMovie, advancedEpisode = defaults.MovieTemplate, defaults.EpisodeTemplate
		tokenEntry.SetText(defaults.TMDBToken)
		languageSelect.SetSelected(defaults.Language)
		preferOriginal.SetChecked(defaults.PreferOriginalTitle)
		includeAdult.SetChecked(defaults.IncludeAdult)
		mediaInfoExecutable.SetText(defaults.MediaInfoExecutable)
		applyingMode = true
		moviePattern.SetText(simpleMovie)
		episodePattern.SetText(simpleEpisode)
		applyingMode = false
		namingMode.SetSelected(mode)
		presetSelect.Enable()
		scanSubfolders.SetChecked(defaults.ScanSubfolders)
		autoMatch.SetChecked(defaults.AutoMatch)
		includeSpecials.SetChecked(defaults.IncludeSpecials)
		confirmRename.SetChecked(defaults.ConfirmRename)
		ignoreHidden.SetChecked(defaults.IgnoreHidden)
		sortMatchedByStatus.SetChecked(defaults.SortMatchedByStatus)
		presetSelect.SetSelected(settings.PresetClean)
		refreshNamingReferences()
		refreshValidation()
	})

	tabs := container.NewAppTabs(providerTab, mediaInfoTab, namingTab, behaviorTab)
	footer := container.NewBorder(nil, nil, restoreButton, container.NewHBox(cancelButton, saveButton), validation)
	window.SetContent(container.NewBorder(nil, footer, nil, nil, tabs))
	window.Resize(fyne.NewSize(1040, 700))
	window.CenterOnScreen()
	refreshValidation()
	window.Show()
}
