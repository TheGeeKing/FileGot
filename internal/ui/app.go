package ui

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/matcher"
	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/rename"
	"github.com/TheGeeKing/FileGot/internal/settings"
	"github.com/TheGeeKing/FileGot/internal/tmdb"
)

type Application struct {
	app      fyne.App
	window   fyne.Window
	settings *settings.Store
	renamer  *rename.Manager

	clientMu    sync.Mutex
	clientToken string
	client      *tmdb.Client

	files           []media.File
	table           *fileTable
	empty           fyne.CanvasObject
	status          *widget.Label
	details         *widget.Label
	selected        int
	selectedRows    map[int]bool
	selectionAnchor int
	sortColumn      int
	sortAscending   bool
	cancel          context.CancelFunc
	busy            bool

	importShowButton *widget.Button
	removeButton     *widget.Button
	clearButton      *widget.Button
	matchButton      *widget.Button
	reviewButton     *widget.Button
	renameButton     *widget.Button
	undoButton       *widget.Button
}

func New(app fyne.App, store *settings.Store, renamer *rename.Manager) *Application {
	application := &Application{
		app: app, window: app.NewWindow("FileGot"), settings: store, renamer: renamer,
		selected: -1, selectedRows: make(map[int]bool), selectionAnchor: -1, sortColumn: -1,
	}
	application.build()
	return application
}

func (application *Application) Run() {
	application.window.Resize(fyne.NewSize(1100, 650))
	application.window.CenterOnScreen()
	application.window.Show()

	if application.renamer.HasPendingRecovery() {
		dialog.ShowConfirm(
			"Interrupted rename",
			"FileGot found an interrupted rename operation. Restore the original filenames now?",
			func(recover bool) {
				if !recover {
					return
				}
				if err := application.renamer.Recover(); err != nil {
					dialog.ShowError(err, application.window)
					return
				}
				application.setStatus("Interrupted rename recovered.")
				application.refresh()
			},
			application.window,
		)
	}
	application.app.Run()
}

func (application *Application) tmdbClient(token string) *tmdb.Client {
	application.clientMu.Lock()
	defer application.clientMu.Unlock()
	if application.client == nil || application.clientToken != token {
		application.clientToken = token
		application.client = tmdb.New(token)
	}
	return application.client
}

func (application *Application) build() {
	application.table = newFileTable(
		func() (int, int) { return len(application.files) + 1, 3 },
		func() fyne.CanvasObject {
			background := canvas.NewRectangle(color.Transparent)
			label := widget.NewLabel("")
			label.SizeName = theme.SizeNameCaptionText
			label.Truncation = fyne.TextTruncateEllipsis
			return container.NewStack(background, label)
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			cell := object.(*fyne.Container)
			background := cell.Objects[0].(*canvas.Rectangle)
			label := cell.Objects[1].(*widget.Label)
			label.TextStyle = fyne.TextStyle{Bold: id.Row == 0}
			if id.Row == 0 {
				background.FillColor = theme.ColorForWidget(theme.ColorNameHeaderBackground, application.table)
				background.Refresh()
				header := []string{"Original File", "Status", "Proposed Name"}[id.Col]
				if id.Col == application.sortColumn {
					if application.sortAscending {
						header += " ▲"
					} else {
						header += " ▼"
					}
				}
				label.SetText(header)
				return
			}
			file := application.files[id.Row-1]
			if application.selectedRows[id.Row-1] {
				background.FillColor = theme.ColorForWidget(theme.ColorNameSelection, application.table)
			} else {
				background.FillColor = statusRowColor(
					rowStatus(file),
					theme.ColorForWidget(theme.ColorNameBackground, application.table),
				)
			}
			background.Refresh()
			label.SetText(fileColumnText(file, id.Col))
		},
	)
	application.table.StickyRowCount = 1
	application.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			if !application.busy {
				application.sortFiles(id.Col)
			}
			application.table.Unselect(id)
			return
		}
		if application.table.contextClick && application.selectedRows[id.Row-1] {
			application.selected = id.Row - 1
		} else {
			application.selectRow(id.Row-1, application.table.modifiers)
		}
		application.table.Refresh()
		application.updateDetails()
		application.updateButtons()
	}
	application.table.menu = application.contextActions

	application.importShowButton = widget.NewButtonWithIcon("Import Show", theme.ContentAddIcon(), application.importShow)
	application.removeButton = widget.NewButtonWithIcon("Remove", theme.ContentRemoveIcon(), application.removeSelected)
	application.clearButton = widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), application.clear)
	application.matchButton = widget.NewButtonWithIcon("Match", theme.SearchIcon(), application.matchOrCancel)
	application.reviewButton = widget.NewButtonWithIcon("Review", theme.VisibilityIcon(), application.reviewSelected)
	application.renameButton = widget.NewButtonWithIcon("Rename", theme.ConfirmIcon(), application.confirmRename)
	application.renameButton.Importance = widget.HighImportance
	application.undoButton = widget.NewButtonWithIcon("Undo Last", theme.ContentUndoIcon(), application.undo)
	showSettings := func() {
		ShowSettings(application.app, application.settings, func() {
			if err := application.refreshProposedNames(); err != nil {
				dialog.ShowError(err, application.window)
				return
			}
			application.setStatus("Settings saved.")
			application.refresh()
		})
	}
	application.window.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu(
			"File",
			fyne.NewMenuItemWithIcon("Add File", theme.FileIcon(), application.addFile),
			fyne.NewMenuItemWithIcon("Add Folder", theme.FolderOpenIcon(), application.addFolder),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon("Settings", theme.SettingsIcon(), showSettings),
		),
		fyne.NewMenu("Help", fyne.NewMenuItemWithIcon("About", theme.InfoIcon(), func() {
			ShowAbout(application.window)
		})),
	))

	toolbar := container.NewHScroll(container.NewHBox(
		application.importShowButton,
		widget.NewSeparator(),
		application.removeButton,
		application.clearButton,
		widget.NewSeparator(),
		application.matchButton,
		application.reviewButton,
		application.renameButton,
		widget.NewSeparator(),
		application.undoButton,
	))
	application.status = widget.NewLabel("Drop video files or a folder to begin.")
	application.status.Wrapping = fyne.TextWrapWord
	application.details = widget.NewLabel("")
	application.details.Selectable = true
	application.details.Wrapping = fyne.TextWrapBreak
	application.details.Hide()
	footer := container.NewVBox(application.details, application.status)
	tableArea := container.New(&fileTableLayout{table: application.table}, application.table)
	emptyTitle := widget.NewLabelWithStyle(
		"Drop video files or a folder here",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)
	emptyHint := widget.NewLabel("or use the File menu to choose a source")
	emptyHint.Alignment = fyne.TextAlignCenter
	application.empty = container.NewCenter(container.NewVBox(emptyTitle, emptyHint))
	fileArea := container.NewStack(tableArea, application.empty)
	application.window.SetContent(container.NewBorder(toolbar, footer, nil, nil, fileArea))
	application.window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		paths := make([]string, 0, len(uris))
		for _, uri := range uris {
			if uri.Scheme() == "file" {
				paths = append(paths, uri.Path())
			}
		}
		application.addPaths(paths)
	})
	application.window.SetOnClosed(func() {
		if application.cancel != nil {
			application.cancel()
		}
	})
	application.updateFileArea()
	application.updateButtons()
}

func (application *Application) sortFiles(column int) {
	application.clearSelection()
	application.sortAscending = column != application.sortColumn || !application.sortAscending
	application.sortColumn = column
	sort.SliceStable(application.files, func(left, right int) bool {
		a := fileColumnText(application.files[left], column)
		b := fileColumnText(application.files[right], column)
		if column == 1 {
			a = string(rowStatus(application.files[left]))
			b = string(rowStatus(application.files[right]))
		}
		a = strings.ToLower(a)
		b = strings.ToLower(b)
		if application.sortAscending {
			return a < b
		}
		return a > b
	})
	application.table.Refresh()
}

func (application *Application) selectRow(row int, modifiers fyne.KeyModifier) {
	if modifiers&fyne.KeyModifierShift != 0 && application.selectionAnchor >= 0 {
		application.selectedRows = make(map[int]bool)
		first, last := min(application.selectionAnchor, row), max(application.selectionAnchor, row)
		for index := first; index <= last; index++ {
			application.selectedRows[index] = true
		}
		application.selected = row
		return
	}
	if modifiers&fyne.KeyModifierShortcutDefault != 0 {
		if application.selectedRows[row] {
			delete(application.selectedRows, row)
			application.selected = -1
			for index := range application.selectedRows {
				application.selected = index
				break
			}
		} else {
			application.selectedRows[row] = true
			application.selected = row
		}
		application.selectionAnchor = row
		return
	}
	application.selectedRows = map[int]bool{row: true}
	application.selected = row
	application.selectionAnchor = row
}

func (application *Application) selectedIndices() []int {
	indices := make([]int, 0, len(application.selectedRows))
	for index := range application.selectedRows {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices
}

func (application *Application) clearSelection() {
	application.selected = -1
	application.selectedRows = make(map[int]bool)
	application.selectionAnchor = -1
}

func fileColumnText(file media.File, column int) string {
	switch column {
	case 0:
		if file.Path == "" {
			return "Expected episode"
		}
		return filepath.Base(file.Path)
	case 1:
		return string(file.Status)
	default:
		return file.Proposed
	}
}

func (application *Application) addFile() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, application.window)
			return
		}
		if reader == nil {
			return
		}
		path := reader.URI().Path()
		_ = reader.Close()
		application.addPaths([]string{path})
	}, application.window)
}

func (application *Application) addFolder() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, application.window)
			return
		}
		if uri != nil {
			application.addPaths([]string{uri.Path()})
		}
	}, application.window)
}

func (application *Application) importShow() {
	if application.busy {
		return
	}
	options := application.settings.Load()
	if err := application.settings.Validate(options); err != nil {
		dialog.ShowError(err, application.window)
		return
	}
	client := application.tmdbClient(options.TMDBToken)
	queryEntry := widget.NewEntry()
	queryEntry.SetPlaceHolder("TV show title")
	showSelect := widget.NewSelect(nil, nil)
	seasonSelect := widget.NewSelect(nil, nil)
	message := widget.NewLabel("Search for a show, select it, then load its seasons.")
	message.Wrapping = fyne.TextWrapWord
	var shows []tmdb.Show
	var seasons []tmdb.Season
	var importDialog dialog.Dialog

	searchButton := widget.NewButton("Search TMDB", nil)
	loadSeasonsButton := widget.NewButton("Load Seasons", nil)
	importButton := widget.NewButton("Import Episodes", nil)
	cancelButton := widget.NewButton("Cancel Request", func() {
		if application.cancel != nil {
			application.cancel()
			message.SetText("Cancelling…")
		}
	})
	closeButton := widget.NewButton("Close", func() {
		if application.cancel != nil {
			application.cancel()
		}
		importDialog.Hide()
	})
	setRequestState := func(running bool) {
		application.busy = running
		if !running {
			application.cancel = nil
		}
		setEnabled(searchButton, !running)
		setEnabled(loadSeasonsButton, !running && len(shows) > 0)
		setEnabled(importButton, !running && len(seasons) > 0)
		setEnabled(cancelButton, running)
		application.updateButtons()
	}
	setRequestState(false)

	searchButton.OnTapped = func() {
		query := strings.TrimSpace(queryEntry.Text)
		if query == "" {
			message.SetText("Enter a TV show title.")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		application.cancel = cancel
		setRequestState(true)
		message.SetText("Searching TMDB…")
		go func() {
			results, err := client.SearchTV(ctx, query, 0, options.Language, options.IncludeAdult)
			fyne.Do(func() {
				setRequestState(false)
				if ctx.Err() != nil {
					message.SetText("Search cancelled. Try again when ready.")
					return
				}
				if err != nil {
					message.SetText("Show search failed: " + err.Error())
					return
				}
				shows = results
				seasons = nil
				labels := make([]string, len(results))
				for index, show := range results {
					labels[index] = fmt.Sprintf("%s (%s) — TMDB %d", show.Name, optionalDateYear(show.FirstAirDate), show.ID)
				}
				showSelect.SetOptions(labels)
				seasonSelect.SetOptions(nil)
				if len(labels) == 0 {
					message.SetText("No shows found. Try a different title.")
					return
				}
				showSelect.SetSelectedIndex(0)
				loadSeasonsButton.Enable()
				message.SetText(fmt.Sprintf("%d show(s) found. Select the correct result.", len(labels)))
			})
		}()
	}

	loadSeasonsButton.OnTapped = func() {
		selected := showSelect.SelectedIndex()
		if selected < 0 || selected >= len(shows) {
			message.SetText("Select a show first.")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		application.cancel = cancel
		setRequestState(true)
		message.SetText("Loading seasons…")
		go func() {
			results, err := client.ShowSeasons(ctx, shows[selected].ID, options.Language)
			fyne.Do(func() {
				setRequestState(false)
				if ctx.Err() != nil {
					message.SetText("Season loading cancelled. Existing rows were not changed.")
					return
				}
				if err != nil {
					message.SetText("Season loading failed: " + err.Error())
					return
				}
				seasons = results
				labels := []string{"All seasons"}
				for _, season := range results {
					labels = append(labels, fmt.Sprintf("%s (%d episodes)", season.Name, season.EpisodeCount))
				}
				seasonSelect.SetOptions(labels)
				seasonSelect.SetSelectedIndex(0)
				importButton.Enable()
				message.SetText("Choose one season or All seasons.")
			})
		}()
	}

	importButton.OnTapped = func() {
		showIndex := showSelect.SelectedIndex()
		seasonIndex := seasonSelect.SelectedIndex()
		if showIndex < 0 || showIndex >= len(shows) || seasonIndex < 0 {
			message.SetText("Select a show and season first.")
			return
		}
		var numbers []int
		if seasonIndex == 0 {
			numbers = selectedSeasonNumbers(seasons, options.IncludeSpecials)
		} else if seasonIndex-1 < len(seasons) {
			numbers = []int{seasons[seasonIndex-1].SeasonNumber}
		}
		if len(numbers) == 0 {
			message.SetText("The selected show has no importable seasons.")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		application.cancel = cancel
		setRequestState(true)
		message.SetText("Loading episodes…")
		go func(show tmdb.Show) {
			episodes, err := loadShowEpisodes(ctx, client, show.ID, numbers, options.Language)
			fyne.Do(func() {
				setRequestState(false)
				if ctx.Err() != nil {
					message.SetText("Episode import cancelled. Existing rows were not changed.")
					return
				}
				if err != nil {
					message.SetText("Episode import failed; existing rows were not changed: " + err.Error())
					return
				}
				if len(episodes) == 0 {
					message.SetText("The selected season returned no episodes; existing rows were not changed.")
					return
				}
				added, err := application.importEpisodes(show, episodes)
				if err != nil {
					message.SetText("Episode import failed: " + err.Error())
					return
				}
				importDialog.Hide()
				application.setStatus(fmt.Sprintf("Imported %d expected episode(s).", added))
				application.refresh()
			})
		}(shows[showIndex])
	}

	content := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Search", queryEntry),
			widget.NewFormItem("Show", showSelect),
			widget.NewFormItem("Season", seasonSelect),
		),
		message,
		container.NewHBox(searchButton, loadSeasonsButton, importButton, cancelButton, closeButton),
	)
	importDialog = dialog.NewCustomWithoutButtons("Import Show", content, application.window)
	importDialog.Resize(fyne.NewSize(720, 420))
	importDialog.Show()
}

func (application *Application) addPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	options := application.settings.Load()
	discovered, err := media.Discover(paths, media.DiscoverOptions{
		Recursive: options.ScanSubfolders, IgnoreHidden: options.IgnoreHidden,
	})
	if err != nil {
		dialog.ShowError(err, application.window)
		return
	}

	existing := make(map[string]struct{}, len(application.files))
	for _, file := range application.files {
		existing[pathKey(file.Path)] = struct{}{}
	}
	added := 0
	for _, file := range discovered {
		if _, duplicate := existing[pathKey(file.Path)]; duplicate {
			continue
		}
		existing[pathKey(file.Path)] = struct{}{}
		application.files = append(application.files, file)
		added++
	}
	application.setStatus(fmt.Sprintf("Added %d video file(s).", added))
	application.reconcileExpectedEpisodes()
	application.refresh()

	if added > 0 && options.AutoMatch {
		if options.TMDBToken == "" {
			application.setStatus("Files added. Enter your TMDB token in Settings to match them.")
			return
		}
		application.startMatch()
	}
}

func (application *Application) importEpisodes(show tmdb.Show, episodes []tmdb.Episode) (int, error) {
	options := application.settings.Load()
	existing := make(map[string]struct{}, len(application.files))
	for _, file := range application.files {
		if file.Imported {
			existing[expectedKey(file.Candidate)] = struct{}{}
		}
	}

	title := show.Name
	if options.PreferOriginalTitle && show.OriginalName != "" {
		title = show.OriginalName
	}
	if title == "" {
		title = show.OriginalName
	}
	imported := make([]media.File, 0, len(episodes))
	for _, episode := range episodes {
		candidate := media.Candidate{
			ID: show.ID, Kind: media.Episode, Title: title, OriginalTitle: show.OriginalName,
			SeriesYear: dateYear(show.FirstAirDate), Season: episode.SeasonNumber, Episode: episode.EpisodeNumber,
			EpisodeTitle: episode.Name,
		}
		if options.PreferOriginalTitle && episode.OriginalName != "" {
			candidate.EpisodeTitle = episode.OriginalName
		}
		if candidate.EpisodeTitle == "" {
			candidate.EpisodeTitle = fmt.Sprintf("Episode %02d", episode.EpisodeNumber)
		}
		key := expectedKey(candidate)
		if _, duplicate := existing[key]; duplicate {
			continue
		}
		proposed, err := options.FormatName("", candidate)
		if err != nil {
			return 0, err
		}
		existing[key] = struct{}{}
		imported = append(imported, media.File{
			Imported: true,
			Parsed: media.Parsed{
				Kind: media.Episode, Query: title, Year: candidate.SeriesYear,
				Season: candidate.Season, Episode: candidate.Episode,
			},
			Candidate: candidate, Proposed: proposed, Status: media.Expected,
			Message: "waiting for a local file",
		})
	}

	application.files = append(application.files, imported...)
	application.reconcileExpectedEpisodes()
	return len(imported), nil
}

func (application *Application) reconcileExpectedEpisodes() {
	options := application.settings.Load()
	for localIndex := 0; localIndex < len(application.files); localIndex++ {
		local := application.files[localIndex]
		if !isUnpairedEpisodeFile(local) {
			continue
		}

		localMatches := 0
		for _, candidate := range application.files {
			if isUnpairedEpisodeFile(candidate) &&
				candidate.Parsed.Season == local.Parsed.Season && candidate.Parsed.Episode == local.Parsed.Episode {
				localMatches++
			}
		}
		if localMatches != 1 {
			continue
		}

		expectedIndex := -1
		for index, expected := range application.files {
			if !expected.IsExpectedEpisode() ||
				expected.Parsed.Season != local.Parsed.Season || expected.Parsed.Episode != local.Parsed.Episode {
				continue
			}
			if expectedIndex >= 0 {
				expectedIndex = -1
				break
			}
			expectedIndex = index
		}
		if expectedIndex < 0 {
			continue
		}

		paired, err := pairEpisode(local, application.files[expectedIndex], options)
		if err != nil {
			application.files[localIndex].Status = media.Error
			application.files[localIndex].Message = err.Error()
			continue
		}
		application.files[localIndex] = paired
		application.files = append(application.files[:expectedIndex], application.files[expectedIndex+1:]...)
		if expectedIndex < localIndex {
			localIndex--
		}
	}
}

func isUnpairedEpisodeFile(file media.File) bool {
	return !file.Imported && file.Path != "" && file.Parsed.Kind == media.Episode && !file.Parsed.MultiEpisode
}

func (application *Application) pairExpected(localIndex, expectedIndex int) error {
	if localIndex < 0 || localIndex >= len(application.files) ||
		expectedIndex < 0 || expectedIndex >= len(application.files) || localIndex == expectedIndex {
		return fmt.Errorf("invalid pairing selection")
	}
	local := application.files[localIndex]
	expected := application.files[expectedIndex]
	if local.Path == "" || !expected.IsExpectedEpisode() {
		return fmt.Errorf("choose a local file and an unpaired expected episode")
	}

	options := application.settings.Load()
	if local.IsEpisodePairing() {
		restored, err := unpairedExpected(local, options)
		if err != nil {
			return err
		}
		application.files = append(application.files, restored)
	}
	paired, err := pairEpisode(local, expected, options)
	if err != nil {
		return err
	}
	application.files[localIndex] = paired
	application.files = append(application.files[:expectedIndex], application.files[expectedIndex+1:]...)
	return nil
}

func (application *Application) unpairExpected(index int) error {
	if index < 0 || index >= len(application.files) {
		return fmt.Errorf("invalid pairing selection")
	}
	paired := application.files[index]
	if !paired.IsEpisodePairing() {
		return fmt.Errorf("selected row is not paired")
	}
	expected, err := unpairedExpected(paired, application.settings.Load())
	if err != nil {
		return err
	}
	application.files[index] = media.File{
		Path: paired.Path, Parsed: paired.Parsed, Status: media.Unmatched, Message: "not paired",
	}
	application.files = append(application.files, expected)
	return nil
}

func unpairedExpected(file media.File, options settings.Settings) (media.File, error) {
	proposed, err := options.FormatName("", file.Candidate)
	if err != nil {
		return media.File{}, err
	}
	file.Path = ""
	file.Imported = true
	file.Parsed = media.Parsed{
		Kind: media.Episode, Query: file.Candidate.Title, Year: file.Candidate.SeriesYear,
		Season: file.Candidate.Season, Episode: file.Candidate.Episode,
	}
	file.Proposed = proposed
	file.Status = media.Expected
	file.Message = "waiting for a local file"
	return file, nil
}

func pairEpisode(local, expected media.File, options settings.Settings) (media.File, error) {
	proposed, err := options.FormatName(local.Path, expected.Candidate)
	if err != nil {
		return media.File{}, err
	}
	expected.Path = local.Path
	expected.Imported = true
	expected.Parsed = local.Parsed
	expected.Proposed = proposed
	expected.Status = media.Ready
	expected.Message = ""
	return expected, nil
}

func expectedKey(candidate media.Candidate) string {
	return fmt.Sprintf("%d/%d/%d", candidate.ID, candidate.Season, candidate.Episode)
}

func dateYear(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func selectedSeasonNumbers(seasons []tmdb.Season, includeSpecials bool) []int {
	numbers := make([]int, 0, len(seasons))
	for _, season := range seasons {
		if season.SeasonNumber == 0 && !includeSpecials {
			continue
		}
		numbers = append(numbers, season.SeasonNumber)
	}
	return numbers
}

func loadShowEpisodes(
	ctx context.Context,
	client *tmdb.Client,
	showID int,
	seasons []int,
	language string,
) ([]tmdb.Episode, error) {
	var episodes []tmdb.Episode
	for _, season := range seasons {
		loaded, err := client.SeasonEpisodes(ctx, showID, season, language)
		if err != nil {
			return nil, fmt.Errorf("load season %d: %w", season, err)
		}
		episodes = append(episodes, loaded...)
	}
	return episodes, nil
}

func (application *Application) removeSelected() {
	if application.selected < 0 || application.selected >= len(application.files) || application.busy {
		return
	}
	if len(application.selectedRows) == 0 {
		application.selectedRows = map[int]bool{application.selected: true}
	}
	remaining := make([]media.File, 0, len(application.files)-len(application.selectedRows))
	for index, file := range application.files {
		if !application.selectedRows[index] {
			remaining = append(remaining, file)
		}
	}
	application.files = remaining
	application.clearSelection()
	application.refresh()
}

func (application *Application) clear() {
	if application.cancel != nil {
		application.cancel()
	}
	application.files = nil
	application.clearSelection()
	application.setStatus("List cleared.")
	application.refresh()
}

func (application *Application) matchOrCancel() {
	if application.busy {
		if application.cancel != nil {
			application.cancel()
		}
		return
	}
	application.startMatch()
}

func (application *Application) startMatch() {
	if len(application.files) == 0 || application.busy {
		return
	}
	options := application.settings.Load()
	if err := application.settings.Validate(options); err != nil {
		dialog.ShowError(err, application.window)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	application.cancel = cancel
	application.busy = true
	application.setStatus("Matching files with TMDB…")
	application.updateButtons()

	input := append([]media.File(nil), application.files...)
	go func() {
		engine := matcher.New(application.tmdbClient(options.TMDBToken))
		results := engine.Match(ctx, input, options)
		fyne.Do(func() {
			application.cancel = nil
			application.busy = false
			if ctx.Err() != nil {
				application.setStatus("Matching cancelled.")
			} else {
				if options.SortMatchedByStatus {
					sortMatchedFiles(results)
					application.clearSelection()
				}
				application.files = results
				application.setStatus(matchSummary(results))
			}
			application.refresh()
		})
	}()
}

func (application *Application) reviewSelected() {
	if !application.canReview() {
		return
	}
	index := application.selected
	file := application.files[index]
	if file.Path == "" {
		return
	}
	options := application.settings.Load()
	candidates := reviewCandidates(file.Candidates)
	selected := -1
	message := widget.NewLabel(fmt.Sprintf("Select the best match for %q.", file.Parsed.Query))
	message.Wrapping = fyne.TextWrapWord
	candidateList := widget.NewList(
		func() int { return len(candidates) },
		func() fyne.CanvasObject {
			poster := canvas.NewImageFromResource(theme.FileImageIcon())
			poster.FillMode = canvas.ImageFillContain
			poster.SetMinSize(fyne.NewSize(40, 60))
			title := widget.NewLabel("")
			title.TextStyle.Bold = true
			overview := widget.NewLabel("")
			return container.NewHBox(poster, container.NewVBox(title, overview))
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			candidate := candidates[id]
			row := object.(*fyne.Container)
			poster := canvas.NewImageFromResource(theme.FileImageIcon())
			if candidate.PosterPath != "" {
				poster = canvas.NewImageFromURI(storage.NewURI("https://image.tmdb.org/t/p/w154" + candidate.PosterPath))
			}
			poster.FillMode = canvas.ImageFillContain
			poster.SetMinSize(fyne.NewSize(40, 60))
			row.Objects[0] = poster
			text := row.Objects[1].(*fyne.Container)
			year := candidate.Year
			if candidate.Kind == media.Episode {
				year = candidate.SeriesYear
			}
			text.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%s (%d)", candidate.Title, year))
			text.Objects[1].(*widget.Label).SetText(truncate(candidate.Overview, 90))
			row.Refresh()
		},
	)
	candidateList.OnSelected = func(id widget.ListItemID) { selected = id }
	if len(candidates) > 0 {
		candidateList.Select(0)
	} else {
		message.SetText("No matches are available. Search TMDB below.")
	}

	reviewCtx, cancelReview := context.WithCancel(context.Background())
	var reviewDialog dialog.Dialog
	selectButton := widget.NewButtonWithIcon("Select", theme.ConfirmIcon(), func() {
		if selected < 0 || selected >= len(candidates) {
			message.SetText("Choose a candidate first.")
			return
		}
		selectedCandidate := candidates[selected]
		input := append([]media.File(nil), application.files...)
		indices := []int{index}
		if selectedCandidate.Kind == media.Episode && file.Parsed.Kind == media.Episode {
			indices = matcher.EpisodeGroupIndices(input, index)
		}
		message.SetText("Loading metadata…")
		go func() {
			engine := matcher.New(application.tmdbClient(options.TMDBToken))
			resolved := engine.ResolveGroup(reviewCtx, input, indices, selectedCandidate, options)
			fyne.Do(func() {
				if reviewCtx.Err() != nil {
					return
				}
				for _, member := range indices {
					if member >= len(application.files) || application.files[member].Path != input[member].Path {
						return
					}
				}
				for _, member := range indices {
					application.files[member] = resolved[member]
				}
				reviewDialog.Hide()
				application.setStatus(fmt.Sprintf("Match applied to %d file(s).", len(indices)))
				application.refresh()
			})
		}()
	})
	setEnabled(selectButton, len(candidates) > 0)
	buttons := []fyne.CanvasObject{selectButton}
	hasExpectedEpisodes := file.IsEpisodePairing()
	for _, candidate := range application.files {
		hasExpectedEpisodes = hasExpectedEpisodes || candidate.IsExpectedEpisode()
	}
	if hasExpectedEpisodes {
		pairButton := widget.NewButton("Expected Episodes…", func() {
			reviewDialog.Hide()
			application.reviewExpectedPairing(index)
		})
		buttons = append(buttons, pairButton)
	}
	top := fyne.CanvasObject(message)
	if len(candidates) == 0 {
		titleEntry := widget.NewEntry()
		titleEntry.SetText(file.Parsed.Query)
		yearEntry := widget.NewEntry()
		yearEntry.SetText(optionalNumber(file.Parsed.Year))
		idEntry := widget.NewEntry()
		idEntry.SetPlaceHolder("Optional")
		kindSelect := widget.NewSelect([]string{"Movie", "TV series"}, nil)
		kindSelect.SetSelected("Movie")
		if file.Parsed.Kind == media.Episode {
			kindSelect.SetSelected("TV series")
		}
		var searchControls *fyne.Container
		searchButton := widget.NewButtonWithIcon("Search", theme.SearchIcon(), nil)
		searchButton.OnTapped = func() {
			parsed, tmdbID, err := manualSearchParsed(
				titleEntry.Text, yearEntry.Text, kindSelect.Selected, idEntry.Text, file.Parsed,
			)
			if err != nil {
				message.SetText(err.Error())
				return
			}
			setEnabled(searchButton, false)
			message.SetText("Searching TMDB…")
			go func() {
				engine := matcher.New(application.tmdbClient(options.TMDBToken))
				var found []media.Candidate
				if tmdbID > 0 {
					candidate, lookupErr := engine.Lookup(reviewCtx, parsed, tmdbID, options)
					err = lookupErr
					if err == nil {
						found = []media.Candidate{candidate}
					}
				} else {
					found, err = engine.Search(reviewCtx, parsed, options)
				}
				fyne.Do(func() {
					if reviewCtx.Err() != nil {
						return
					}
					setEnabled(searchButton, true)
					if err != nil {
						message.SetText("Search failed: " + err.Error())
						return
					}
					candidates = reviewCandidates(found)
					selected = -1
					candidateList.UnselectAll()
					candidateList.Refresh()
					setEnabled(selectButton, len(candidates) > 0)
					if len(candidates) == 0 {
						message.SetText("No matches found. Try another title, year, or type.")
						return
					}
					searchControls.Hide()
					message.SetText(fmt.Sprintf("Found %d match(es). Choose one below.", len(candidates)))
					candidateList.Select(0)
				})
			}()
		}
		searchControls = container.NewVBox(
			widget.NewForm(widget.NewFormItem("Title", titleEntry)),
			container.NewGridWithColumns(
				3,
				container.NewVBox(widget.NewLabel("Year"), yearEntry),
				container.NewVBox(widget.NewLabel("Type"), kindSelect),
				container.NewVBox(widget.NewLabel("TMDB ID"), idEntry),
			),
			container.NewCenter(searchButton),
		)
		top = container.NewVBox(message, searchControls)
	}
	content := container.NewBorder(top, container.NewCenter(container.NewHBox(buttons...)), nil, nil, candidateList)
	title := "Select Movie"
	if file.Parsed.Kind == media.Episode {
		title = "Select TV Series"
	}
	reviewDialog = dialog.NewCustom(title, "Skip", content, application.window)
	reviewDialog.SetOnClosed(cancelReview)
	reviewDialog.Resize(fyne.NewSize(700, 500))
	reviewDialog.Show()
}

func (application *Application) reviewExpectedPairing(index int) {
	if index < 0 || index >= len(application.files) {
		return
	}
	file := application.files[index]
	var expectedIndices []int
	var expectedLabels []string
	for expectedIndex, expected := range application.files {
		if expected.IsExpectedEpisode() {
			expectedIndices = append(expectedIndices, expectedIndex)
			expectedLabels = append(expectedLabels, fmt.Sprintf(
				"%s — S%02dE%02d — %s",
				expected.Candidate.Title,
				expected.Candidate.Season,
				expected.Candidate.Episode,
				expected.Candidate.EpisodeTitle,
			))
		}
	}
	expectedSelect := widget.NewSelect(expectedLabels, nil)
	if len(expectedLabels) > 0 {
		expectedSelect.SetSelectedIndex(0)
	}
	message := widget.NewLabel("Choose an expected episode to pair with this file.")
	var pairingDialog dialog.Dialog
	pairButton := widget.NewButton("Pair", func() {
		selected := expectedSelect.SelectedIndex()
		if selected < 0 || selected >= len(expectedIndices) {
			message.SetText("Choose an expected episode first.")
			return
		}
		if err := application.pairExpected(index, expectedIndices[selected]); err != nil {
			message.SetText(err.Error())
			return
		}
		pairingDialog.Hide()
		application.clearSelection()
		application.setStatus("Expected episode paired.")
		application.refresh()
	})
	setEnabled(pairButton, len(expectedLabels) > 0)
	removeButton := widget.NewButton("Remove Pairing", func() {
		if err := application.unpairExpected(index); err != nil {
			message.SetText(err.Error())
			return
		}
		pairingDialog.Hide()
		application.clearSelection()
		application.setStatus("Pairing removed.")
		application.refresh()
	})
	setEnabled(removeButton, file.IsEpisodePairing())
	content := container.NewVBox(
		message,
		widget.NewForm(widget.NewFormItem("Expected episode", expectedSelect)),
		container.NewCenter(container.NewHBox(pairButton, removeButton)),
	)
	pairingDialog = dialog.NewCustom("Expected Episode Pairing", "Cancel", content, application.window)
	pairingDialog.Resize(fyne.NewSize(650, 220))
	pairingDialog.Show()
}

func (application *Application) confirmRename() {
	if !application.canRename() {
		return
	}
	count := len(application.renameOperations())
	apply := func() { application.applyRename() }
	if application.settings.Load().ConfirmRename {
		dialog.ShowConfirm(
			"Rename files",
			fmt.Sprintf("Rename %d file(s)? Existing files will never be overwritten.", count),
			func(confirmed bool) {
				if confirmed {
					apply()
				}
			},
			application.window,
		)
		return
	}
	apply()
}

func (application *Application) applyRename() {
	operations := application.renameOperations()
	application.busy = true
	application.setStatus("Renaming files…")
	application.updateButtons()
	go func() {
		err := application.renamer.Apply(operations)
		fyne.Do(func() {
			application.busy = false
			if err != nil {
				dialog.ShowError(err, application.window)
				application.setStatus("Rename failed; FileGot attempted to restore original names.")
			} else {
				application.files = remainingAfterRename(application.files, operations)
				application.clearSelection()
				application.setStatus(fmt.Sprintf("Renamed %d file(s). Undo Last is available.", len(operations)))
			}
			application.refresh()
		})
	}()
}

func (application *Application) renameOperations() []rename.Operation {
	operations := make([]rename.Operation, 0, len(application.files))
	for index, file := range application.files {
		if len(application.selectedRows) > 0 && !application.selectedRows[index] {
			continue
		}
		if file.Path == "" || file.Status != media.Ready || unchanged(file) {
			continue
		}
		operations = append(operations, rename.Operation{From: file.Path, To: matcher.Destination(file)})
	}
	return operations
}

func remainingAfterRename(files []media.File, operations []rename.Operation) []media.File {
	renamed := make(map[string]bool, len(operations))
	for _, operation := range operations {
		renamed[operation.From] = true
	}
	remaining := make([]media.File, 0, len(files))
	for _, file := range files {
		if !renamed[file.Path] {
			remaining = append(remaining, file)
		}
	}
	return remaining
}

func (application *Application) refreshProposedNames() error {
	files := append([]media.File(nil), application.files...)
	options := application.settings.Load()
	for index, file := range files {
		if !file.Imported {
			continue
		}
		proposed, err := options.FormatName(file.Path, file.Candidate)
		if err != nil {
			return err
		}
		files[index].Proposed = proposed
	}
	application.files = files
	return nil
}

func (application *Application) undo() {
	if !application.renamer.HasUndo() || application.busy {
		return
	}
	dialog.ShowConfirm("Undo last rename", "Restore every file from the last completed batch?", func(confirmed bool) {
		if !confirmed {
			return
		}
		application.busy = true
		application.updateButtons()
		go func() {
			err := application.renamer.Undo()
			fyne.Do(func() {
				application.busy = false
				if err != nil {
					dialog.ShowError(err, application.window)
					application.setStatus("Undo failed.")
				} else {
					application.files = nil
					application.clearSelection()
					application.setStatus("Last rename restored.")
				}
				application.refresh()
			})
		}()
	}, application.window)
}

func (application *Application) canRename() bool {
	return !application.busy && len(application.renameOperations()) > 0
}

func (application *Application) refresh() {
	application.table.Refresh()
	application.updateFileArea()
	application.updateDetails()
	application.updateButtons()
}

func (application *Application) updateButtons() {
	setEnabled(application.importShowButton, !application.busy)
	setEnabled(application.removeButton, application.canRemove())
	setEnabled(application.clearButton, !application.busy && len(application.files) > 0)
	setEnabled(application.reviewButton, application.canReview())
	setEnabled(application.renameButton, application.canRename())
	setEnabled(application.undoButton, !application.busy && application.renamer.HasUndo())

	if application.busy && application.cancel != nil {
		application.matchButton.SetText("Cancel")
		application.matchButton.Enable()
	} else {
		application.matchButton.SetText("Match")
		setEnabled(application.matchButton, !application.busy && len(application.files) > 0)
	}
}

func (application *Application) canReview() bool {
	return !application.busy && len(application.selectedRows) == 1 &&
		application.selected >= 0 && application.selected < len(application.files) &&
		application.files[application.selected].Path != "" &&
		application.files[application.selected].Status != media.Unsupported
}

func (application *Application) canRemove() bool {
	return !application.busy && application.selected >= 0 && application.selected < len(application.files)
}

func (application *Application) contextActions() (*fyne.MenuItem, *fyne.MenuItem, *fyne.MenuItem) {
	renameItem := fyne.NewMenuItem("Rename", application.confirmRename)
	renameItem.Disabled = !application.canRename()
	reviewItem := fyne.NewMenuItem("Review", application.reviewSelected)
	reviewItem.Disabled = !application.canReview()
	removeItem := fyne.NewMenuItem("Remove", application.removeSelected)
	removeItem.Disabled = !application.canRemove()
	return renameItem, reviewItem, removeItem
}

func (application *Application) updateFileArea() {
	if len(application.files) == 0 {
		application.table.Hide()
		application.empty.Show()
		return
	}
	application.empty.Hide()
	application.table.Show()
}

func (application *Application) updateDetails() {
	if application.selected < 0 || application.selected >= len(application.files) {
		application.details.Hide()
		application.window.Content().Refresh()
		return
	}
	file := application.files[application.selected]
	details := file.Path
	if details == "" {
		details = fmt.Sprintf(
			"%s S%02dE%02d",
			file.Candidate.Title,
			file.Candidate.Season,
			file.Candidate.Episode,
		)
	}
	details += " — " + string(file.Status)
	if file.Message != "" {
		details += " — " + file.Message
	}
	application.details.SetText(details)
	application.details.Show()
	application.window.Content().Refresh()
}

func (application *Application) setStatus(value string) {
	application.status.SetText(value)
}

func setEnabled(button *widget.Button, enabled bool) {
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func matchSummary(files []media.File) string {
	counts := make(map[media.Status]int)
	for _, file := range files {
		if unchanged(file) {
			continue
		}
		counts[file.Status]++
	}
	return fmt.Sprintf(
		"Matching complete: %d ready, %d need review, %d unmatched, %d errors.",
		counts[media.Ready], counts[media.Review], counts[media.Unmatched], counts[media.Error]+counts[media.Unsupported],
	)
}

func unchanged(file media.File) bool {
	return file.Status == media.Ready && filepath.Base(file.Path) == file.Proposed
}

func rowStatus(file media.File) media.Status {
	if unchanged(file) {
		return media.Unsupported
	}
	return file.Status
}

func sortMatchedFiles(files []media.File) {
	sort.SliceStable(files, func(left, right int) bool {
		return matchedFileRank(files[left]) < matchedFileRank(files[right])
	})
}

func matchedFileRank(file media.File) int {
	if unchanged(file) {
		return 3
	}
	switch file.Status {
	case media.Unmatched:
		return 0
	case media.Review:
		return 1
	case media.Ready:
		return 2
	default:
		return 4
	}
}

func reviewParsed(kind, query, year, season, episode string) (media.Parsed, error) {
	parsed := media.Parsed{Kind: media.Movie, Query: strings.TrimSpace(query)}
	if parsed.Query == "" {
		return parsed, fmt.Errorf("search text is required")
	}
	if kind == "TV episode" {
		parsed.Kind = media.Episode
	}
	var err error
	if parsed.Year, err = parseOptionalNumber("year", year); err != nil {
		return parsed, err
	}
	if parsed.Kind == media.Episode {
		if parsed.Season, err = parseRequiredNumber("season", season); err != nil {
			return parsed, err
		}
		if parsed.Episode, err = parseRequiredNumber("episode", episode); err != nil {
			return parsed, err
		}
	}
	return parsed, nil
}

func manualSearchParsed(title, year, kind, id string, original media.Parsed) (media.Parsed, int, error) {
	parsed := media.Parsed{Query: strings.TrimSpace(title)}
	switch kind {
	case "Movie":
		parsed.Kind = media.Movie
	case "TV series":
		parsed.Kind = media.Episode
		parsed.Season = original.Season
		parsed.Episode = original.Episode
	default:
		return parsed, 0, fmt.Errorf("type is required")
	}
	tmdbID := 0
	var err error
	if strings.TrimSpace(id) != "" {
		if tmdbID, err = parseRequiredNumber("TMDB ID", id); err != nil || tmdbID == 0 {
			return parsed, 0, fmt.Errorf("TMDB ID must be a positive number")
		}
		return parsed, tmdbID, nil
	}
	if parsed.Query == "" {
		return parsed, 0, fmt.Errorf("title is required")
	}
	if parsed.Year, err = parseOptionalNumber("year", year); err != nil {
		return parsed, 0, err
	}
	return parsed, 0, nil
}

func reviewCandidates(candidates []media.Candidate) []media.Candidate {
	limit := len(candidates)
	if limit > 10 && candidates[0].Kind == media.Episode {
		limit = 10
	}
	return append([]media.Candidate(nil), candidates[:limit]...)
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func parseOptionalNumber(name, value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return parseRequiredNumber(name, value)
}

func parseRequiredNumber(name, value string) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", name)
	}
	return number, nil
}

func optionalNumber(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func optionalDateYear(value string) string {
	year := dateYear(value)
	if year == 0 {
		return "unknown year"
	}
	return strconv.Itoa(year)
}

func statusRowColor(status media.Status, background color.Color) color.Color {
	dark := isDarkColor(background)
	switch status {
	case media.Expected:
		if dark {
			return color.NRGBA{R: 0x24, G: 0x34, B: 0x48, A: 0xff}
		}
		return color.NRGBA{R: 0xe8, G: 0xf1, B: 0xfc, A: 0xff}
	case media.Ready:
		if dark {
			return color.NRGBA{R: 0x17, G: 0x3c, B: 0x2b, A: 0xff}
		}
		return color.NRGBA{R: 0xe9, G: 0xf7, B: 0xef, A: 0xff}
	case media.Review, media.Unmatched:
		if dark {
			return color.NRGBA{R: 0x47, G: 0x38, B: 0x17, A: 0xff}
		}
		return color.NRGBA{R: 0xff, G: 0xf4, B: 0xd6, A: 0xff}
	case media.Conflict, media.Error:
		if dark {
			return color.NRGBA{R: 0x48, G: 0x24, B: 0x24, A: 0xff}
		}
		return color.NRGBA{R: 0xfc, G: 0xe8, B: 0xe6, A: 0xff}
	case media.Unsupported:
		if dark {
			return color.NRGBA{R: 0x2d, G: 0x30, B: 0x35, A: 0xff}
		}
		return color.NRGBA{R: 0xef, G: 0xf1, B: 0xf3, A: 0xff}
	default:
		return color.Transparent
	}
}

func isDarkColor(value color.Color) bool {
	red, green, blue, _ := value.RGBA()
	return (red*299+green*587+blue*114)/1000 < 0x8000
}

type fileTable struct {
	widget.Table
	modifiers    fyne.KeyModifier
	contextClick bool
	menu         func() (*fyne.MenuItem, *fyne.MenuItem, *fyne.MenuItem)
}

func newFileTable(
	length func() (int, int),
	create func() fyne.CanvasObject,
	update func(widget.TableCellID, fyne.CanvasObject),
) *fileTable {
	table := &fileTable{}
	table.Length = length
	table.CreateCell = create
	table.UpdateCell = update
	table.ExtendBaseWidget(table)
	return table
}

func (table *fileTable) Tapped(event *fyne.PointEvent) {
	table.modifiers = 0
	defer func() { table.modifiers = 0 }()
	if driver, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		table.modifiers = driver.CurrentKeyModifiers()
	}
	if table.modifiers&(fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift) != 0 {
		table.Table.UnselectAll()
	}
	table.Table.Tapped(event)
}

func (table *fileTable) TypedKey(event *fyne.KeyEvent) {
	table.modifiers = 0
	table.Table.TypedKey(event)
}

func (table *fileTable) TappedSecondary(event *fyne.PointEvent) {
	table.contextClick = true
	table.modifiers = 0
	table.Table.Tapped(event)
	table.contextClick = false
	if table.menu == nil {
		return
	}
	renameItem, reviewItem, removeItem := table.menu()
	widget.ShowPopUpMenuAtPosition(
		fyne.NewMenu("", renameItem, reviewItem, fyne.NewMenuItemSeparator(), removeItem),
		fyne.CurrentApp().Driver().CanvasForObject(table),
		event.AbsolutePosition,
	)
}

type fileTableLayout struct {
	table *fileTable
}

func (layout *fileTableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	statusWidth := fileStatusColumnWidth()
	separators := theme.Padding() * 2
	filenameWidth := max((size.Width-statusWidth-separators)/2, 220)
	layout.table.SetColumnWidth(0, filenameWidth)
	layout.table.SetColumnWidth(1, statusWidth)
	layout.table.SetColumnWidth(2, filenameWidth)
	objects[0].Resize(size)
}

func (layout *fileTableLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(440+fileStatusColumnWidth()+theme.Padding()*2, 220)
}

func fileStatusColumnWidth() float32 {
	status := widget.NewLabel(string(media.Unsupported))
	status.SizeName = theme.SizeNameCaptionText
	header := widget.NewLabelWithStyle("Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header.SizeName = theme.SizeNameCaptionText
	return max(status.MinSize().Width, header.MinSize().Width) + theme.Padding()*2
}
