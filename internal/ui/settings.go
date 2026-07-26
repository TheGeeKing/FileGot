package ui

import (
	"context"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/media"
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

var advancedExpressionRows = []namingSyntaxRow{
	{`{" ($y)"}`, "Optional fragment; omitted when a binding is unavailable", "(2024)", "Expression"},
	{`$y or ${y}`, "Insert a binding inside a quoted fragment", "2024", "Interpolation"},
	{`{n.space('.').lower()}`, "Apply methods from left to right", "dune:.part.two", "Method chain"},
	{`{y ? " ($y)" : ""}`, "Choose a value using a condition", "(2024)", "Conditional"},
	{`!  &&  ||  ==  !=`, "Supported conditional operators", `{y != "" && n != "" ? n : ""}`, "Operator"},
	{`'text' or "text"`, "String argument", "text", "String literal"},
	{`/pattern/`, "RE2 regular-expression argument", `[!?.]+$`, "Regex literal"},
	{`3`, "Positive integer argument", `{y.pad(6)}`, "Integer literal"},
}

var advancedMethodRows = []namingSyntaxRow{
	{`{n.acronym()}`, "Keep the first letter of each word", "DPT", "String method"},
	{`{n.after(': ')}`, "Keep text after a separator", "Part Two", "String method"},
	{`{n.before(': ')}`, "Keep text before a separator", "Dune", "String method"},
	{`{n.clean()}`, "Remove characters unsafe in filenames", "Dune Part Two", "String method"},
	{`{n.colon(' - ')}`, "Replace colons", "Dune - Part Two", "String method"},
	{`{primaryTitle.default('Unknown')}`, "Use a fallback when a binding is unavailable", "Unknown", "String method"},
	{`{n.initialName()}`, "Abbreviate all but the last word", "D. P. Two", "String method"},
	{`{n.lower()}`, "Convert text to lowercase", "dune: part two", "String method"},
	{`{n.lowerTrail()}`, "Lowercase every letter except each word's first", "Dune: Part Two", "String method"},
	{`{n.pad(20, '0')}`, "Pad text on the left to a length", "000000Dune: Part Two", "String method"},
	{`{n.removeAll(/[!?.]+$/)}`, "Remove every regular-expression match", "Dune: Part Two", "String method"},
	{`{n.replace('Two', '2')}`, "Replace literal text", "Dune: Part 2", "String method"},
	{`{n.replaceAll(/Part/, 'Chapter')}`, "Replace every regular-expression match", "Dune: Chapter Two", "String method"},
	{`{n.roman()}`, "Convert standalone numbers from 1 through 12", "Episode IV", "String method"},
	{`{n.slash('.')}`, "Replace forward and backward slashes", "Dune.Part Two", "String method"},
	{`{n.sortName()}`, "Remove a leading English article", "Walking Dead", "String method"},
	{`{n.space('.')}`, "Replace whitespace", "Dune:.Part.Two", "String method"},
	{`{n.trim()}`, "Remove surrounding whitespace", "Dune: Part Two", "String method"},
	{`{n.upper()}`, "Convert text to uppercase", "DUNE: PART TWO", "String method"},
	{`{n.upperInitial()}`, "Uppercase each word's first letter", "Dune: Part Two", "String method"},
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

	var bindings []namingSyntaxRow
	if kind == media.Movie {
		bindings = []namingSyntaxRow{
			{"{n}", "Movie title", "Dune: Part Two", "String"},
			{"{ny}", "Movie title and release year", "Dune: Part Two (2024)", "String"},
			{"{y}", "Release year", "2024", "Integer"},
			{"{primaryTitle}", "Original movie title", "Dune: Part Two", "String"},
			{"{tmdbid}", "TMDB movie ID", "438631", "Integer"},
		}
	} else {
		bindings = []namingSyntaxRow{
			{"{n}", "Series title", "The Last of Us", "String"},
			{"{ny}", "Series title and premiere year", "The Last of Us (2023)", "String"},
			{"{y}", "Series premiere year", "2023", "Integer"},
			{"{s}", "Season number", "1", "Integer"},
			{"{e}", "Episode number", "3", "Integer"},
			{"{sxe}", "Season and episode", "1x03", "String"},
			{"{s00e00}", "Padded season and episode", "S01E03", "String"},
			{"{t}", "Episode title", "Long, Long Time", "String"},
			{"{primaryTitle}", "Original series title", "The Last of Us", "String"},
			{"{tmdbid}", "TMDB series ID", "100088", "Integer"},
		}
	}
	return slices.Concat(bindings, advancedExpressionRows, advancedMethodRows)
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

	mode := current.NamingMode
	simpleMovie, simpleEpisode := current.MoviePattern, current.EpisodePattern
	advancedMovie, advancedEpisode := current.MovieTemplate, current.EpisodeTemplate
	namingMode := widget.NewRadioGroup([]string{settings.NamingSimple, settings.NamingAdvanced}, nil)
	namingMode.Horizontal = true
	namingMode.Required = true
	namingMode.SetSelected(mode)
	moviePattern := widget.NewEntry()
	episodePattern := widget.NewEntry()
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

	validation := widget.NewLabel("")
	validation.Wrapping = fyne.TextWrapWord
	saveButton := widget.NewButton("Save", nil)
	var applyingPreset bool
	var applyingMode bool

	value := func() settings.Settings {
		return settings.Settings{
			TMDBToken: tokenEntry.Text, Language: languageSelect.Selected,
			PreferOriginalTitle: preferOriginal.Checked, IncludeAdult: includeAdult.Checked,
			NamingMode: mode, MoviePattern: simpleMovie, EpisodePattern: simpleEpisode,
			MovieTemplate: advancedMovie, EpisodeTemplate: advancedEpisode,
			ScanSubfolders: scanSubfolders.Checked, AutoMatch: autoMatch.Checked,
			IncludeSpecials: includeSpecials.Checked,
			ConfirmRename:   confirmRename.Checked, IgnoreHidden: ignoreHidden.Checked,
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
	moviePattern.OnChanged = patternChanged
	episodePattern.OnChanged = patternChanged
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
	scanSubfolders.OnChanged = func(bool) { refreshValidation() }
	autoMatch.OnChanged = func(bool) { refreshValidation() }
	includeSpecials.OnChanged = func(bool) { refreshValidation() }
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
		presetSelect.SetSelected(settings.PresetClean)
		refreshNamingReferences()
		refreshValidation()
	})

	tabs := container.NewAppTabs(providerTab, namingTab, behaviorTab)
	footer := container.NewBorder(nil, nil, restoreButton, container.NewHBox(cancelButton, saveButton), validation)
	window.SetContent(container.NewBorder(nil, footer, nil, nil, tabs))
	window.Resize(fyne.NewSize(1040, 700))
	window.CenterOnScreen()
	refreshValidation()
	window.Show()
}
