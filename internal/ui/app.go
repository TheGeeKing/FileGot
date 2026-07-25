package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/thegeeking/FileGot/internal/matcher"
	"github.com/thegeeking/FileGot/internal/media"
	"github.com/thegeeking/FileGot/internal/rename"
	"github.com/thegeeking/FileGot/internal/settings"
	"github.com/thegeeking/FileGot/internal/tmdb"
)

type Application struct {
	app      fyne.App
	window   fyne.Window
	settings *settings.Store
	renamer  *rename.Manager

	files    []media.File
	table    *widget.Table
	status   *widget.Label
	selected int
	cancel   context.CancelFunc
	busy     bool

	addFileButton   *widget.Button
	addFolderButton *widget.Button
	removeButton    *widget.Button
	clearButton     *widget.Button
	matchButton     *widget.Button
	reviewButton    *widget.Button
	renameButton    *widget.Button
	undoButton      *widget.Button
}

func New(app fyne.App, store *settings.Store, renamer *rename.Manager) *Application {
	application := &Application{
		app: app, window: app.NewWindow("FileGot"), settings: store, renamer: renamer, selected: -1,
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

func (application *Application) build() {
	application.table = widget.NewTable(
		func() (int, int) { return len(application.files) + 1, 3 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			label.TextStyle = fyne.TextStyle{Bold: id.Row == 0}
			if id.Row == 0 {
				label.SetText([]string{"Original File", "Status", "Proposed Name"}[id.Col])
				return
			}
			file := application.files[id.Row-1]
			switch id.Col {
			case 0:
				label.SetText(filepath.Base(file.Path))
			case 1:
				status := string(file.Status)
				if file.Message != "" {
					status += " — " + file.Message
				}
				label.SetText(status)
			case 2:
				label.SetText(file.Proposed)
			}
		},
	)
	application.table.SetColumnWidth(0, 390)
	application.table.SetColumnWidth(1, 270)
	application.table.SetColumnWidth(2, 390)
	application.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			application.table.Unselect(id)
			return
		}
		application.selected = id.Row - 1
		application.updateButtons()
	}
	application.table.OnUnselected = func(widget.TableCellID) {
		application.selected = -1
		application.updateButtons()
	}

	application.addFileButton = widget.NewButton("Add File", application.addFile)
	application.addFolderButton = widget.NewButton("Add Folder", application.addFolder)
	application.removeButton = widget.NewButton("Remove", application.removeSelected)
	application.clearButton = widget.NewButton("Clear", application.clear)
	application.matchButton = widget.NewButton("Match", application.matchOrCancel)
	application.reviewButton = widget.NewButton("Review", application.reviewSelected)
	application.renameButton = widget.NewButton("Rename", application.confirmRename)
	application.undoButton = widget.NewButton("Undo Last", application.undo)
	settingsButton := widget.NewButton("Settings", func() {
		ShowSettings(application.app, application.settings, func() {
			application.setStatus("Settings saved.")
			application.refresh()
		})
	})
	aboutButton := widget.NewButton("About", func() { ShowAbout(application.window) })

	toolbar := container.NewHScroll(container.NewHBox(
		application.addFileButton,
		application.addFolderButton,
		application.removeButton,
		application.clearButton,
		application.matchButton,
		application.reviewButton,
		application.renameButton,
		application.undoButton,
		settingsButton,
		aboutButton,
	))
	application.status = widget.NewLabel("Drop video files or a folder to begin.")
	application.status.Wrapping = fyne.TextWrapWord
	application.window.SetContent(container.NewBorder(toolbar, application.status, nil, nil, application.table))
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
	application.updateButtons()
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
	application.refresh()

	if added > 0 && options.AutoMatch {
		if options.TMDBToken == "" {
			application.setStatus("Files added. Enter your TMDB token in Settings to match them.")
			return
		}
		application.startMatch()
	}
}

func (application *Application) removeSelected() {
	if application.selected < 0 || application.selected >= len(application.files) {
		return
	}
	application.files = append(application.files[:application.selected], application.files[application.selected+1:]...)
	application.selected = -1
	application.refresh()
}

func (application *Application) clear() {
	if application.cancel != nil {
		application.cancel()
	}
	application.files = nil
	application.selected = -1
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
		engine := matcher.New(tmdb.New(options.TMDBToken))
		results := engine.Match(ctx, input, options)
		fyne.Do(func() {
			application.cancel = nil
			application.busy = false
			if ctx.Err() != nil {
				application.setStatus("Matching cancelled.")
			} else {
				application.files = results
				application.setStatus(matchSummary(results))
			}
			application.refresh()
		})
	}()
}

func (application *Application) reviewSelected() {
	if application.selected < 0 || application.selected >= len(application.files) || application.busy {
		return
	}
	index := application.selected
	file := application.files[index]
	options := application.settings.Load()

	queryEntry := widget.NewEntry()
	queryEntry.SetText(file.Parsed.Query)
	kindSelect := widget.NewSelect([]string{"Movie", "TV episode"}, nil)
	if file.Parsed.Kind == media.Episode {
		kindSelect.SetSelected("TV episode")
	} else {
		kindSelect.SetSelected("Movie")
	}
	yearEntry := widget.NewEntry()
	yearEntry.SetText(optionalNumber(file.Parsed.Year))
	seasonEntry := widget.NewEntry()
	seasonEntry.SetText(optionalNumber(file.Parsed.Season))
	episodeEntry := widget.NewEntry()
	episodeEntry.SetText(optionalNumber(file.Parsed.Episode))
	candidateSelect := widget.NewSelect(nil, nil)
	message := widget.NewLabel("Edit the search values or choose a candidate.")
	message.Wrapping = fyne.TextWrapWord
	candidates := append([]media.Candidate(nil), file.Candidates...)

	updateCandidates := func() {
		labels := make([]string, len(candidates))
		for index, candidate := range candidates {
			year := candidate.Year
			if candidate.Kind == media.Episode {
				year = candidate.SeriesYear
			}
			labels[index] = fmt.Sprintf("%s (%d) — TMDB %d", candidate.Title, year, candidate.ID)
		}
		candidateSelect.SetOptions(labels)
		if len(labels) > 0 {
			candidateSelect.SetSelectedIndex(0)
		}
	}
	updateCandidates()
	invalidateCandidates := func(string) {
		candidates = nil
		candidateSelect.SetOptions(nil)
		message.SetText("Search again after changing match fields.")
	}
	queryEntry.OnChanged = invalidateCandidates
	yearEntry.OnChanged = invalidateCandidates
	seasonEntry.OnChanged = invalidateCandidates
	episodeEntry.OnChanged = invalidateCandidates
	kindSelect.OnChanged = invalidateCandidates

	var reviewDialog dialog.Dialog
	searchButton := widget.NewButton("Search TMDB", func() {
		parsed, err := reviewParsed(kindSelect.Selected, queryEntry.Text, yearEntry.Text, seasonEntry.Text, episodeEntry.Text)
		if err != nil {
			message.SetText(err.Error())
			return
		}
		message.SetText("Searching…")
		go func() {
			engine := matcher.New(tmdb.New(options.TMDBToken))
			results, err := engine.Search(context.Background(), parsed, options)
			fyne.Do(func() {
				if err != nil {
					message.SetText(err.Error())
					return
				}
				candidates = results
				updateCandidates()
				if len(results) == 0 {
					message.SetText("No TMDB results.")
				} else {
					message.SetText(fmt.Sprintf("%d candidate(s) found.", len(results)))
				}
			})
		}()
	})
	applyButton := widget.NewButton("Use Selected Match", func() {
		selected := candidateSelect.SelectedIndex()
		if selected < 0 || selected >= len(candidates) {
			message.SetText("Choose a candidate first.")
			return
		}
		parsed, err := reviewParsed(kindSelect.Selected, queryEntry.Text, yearEntry.Text, seasonEntry.Text, episodeEntry.Text)
		if err != nil {
			message.SetText(err.Error())
			return
		}
		selectedCandidate := candidates[selected]
		selectedCandidate.Kind = parsed.Kind
		file.Parsed = parsed
		message.SetText("Loading metadata…")
		go func() {
			engine := matcher.New(tmdb.New(options.TMDBToken))
			resolved := engine.Resolve(context.Background(), file, selectedCandidate, options)
			fyne.Do(func() {
				if resolved.Status == media.Error {
					message.SetText(resolved.Message)
					return
				}
				if index < len(application.files) && application.files[index].Path == file.Path {
					application.files[index] = resolved
				}
				reviewDialog.Hide()
				application.setStatus("Match selected.")
				application.refresh()
			})
		}()
	})

	form := widget.NewForm(
		widget.NewFormItem("Type", kindSelect),
		widget.NewFormItem("Search", queryEntry),
		widget.NewFormItem("Year", yearEntry),
		widget.NewFormItem("Season", seasonEntry),
		widget.NewFormItem("Episode", episodeEntry),
		widget.NewFormItem("Candidate", candidateSelect),
	)
	content := container.NewVBox(form, message, container.NewHBox(searchButton, applyButton))
	reviewDialog = dialog.NewCustom("Review Match", "Close", content, application.window)
	reviewDialog.Resize(fyne.NewSize(650, 430))
	reviewDialog.Show()
}

func (application *Application) confirmRename() {
	if !application.canRename() {
		return
	}
	apply := func() { application.applyRename() }
	if application.settings.Load().ConfirmRename {
		dialog.ShowConfirm(
			"Rename files",
			fmt.Sprintf("Rename %d file(s)? Existing files will never be overwritten.", len(application.files)),
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
	operations := make([]rename.Operation, 0, len(application.files))
	for _, file := range application.files {
		if filepath.Base(file.Path) == file.Proposed {
			continue
		}
		operations = append(operations, rename.Operation{
			From: file.Path,
			To:   matcher.Destination(file),
		})
	}
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
				application.files = nil
				application.selected = -1
				application.setStatus(fmt.Sprintf("Renamed %d file(s). Undo Last is available.", len(operations)))
			}
			application.refresh()
		})
	}()
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
					application.selected = -1
					application.setStatus("Last rename restored.")
				}
				application.refresh()
			})
		}()
	}, application.window)
}

func (application *Application) canRename() bool {
	if len(application.files) == 0 || application.busy {
		return false
	}
	changes := 0
	for _, file := range application.files {
		if file.Status != media.Ready || file.Proposed == "" {
			return false
		}
		if filepath.Base(file.Path) != file.Proposed {
			changes++
		}
	}
	return changes > 0
}

func (application *Application) refresh() {
	application.table.Refresh()
	application.updateButtons()
}

func (application *Application) updateButtons() {
	setEnabled(application.addFileButton, !application.busy)
	setEnabled(application.addFolderButton, !application.busy)
	setEnabled(application.removeButton, !application.busy && application.selected >= 0)
	setEnabled(application.clearButton, !application.busy && len(application.files) > 0)
	canReview := !application.busy && application.selected >= 0 &&
		application.files[application.selected].Status != media.Unsupported
	setEnabled(application.reviewButton, canReview)
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
		counts[file.Status]++
	}
	return fmt.Sprintf(
		"Matching complete: %d ready, %d need review, %d unmatched, %d errors.",
		counts[media.Ready], counts[media.Review], counts[media.Unmatched], counts[media.Error]+counts[media.Unsupported],
	)
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
