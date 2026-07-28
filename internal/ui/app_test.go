package ui

import (
	"context"
	"image/color"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/matcher"
	"github.com/TheGeeKing/FileGot/internal/media"
	"github.com/TheGeeKing/FileGot/internal/metadata"
	"github.com/TheGeeKing/FileGot/internal/rename"
	"github.com/TheGeeKing/FileGot/internal/settings"
	"github.com/TheGeeKing/FileGot/internal/tmdb"
)

func TestMainWindowPresentation(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "last-rename.json")),
	)

	prompt := findLabelWithText(application.window.Content(), "Drop video files or a folder here")
	if prompt == nil || !application.empty.Visible() || application.table.Visible() {
		t.Fatal("empty window should show the drop prompt instead of the table")
	}
	if !menuContains(application.window.MainMenu(), "Add File") ||
		!menuContains(application.window.MainMenu(), "Add Folder") ||
		!menuContains(application.window.MainMenu(), "Settings") ||
		!menuContains(application.window.MainMenu(), "About") {
		t.Fatal("main menu should contain Add File, Add Folder, Settings, and About")
	}
	if application.renameButton.Importance != widget.HighImportance {
		t.Fatal("Rename should be the primary action")
	}

	application.files = []media.File{
		{
			Path:     `C:\media\a filename long enough to require truncation.mkv`,
			Status:   media.Ready,
			Proposed: "Matched name.mkv",
		},
		{
			Path:    `C:\media\review.mkv`,
			Status:  media.Review,
			Message: "choose a TMDB match",
		},
		{Path: `C:\media\error.mkv`, Status: media.Error},
		{Path: `C:\media\unsupported.txt`, Status: media.Unsupported},
	}
	application.refresh()
	if application.empty.Visible() || !application.table.Visible() {
		t.Fatal("file table should replace the empty prompt after files are added")
	}

	rows, columns := application.table.Length()
	if rows != 5 || columns != 3 || application.table.ShowHeaderRow || application.table.StickyRowCount != 1 {
		t.Fatalf(
			"compact table = %dx%d, native header %t, sticky rows %d; want 5x3 with a fixed non-resizable first row",
			rows,
			columns,
			application.table.ShowHeaderRow,
			application.table.StickyRowCount,
		)
	}
	cell := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)
	_, label := fileCellParts(cell)
	if label == nil || label.Truncation != fyne.TextTruncateEllipsis ||
		label.SizeName != theme.SizeNameCaptionText {
		t.Fatal("table cells should use compact text with ellipsis")
	}
	application.table.UpdateCell(widget.TableCellID{Row: 2, Col: 1}, cell)
	if label.Text != string(media.Review) {
		t.Fatalf("status cell = %q, want short status only", label.Text)
	}

	statusColors := make(map[media.Status][4]uint32)
	for row, file := range application.files {
		cell := application.table.CreateCell()
		application.table.UpdateCell(widget.TableCellID{Row: row + 1, Col: 0}, cell)
		background, _ := fileCellParts(cell)
		statusColors[file.Status] = rgba(background.FillColor)
	}
	if statusColors[media.Ready] == statusColors[media.Review] ||
		statusColors[media.Review] == statusColors[media.Error] ||
		statusColors[media.Error] == statusColors[media.Unsupported] {
		t.Fatalf("status colors should be semantically distinct: %#v", statusColors)
	}

	app.Settings().SetTheme(theme.LightTheme())
	lightCell := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, lightCell)
	lightBackground, _ := fileCellParts(lightCell)
	light := lightBackground.FillColor
	app.Settings().SetTheme(theme.DarkTheme())
	darkCell := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, darkCell)
	darkBackground, _ := fileCellParts(darkCell)
	dark := darkBackground.FillColor
	if rgba(light) == rgba(dark) || luminance(light) <= luminance(dark) {
		t.Fatal("status tints should adapt to light and dark themes")
	}

	application.window.Show()
	t.Cleanup(application.window.Close)
	application.window.Content().Resize(fyne.NewSize(700, 400))
	canvas.Refresh(application.window.Content())
	application.table.Resize(fyne.NewSize(700, 300))
	application.table.Refresh()
	widths := make(map[string]float32)
	for _, object := range test.LaidOutObjects(application.table) {
		if label, ok := object.(*widget.Label); ok {
			widths[label.Text] = label.Size().Width
		}
	}
	original, status, proposed := widths["Original File"], widths["Status"], widths["Proposed Name"]
	statusLabel := widget.NewLabel(string(media.Unsupported))
	statusLabel.SizeName = theme.SizeNameCaptionText
	if status > 140 || status < statusLabel.MinSize().Width+theme.Padding()*2 ||
		math.Abs(float64(original-proposed)) > 1 || original+status+proposed > 700 {
		t.Fatalf("responsive column widths = %.1f / %.1f / %.1f", original, status, proposed)
	}

	application.table.Select(widget.TableCellID{Row: 2, Col: 0})
	wantDetails := `C:\media\review.mkv — review — choose a TMDB match`
	if findLabelWithText(application.window.Content(), wantDetails) == nil {
		t.Fatalf("selected row details not shown; want %q", wantDetails)
	}
	canvas.Refresh(application.window.Content())
	if application.details.Position().Y+application.details.MinSize().Height > application.status.Position().Y {
		t.Fatal("selected row details should not overlap the global status line")
	}
	selectedCell := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 2, Col: 2}, selectedCell)
	selectedBackground, _ := fileCellParts(selectedCell)
	if got, want := rgba(selectedBackground.FillColor), rgba(theme.ColorForWidget(theme.ColorNameSelection, application.table)); got != want {
		t.Fatalf("selected row color = %#v, want %#v", got, want)
	}
}

func TestMediaDetailsRequireSelectedLocalFile(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "last-rename.json")),
	)
	if application.mediaDetailsButton == nil || !application.mediaDetailsButton.Disabled() {
		t.Fatal("media details should be disabled without a selected local file")
	}

	application.files = []media.File{{Path: "movie.mkv", Status: media.Review}}
	application.selected = 0
	application.selectedRows[0] = true
	application.updateButtons()
	if application.mediaDetailsButton.Disabled() {
		t.Fatal("media details should be enabled for a selected local file")
	}
}

func TestTableColumnsSort(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "last-rename.json")),
	)
	application.files = []media.File{
		{Path: "zeta.mkv", Status: media.Review, Proposed: "beta.mkv"},
		{Path: "Alpha.mkv", Status: media.Ready, Proposed: "zeta.mkv"},
		{Path: "middle.mkv", Status: media.Error, Proposed: "alpha.mkv"},
	}

	application.table.Select(widget.TableCellID{Row: 0, Col: 0})
	if got := []string{
		filepath.Base(application.files[0].Path),
		filepath.Base(application.files[1].Path),
		filepath.Base(application.files[2].Path),
	}; !slices.Equal(got, []string{"Alpha.mkv", "middle.mkv", "zeta.mkv"}) {
		t.Fatalf("original file ascending = %v", got)
	}
	header := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, header)
	_, label := fileCellParts(header)
	if label.Text != "Original File ▲" {
		t.Fatalf("ascending header = %q", label.Text)
	}

	application.table.Select(widget.TableCellID{Row: 0, Col: 0})
	if got := filepath.Base(application.files[0].Path); got != "zeta.mkv" {
		t.Fatalf("original file descending starts with %q", got)
	}
	application.table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, header)
	if label.Text != "Original File ▼" {
		t.Fatalf("descending header = %q", label.Text)
	}

	application.table.Select(widget.TableCellID{Row: 0, Col: 1})
	if got := []media.Status{
		application.files[0].Status,
		application.files[1].Status,
		application.files[2].Status,
	}; !slices.Equal(got, []media.Status{media.Error, media.Ready, media.Review}) {
		t.Fatalf("status ascending = %v", got)
	}

	application.table.Select(widget.TableCellID{Row: 0, Col: 2})
	if got := []string{
		application.files[0].Proposed,
		application.files[1].Proposed,
		application.files[2].Proposed,
	}; !slices.Equal(got, []string{"alpha.mkv", "beta.mkv", "zeta.mkv"}) {
		t.Fatalf("proposed name ascending = %v", got)
	}
}

func TestSortMatchedFilesByStatus(t *testing.T) {
	files := []media.File{
		{Path: "same.mkv", Status: media.Ready, Proposed: "same.mkv"},
		{Path: "ready.mkv", Status: media.Ready, Proposed: "renamed.mkv"},
		{Path: "review.mkv", Status: media.Review},
		{Path: "unmatched.mkv", Status: media.Unmatched},
	}

	sortMatchedFiles(files)
	got := []string{files[0].Path, files[1].Path, files[2].Path, files[3].Path}
	if !slices.Equal(got, []string{"unmatched.mkv", "review.mkv", "ready.mkv", "same.mkv"}) {
		t.Fatalf("matched file order = %v", got)
	}
}

func TestFileSelectionTransitions(t *testing.T) {
	application := &Application{selected: -1, selectionAnchor: -1}

	application.selectRow(1, 0)
	application.selectRow(3, fyne.KeyModifierControl)
	if got := application.selectedIndices(); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("Ctrl-click selection = %v, want [1 3]", got)
	}

	application.selectRow(1, fyne.KeyModifierControl)
	if got := application.selectedIndices(); !slices.Equal(got, []int{3}) {
		t.Fatalf("Ctrl-click deselection = %v, want [3]", got)
	}

	application.selectRow(0, 0)
	application.selectRow(3, fyne.KeyModifierShift)
	if got := application.selectedIndices(); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Fatalf("Shift-click selection = %v, want [0 1 2 3]", got)
	}

	application.selectRow(2, 0)
	if got := application.selectedIndices(); !slices.Equal(got, []int{2}) {
		t.Fatalf("plain-click selection = %v, want [2]", got)
	}
	application.selectRow(0, fyne.KeyModifierControl)
	application.selectRow(3, fyne.KeyModifierShift)
	if got := application.selectedIndices(); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Fatalf("Shift-click after anchor change = %v, want [0 1 2 3]", got)
	}
}

func TestContextActionsFollowSelection(t *testing.T) {
	application := &Application{
		files: []media.File{
			{Path: filepath.Join("media", "old.mkv"), Status: media.Ready, Proposed: "new.mkv", Candidate: media.Candidate{ID: 1}},
			{Path: filepath.Join("media", "review.mkv"), Status: media.Review},
		},
		selected:        0,
		selectionAnchor: 0,
		selectedRows:    map[int]bool{0: true},
	}

	renameItem, metadataItem, reviewItem, removeItem := application.contextActions()
	if renameItem.Disabled || metadataItem.Disabled || reviewItem.Disabled || removeItem.Disabled {
		t.Fatal("single ready row should expose Rename, Write Metadata, Review, and Remove")
	}

	application.selectRow(1, fyne.KeyModifierControl)
	renameItem, metadataItem, reviewItem, removeItem = application.contextActions()
	if renameItem.Disabled {
		t.Fatal("selection containing a ready row should expose Rename")
	}
	if !reviewItem.Disabled {
		t.Fatal("Review should be unavailable for multiple rows")
	}
	if metadataItem.Disabled {
		t.Fatal("Write Metadata should be available for the ready row")
	}
	if removeItem.Disabled {
		t.Fatal("Remove should be available for multiple rows")
	}
	if operations := application.renameOperations(); len(operations) != 1 ||
		operations[0].From != application.files[0].Path {
		t.Fatalf("batch rename operations = %#v, want only selected ready rows", operations)
	}
	if remaining := remainingAfterRename(application.files, application.renameOperations()); len(remaining) != 1 ||
		remaining[0].Path != application.files[1].Path {
		t.Fatalf("remaining rows = %#v, want the unrenamed selection retained", remaining)
	}
}

func TestRemoveSelectedRows(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "rename.json")),
	)
	application.files = []media.File{
		{Path: "first.mkv"},
		{Path: "second.mkv"},
		{Path: "third.mkv"},
	}
	application.selected = 0
	application.selectionAnchor = 0
	application.selectedRows = map[int]bool{0: true, 2: true}

	application.removeSelected()
	if len(application.files) != 1 || application.files[0].Path != "second.mkv" {
		t.Fatalf("remaining files = %#v, want only second.mkv", application.files)
	}
	if len(application.selectedRows) != 0 || application.selected != -1 {
		t.Fatal("Remove should clear the selection")
	}
}

func TestRightClickSelectsRowAndPreservesSelectedRows(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "rename.json")),
	)
	application.files = []media.File{
		{Path: "first.mkv", Status: media.Ready, Proposed: "new-first.mkv"},
		{Path: "second.mkv", Status: media.Ready, Proposed: "new-second.mkv"},
	}
	application.table.Resize(fyne.NewSize(500, 300))
	application.table.CreateRenderer().Layout(application.table.Size())

	rowHeight := application.table.CreateCell().MinSize().Height + theme.Padding()
	application.table.TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(10, rowHeight+1),
		AbsolutePosition: fyne.NewPos(10, rowHeight+1),
	})
	if got := application.selectedIndices(); !slices.Equal(got, []int{0}) {
		t.Fatalf("right-click selection = %v, want [0]", got)
	}

	application.selectRow(1, fyne.KeyModifierControl)
	application.table.TappedSecondary(&fyne.PointEvent{
		Position:         fyne.NewPos(10, rowHeight+1),
		AbsolutePosition: fyne.NewPos(10, rowHeight+1),
	})
	if got := application.selectedIndices(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("right-click selected row changed selection to %v", got)
	}
}

func TestReviewParsed(t *testing.T) {
	parsed, err := reviewParsed("TV episode", "The Last of Us", "2023", "1", "3")
	if err != nil {
		t.Fatal(err)
	}
	want := media.Parsed{Kind: media.Episode, Query: "The Last of Us", Year: 2023, Season: 1, Episode: 3}
	if parsed != want {
		t.Fatalf("reviewParsed() = %#v, want %#v", parsed, want)
	}

	if _, err := reviewParsed("TV episode", "Show", "", "", "2"); err == nil {
		t.Fatal("missing season should fail")
	}
}

func TestReviewCandidatesLimitsTVResults(t *testing.T) {
	candidates := make([]media.Candidate, 12)
	for index := range candidates {
		candidates[index].Kind = media.Episode
	}
	if got := reviewCandidates(candidates); len(got) != 10 {
		t.Fatalf("TV candidates = %d, want 10", len(got))
	}

	for index := range candidates {
		candidates[index].Kind = media.Movie
	}
	if got := reviewCandidates(candidates); len(got) != 12 {
		t.Fatalf("movie candidates = %d, want unchanged", len(got))
	}
}

func TestManualReviewSearchMovieAndEpisode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/search/movie":
			if request.URL.Query().Get("query") != "Dune" || request.URL.Query().Get("year") != "2024" {
				t.Fatalf("movie query = %q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"results":[{"id":1,"title":"Dune","release_date":"2024-01-01"}]}`))
		case "/search/tv":
			if request.URL.Query().Get("query") != "The Last of Us" ||
				request.URL.Query().Get("first_air_date_year") != "2023" {
				t.Fatalf("TV query = %q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"results":[{"id":2,"name":"The Last of Us","first_air_date":"2023-01-15"}]}`))
		case "/movie/99":
			_, _ = writer.Write([]byte(`{"id":99,"title":"Exact Movie","release_date":"2020-01-01"}`))
		case "/tv/88":
			_, _ = writer.Write([]byte(`{"id":88,"name":"Exact Show","first_air_date":"2021-01-01"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := settings.Defaults()
	engine := matcher.New(tmdb.NewWithHTTPClient("token", server.URL, server.Client()))
	movie, id, err := manualSearchParsed("Dune", "2024", "Movie", "", media.Parsed{})
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Fatalf("search ID = %d, want 0", id)
	}
	movies, err := engine.Search(context.Background(), movie, options)
	if err != nil || len(movies) != 1 || movies[0].Kind != media.Movie {
		t.Fatalf("movie search = %#v, %v", movies, err)
	}

	file := media.File{
		Path: "The.Last.of.Us.S01E03.mkv", Status: media.Unmatched,
		Parsed: media.Parsed{Kind: media.Episode, Season: 1, Episode: 3},
	}
	episode, id, err := manualSearchParsed("The Last of Us", "2023", "TV series", "", file.Parsed)
	if err != nil {
		t.Fatal(err)
	}
	shows, err := engine.Search(context.Background(), episode, options)
	if err != nil || len(shows) != 1 || shows[0].Season != 1 || shows[0].Episode != 3 {
		t.Fatalf("TV search = %#v, %v", shows, err)
	}
	if file.Status != media.Unmatched || file.Candidate.ID != 0 || file.Proposed != "" {
		t.Fatalf("search without selection changed file: %#v", file)
	}

	exact, id, err := manualSearchParsed("", "ignored", "TV series", "88", file.Parsed)
	if err != nil || id != 88 {
		t.Fatalf("exact request = %#v, %d, %v", exact, id, err)
	}
	candidate, err := engine.Lookup(context.Background(), exact, id, options)
	if err != nil || candidate.ID != 88 || candidate.Season != 1 || candidate.Episode != 3 {
		t.Fatalf("exact TV lookup = %#v, %v", candidate, err)
	}
	exact, id, err = manualSearchParsed("", "", "Movie", "99", media.Parsed{})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = engine.Lookup(context.Background(), exact, id, options)
	if err != nil || candidate.ID != 99 || candidate.Kind != media.Movie {
		t.Fatalf("exact movie lookup = %#v, %v", candidate, err)
	}
}

func TestManualReviewSearchValidationDoesNotChangeFile(t *testing.T) {
	file := media.File{Path: "unchanged.mkv", Status: media.Unmatched}
	before := file
	if _, _, err := manualSearchParsed("", "bad", "Movie", "", file.Parsed); err == nil {
		t.Fatal("empty title should fail validation")
	}
	if file.Path != before.Path || file.Status != before.Status {
		t.Fatalf("validation changed file from %#v to %#v", before, file)
	}
}

func TestExpectedEpisodeImportPairsAndDeduplicates(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	application.files = []media.File{{
		Path:   `C:\media\Show.S01E01.MKV`,
		Parsed: media.Parsed{Kind: media.Episode, Query: "Show", Season: 1, Episode: 1},
	}}
	show := tmdb.Show{ID: 42, Name: "Show", FirstAirDate: "2024-01-01"}

	added, err := application.importEpisodes(show, []tmdb.Episode{
		{ID: 101, Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 102, Name: "Second", SeasonNumber: 1, EpisodeNumber: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || len(application.files) != 2 {
		t.Fatalf("added=%d files=%#v", added, application.files)
	}
	paired := application.files[0]
	if !paired.IsEpisodePairing() || paired.Status != media.Ready || paired.Proposed != "Show - S01E01 - Pilot.MKV" {
		t.Fatalf("paired row = %#v", paired)
	}
	expected := application.files[1]
	if !expected.IsExpectedEpisode() || expected.Proposed != "Show - S01E02 - Second" {
		t.Fatalf("expected row = %#v", expected)
	}

	added, err = application.importEpisodes(show, []tmdb.Episode{
		{ID: 101, Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 102, Name: "Second", SeasonNumber: 1, EpisodeNumber: 2},
	})
	if err != nil || added != 0 || len(application.files) != 2 {
		t.Fatalf("duplicate import added=%d err=%v files=%#v", added, err, application.files)
	}
}

func TestAutomaticPairingRefusesAmbiguousExpectedEpisodes(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	episode := []tmdb.Episode{{Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1}}
	if _, err := application.importEpisodes(tmdb.Show{ID: 1, Name: "First"}, episode); err != nil {
		t.Fatal(err)
	}
	if _, err := application.importEpisodes(tmdb.Show{ID: 2, Name: "Second"}, episode); err != nil {
		t.Fatal(err)
	}
	application.files = append(application.files, media.File{
		Path: `C:\media\Unknown.S01E01.mkv`,
		Parsed: media.Parsed{
			Kind: media.Episode, Season: 1, Episode: 1,
		},
	})

	application.reconcileExpectedEpisodes()

	if application.files[2].Imported || application.files[2].Status == media.Ready {
		t.Fatalf("ambiguous file should remain unpaired: %#v", application.files)
	}
}

func TestAutomaticPairingHandlesMultipleFilesAddedAfterImport(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	if _, err := application.importEpisodes(tmdb.Show{ID: 1, Name: "Show"}, []tmdb.Episode{
		{Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
		{Name: "Second", SeasonNumber: 1, EpisodeNumber: 2},
	}); err != nil {
		t.Fatal(err)
	}
	application.files = append(application.files,
		media.File{Path: `C:\media\Show.S01E01.mkv`, Parsed: media.Parsed{Kind: media.Episode, Season: 1, Episode: 1}},
		media.File{Path: `C:\media\Show.S01E02.mkv`, Parsed: media.Parsed{Kind: media.Episode, Season: 1, Episode: 2}},
	)

	application.reconcileExpectedEpisodes()

	paired := 0
	for _, file := range application.files {
		if file.IsEpisodePairing() {
			paired++
		}
	}
	if paired != 2 {
		t.Fatalf("paired %d rows: %#v", paired, application.files)
	}
}

func TestAutomaticPairingRefusesAmbiguousLocalFiles(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	if _, err := application.importEpisodes(tmdb.Show{ID: 1, Name: "Show"}, []tmdb.Episode{
		{Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
	}); err != nil {
		t.Fatal(err)
	}
	parsed := media.Parsed{Kind: media.Episode, Season: 1, Episode: 1}
	application.files = append(application.files,
		media.File{Path: `C:\media\first.mkv`, Parsed: parsed},
		media.File{Path: `C:\media\second.mkv`, Parsed: parsed},
	)

	application.reconcileExpectedEpisodes()

	for _, file := range application.files {
		if file.IsEpisodePairing() {
			t.Fatalf("ambiguous local files should remain unpaired: %#v", application.files)
		}
	}
}

func TestManualExpectedPairingCanChangeAndRemove(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	episode := []tmdb.Episode{{Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1}}
	if _, err := application.importEpisodes(tmdb.Show{ID: 1, Name: "First"}, episode); err != nil {
		t.Fatal(err)
	}
	if _, err := application.importEpisodes(tmdb.Show{ID: 2, Name: "Second"}, episode); err != nil {
		t.Fatal(err)
	}
	application.files = append(application.files, media.File{
		Path:   `C:\media\Unknown.S01E01.mkv`,
		Parsed: media.Parsed{Kind: media.Episode, Season: 1, Episode: 1},
	})

	if err := application.pairExpected(2, 0); err != nil {
		t.Fatal(err)
	}
	paired, other := expectedRows(application.files)
	if paired < 0 || other < 0 || application.files[paired].Candidate.ID != 1 {
		t.Fatalf("first manual pairing = %#v", application.files)
	}
	if err := application.pairExpected(paired, other); err != nil {
		t.Fatal(err)
	}
	paired, other = expectedRows(application.files)
	if paired < 0 || other < 0 || application.files[paired].Candidate.ID != 2 ||
		application.files[other].Candidate.ID != 1 {
		t.Fatalf("changed manual pairing = %#v", application.files)
	}
	if err := application.unpairExpected(paired); err != nil {
		t.Fatal(err)
	}
	expectedCount, localCount := 0, 0
	for _, file := range application.files {
		if file.IsExpectedEpisode() {
			expectedCount++
		}
		if !file.Imported && file.Path != "" {
			localCount++
		}
	}
	if expectedCount != 2 || localCount != 1 {
		t.Fatalf("removed manual pairing = %#v", application.files)
	}
}

func TestRenameIgnoresAndRetainsUnpairedExpectedEpisodes(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "rename.json")),
	)
	application.files = []media.File{
		{
			Path: `C:\media\Show.S01E01.mkv`, Imported: true, Status: media.Ready,
			Proposed: "Show - S01E01 - Pilot.mkv",
		},
		{Imported: true, Status: media.Expected, Proposed: "Show - S01E02 - Second"},
	}

	if !application.canRename() {
		t.Fatal("unpaired expected episode should not block a ready rename")
	}
	operations := application.renameOperations()
	if len(operations) != 1 || operations[0].From == "" {
		t.Fatalf("rename operations = %#v", operations)
	}
	remaining := remainingAfterRename(application.files, operations)
	if len(remaining) != 1 || remaining[0].Path != "" || remaining[0].Status != media.Expected {
		t.Fatalf("remaining rows = %#v", remaining)
	}
}

func TestEmbeddedMetadataUsesMovieAndEpisodeFields(t *testing.T) {
	fields := metadata.AllWriteFields()
	movie := embeddedMetadata(media.File{Candidate: media.Candidate{
		ID: 1, Kind: media.Movie, Title: "Localized", OriginalTitle: "Original",
		ReleaseDate: "2024-02-03", Overview: "Movie overview",
		Genre: "Action", LawRating: "PG-13", Directors: []string{"Dir"}, Actors: []string{"Star"},
	}}, fields)
	if movie.Title != "Localized" || movie.OriginalTitle != "Original" ||
		movie.Date != "2024-02-03" || movie.TMDBID != 1 || movie.Overview != "Movie overview" ||
		movie.Genre != "Action" || movie.LawRating != "PG-13" ||
		len(movie.Directors) != 1 || movie.Directors[0] != "Dir" {
		t.Fatalf("movie metadata = %#v", movie)
	}

	episode := embeddedMetadata(media.File{Candidate: media.Candidate{
		ID: 2, EpisodeTMDBID: 24, Kind: media.Episode, Title: "Series", EpisodeTitle: "Localized episode",
		OriginalEpisodeTitle: "Original episode", AirDate: "2024-04-05",
		Season: 3, Episode: 4, Overview: "Episode overview", Genre: "Drama",
	}}, fields)
	if episode.Title != "Localized episode" || episode.OriginalTitle != "Original episode" ||
		episode.Date != "2024-04-05" || episode.Series != "Series" ||
		episode.Season != 3 || episode.Episode != 4 || episode.TMDBID != 24 ||
		episode.Genre != "Drama" || !episode.IsEpisode {
		t.Fatalf("episode metadata = %#v", episode)
	}

	titleOnly := fields
	titleOnly.OriginalTitle = false
	titleOnly.Comment = false
	titleOnly.DateReleased = false
	titleOnly.Genre = false
	titleOnly.LawRating = false
	titleOnly.Directors = false
	titleOnly.Writers = false
	titleOnly.Actors = false
	titleOnly.TMDBID = false
	titleOnly.SeriesInfo = false
	filtered := embeddedMetadata(media.File{Candidate: media.Candidate{
		ID: 1, Kind: media.Movie, Title: "Localized", OriginalTitle: "Original",
		ReleaseDate: "2024-02-03", Overview: "Movie overview", Genre: "Action",
	}}, titleOnly)
	if filtered.Title != "Localized" || filtered.Overview != "" || filtered.Genre != "" ||
		filtered.Date != "" || filtered.TMDBID != 0 {
		t.Fatalf("title-only metadata = %#v", filtered)
	}
}

func TestSameNameFileCanWritePendingMetadata(t *testing.T) {
	application := &Application{
		files: []media.File{{
			Path: "same.mkv", Proposed: "same.mkv", Status: media.Ready,
			Candidate:       media.Candidate{ID: 1, Kind: media.Movie, Title: "Movie"},
			MetadataPending: true,
		}},
		selectedRows: map[int]bool{0: true},
	}
	operations := application.metadataOperations()
	if len(operations) != 1 || operations[0].From != operations[0].To {
		t.Fatalf("metadata operations = %#v", operations)
	}
	if rowStatus(application.files[0]) != media.Metadata {
		t.Fatalf("row status = %q", rowStatus(application.files[0]))
	}
	application.files[0].MetadataPending = false
	if rowStatus(application.files[0]) != media.SameName {
		t.Fatalf("written same-name row status = %q", rowStatus(application.files[0]))
	}
}

func TestUnsupportedMetadataResultOnlyAffectsSelectionAndCanBeCleared(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	options.WriteEmbeddedMetadata = true
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	application.files = []media.File{
		{Path: "selected.avi", Status: media.Ready, Proposed: "selected-new.avi"},
		{Path: "other.avi", Status: media.Ready, Proposed: "other-new.avi"},
	}
	application.selectedRows = map[int]bool{0: true}
	application.markUnsupportedMetadataRows()
	if application.files[0].Status != media.Unsupported || application.files[1].Status != media.Ready {
		t.Fatalf("statuses = %q, %q", application.files[0].Status, application.files[1].Status)
	}

	options.WriteEmbeddedMetadata = false
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	if err := application.refreshProposedNames(); err != nil {
		t.Fatal(err)
	}
	if application.files[0].Status != media.Ready {
		t.Fatal("disabling metadata should restore filename-only rename eligibility")
	}
}

func TestUnchangedFileIsNeutralAndSkipped(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	application := New(
		app,
		settings.NewStore(app.Preferences()),
		rename.NewManager(filepath.Join(t.TempDir(), "rename.json")),
	)
	application.files = []media.File{
		{Path: filepath.Join("media", "old.mkv"), Status: media.Ready, Proposed: "new.mkv"},
		{Path: filepath.Join("media", "same.mkv"), Status: media.Ready, Proposed: "same.mkv"},
	}

	cell := application.table.CreateCell()
	application.table.UpdateCell(widget.TableCellID{Row: 2, Col: 0}, cell)
	background, _ := fileCellParts(cell)
	neutral := statusRowColor(
		media.SameName,
		theme.ColorForWidget(theme.ColorNameBackground, application.table),
	)
	if rgba(background.FillColor) != rgba(neutral) {
		t.Fatal("unchanged file should use the neutral row color")
	}
	if operations := application.renameOperations(); len(operations) != 1 || operations[0].From != application.files[0].Path {
		t.Fatalf("rename operations = %#v, want only the changed file", operations)
	}
	if got := matchSummary(application.files); got != "Matching complete: 1 ready, 0 need review, 0 unmatched, 0 errors." {
		t.Fatalf("match summary = %q", got)
	}
	application.table.Select(widget.TableCellID{Row: 0, Col: 1})
	application.table.Select(widget.TableCellID{Row: 0, Col: 1})
	if !unchanged(application.files[0]) {
		t.Fatal("descending status sort should put neutral rows before ready rows")
	}
	application.files = application.files[:1]
	if application.canRename() {
		t.Fatal("unchanged file should not enable Rename")
	}
}

func TestNamingPatternRefreshesExpectedAndPairedRows(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := settings.NewStore(app.Preferences())
	options := settings.Defaults()
	options.TMDBToken = "token"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}
	application := New(app, store, rename.NewManager(filepath.Join(t.TempDir(), "rename.json")))
	application.files = []media.File{{
		Path:   `C:\media\Show.S01E01.mp4`,
		Parsed: media.Parsed{Kind: media.Episode, Season: 1, Episode: 1},
	}}
	show := tmdb.Show{ID: 42, Name: "Show"}
	if _, err := application.importEpisodes(show, []tmdb.Episode{
		{Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
		{Name: "Second", SeasonNumber: 1, EpisodeNumber: 2},
	}); err != nil {
		t.Fatal(err)
	}
	options.EpisodePattern = "{series}.S{season}E{episode}.{episode_title}"
	if err := store.Save(options); err != nil {
		t.Fatal(err)
	}

	if err := application.refreshProposedNames(); err != nil {
		t.Fatal(err)
	}

	if application.files[0].Proposed != "Show.S01E01.Pilot.mp4" ||
		application.files[1].Proposed != "Show.S01E02.Second" {
		t.Fatalf("refreshed names = %#v", application.files)
	}
}

func TestAllSeasonImportFiltersSpecialsAndFailsAtomically(t *testing.T) {
	seasons := []tmdb.Season{
		{Name: "Specials", SeasonNumber: 0},
		{Name: "Season 1", SeasonNumber: 1},
	}
	if got := selectedSeasonNumbers(seasons, true); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("with specials = %v", got)
	}
	if got := selectedSeasonNumbers(seasons, false); !slices.Equal(got, []int{1}) {
		t.Fatalf("without specials = %v", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/tv/42/season/0" {
			_, _ = writer.Write([]byte(`{"episodes":[{"name":"Special","season_number":0,"episode_number":1}]}`))
			return
		}
		http.Error(writer, `{"status_message":"season unavailable"}`, http.StatusBadRequest)
	}))
	defer server.Close()
	client := tmdb.NewWithHTTPClient("token", server.URL, server.Client())

	episodes, err := loadShowEpisodes(context.Background(), client, 42, []int{0, 1}, "en-US")
	if err == nil || episodes != nil {
		t.Fatalf("partial import should return no episodes: episodes=%#v err=%v", episodes, err)
	}
}

func TestNamingReferenceTablesCoverCompatibleSyntax(t *testing.T) {
	movie := namingReferenceRows(settings.NamingAdvanced, media.Movie)
	episode := namingReferenceRows(settings.NamingAdvanced, media.Episode)

	for _, syntax := range []string{"{n}", "{ny}", "{y}", "{primaryTitle}", "{tmdbid}"} {
		if !hasNamingSyntax(movie, syntax) {
			t.Errorf("movie reference is missing %s", syntax)
		}
	}
	for _, syntax := range []string{"{n}", "{ny}", "{y}", "{s}", "{e}", "{sxe}", "{s00e00}", "{t}", "{primaryTitle}", "{tmdbid}"} {
		if !hasNamingSyntax(episode, syntax) {
			t.Errorf("episode reference is missing %s", syntax)
		}
	}
	for _, method := range media.AdvancedTemplateMethods() {
		for kind, rows := range map[string][]namingSyntaxRow{"movie": movie, "episode": episode} {
			if !slices.ContainsFunc(rows, func(row namingSyntaxRow) bool {
				return strings.Contains(row.Syntax, "."+method+"(")
			}) {
				t.Errorf("%s reference is missing %s", kind, method)
			}
		}
	}
	for _, syntax := range []string{
		`{" ($y)"}`,
		`$y or ${y}`,
		`{n.space('.').lower()}`,
		`{y ? " ($y)" : ""}`,
		`!  &&  ||  ==  !=`,
		`'text' or "text"`,
		`/pattern/`,
		`3`,
	} {
		if !hasNamingSyntax(movie, syntax) || !hasNamingSyntax(episode, syntax) {
			t.Errorf("advanced references are missing %s", syntax)
		}
	}

	if !hasNamingSyntax(namingReferenceRows(settings.NamingSimple, media.Movie), "{title}") {
		t.Error("simple movie reference is missing {title}")
	}
	if !hasNamingSyntax(namingReferenceRows(settings.NamingSimple, media.Episode), "{series}") {
		t.Error("simple episode reference is missing {series}")
	}

	table := newNamingReferenceTable(func() []namingSyntaxRow { return movie })
	rows, columns := table.Length()
	if rows != len(movie) || columns != 4 || !table.ShowHeaderRow || table.StickyRowCount != 0 {
		t.Fatalf("reference table = %dx%d, header %t, sticky rows %d", rows, columns, table.ShowHeaderRow, table.StickyRowCount)
	}
	header := table.CreateHeader()
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 0}, header)
	if header.(*widget.Label).Text != "Syntax" {
		t.Fatalf("first header = %q", header.(*widget.Label).Text)
	}
}

func TestAdvancedTemplateEntryAcceptsAndDismissesCompletions(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("Advanced template")
	entry := newAdvancedTemplateEntry(media.Movie, func() bool { return true })
	window.SetContent(entry)
	window.Show()
	t.Cleanup(window.Close)

	test.Type(entry, "{n.")
	if entry.Text != "{n." || len(entry.completions) != len(media.AdvancedTemplateMethods()) {
		t.Fatalf("typing changed text or missed completions: text=%q completions=%d", entry.Text, len(entry.completions))
	}
	if entry.popup == nil || !entry.popup.Visible() ||
		!strings.Contains(entry.completionButtons[0].Text, "String") ||
		!strings.Contains(entry.completionButtons[0].Text, "{n.acronym()}") {
		t.Fatal("completion popup should show return type and compact example")
	}

	for range 8 {
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	}
	if entry.completionScroll == nil || entry.completionScroll.Offset.Y == 0 {
		t.Fatal("keyboard navigation should scroll the selected completion into view")
	}
	for range 8 {
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	}
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	if entry.Text != "{n.after()" || entry.CursorTextOffset() != len([]rune("{n.after(")) {
		t.Fatalf("accepted completion = %q at %d", entry.Text, entry.CursorTextOffset())
	}
	if entry.signature == nil || entry.signature.Name != "after" || entry.signature.ActiveParameter != 0 {
		t.Fatalf("signature after completion = %#v", entry.signature)
	}

	entry.SetText("{n.")
	entry.CursorColumn = len([]rune(entry.Text))
	entry.refreshAssist()
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if entry.Text != "{n." || entry.popup != nil || len(entry.completions) != 0 {
		t.Fatalf("escape changed text or kept popup: text=%q completions=%d", entry.Text, len(entry.completions))
	}

	entry.SetText("prefix {pr} suffix")
	entry.CursorColumn = 10
	entry.refreshAssist()
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	if entry.Text != "prefix {primaryTitle} suffix" {
		t.Fatalf("completion replaced unrelated text: %q", entry.Text)
	}
}

func TestAdvancedTemplateEntryAcceptsFocusedCanvasTyping(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("Advanced template")
	entry := newAdvancedTemplateEntry(media.Movie, func() bool { return true })
	window.SetContent(entry)
	window.Show()
	t.Cleanup(window.Close)

	mouse := &desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(1, 1)},
		Button:     desktop.MouseButtonPrimary,
	}
	entry.MouseDown(mouse)
	entry.MouseUp(mouse)
	if window.Canvas().Focused() != entry {
		t.Fatal("clicking the template entry did not focus it")
	}
	window.Canvas().Focused().TypedRune('{')
	if window.Canvas().Focused() == nil {
		t.Fatal("opening completions stole focus from the template entry")
	}
	window.Canvas().Focused().TypedRune('n')
	window.Canvas().Focused().TypedRune('.')

	if entry.Text != "{n." {
		t.Fatalf("focused canvas typing = %q, want %q", entry.Text, "{n.")
	}
}

func TestAdvancedTemplateEntryMouseCompletionPlacesCursor(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("Advanced template")
	entry := newAdvancedTemplateEntry(media.Movie, func() bool { return true })
	window.SetContent(entry)
	window.Show()
	t.Cleanup(window.Close)

	var cursorMoves []int
	entry.Entry.OnCursorChanged = func() {
		cursorMoves = append(cursorMoves, entry.CursorTextOffset())
		entry.cursorChanged()
	}
	test.Type(entry, "{n.acr")
	cursorMoves = nil
	test.Tap(entry.completionButtons[0])
	if entry.Text != "{n.acronym()" || entry.CursorTextOffset() != len([]rune("{n.acronym()")) {
		t.Fatalf("no-argument completion = %q at %d", entry.Text, entry.CursorTextOffset())
	}
	if len(cursorMoves) == 0 || cursorMoves[len(cursorMoves)-1] != entry.CursorTextOffset() {
		t.Fatalf("rendered cursor did not move to accepted no-argument completion: %v", cursorMoves)
	}

	entry.SetText("{n.aft")
	entry.CursorColumn = len([]rune(entry.Text))
	entry.refreshAssist()
	cursorMoves = nil
	test.Tap(entry.completionButtons[0])
	if entry.Text != "{n.after()" || entry.CursorTextOffset() != len([]rune("{n.after(")) {
		t.Fatalf("argument completion = %q at %d", entry.Text, entry.CursorTextOffset())
	}
	if len(cursorMoves) == 0 || cursorMoves[len(cursorMoves)-1] != entry.CursorTextOffset() {
		t.Fatalf("rendered cursor did not move inside accepted argument completion: %v", cursorMoves)
	}

	entry.SetText(`{" ($y.`)
	entry.CursorColumn = len([]rune(entry.Text))
	entry.refreshAssist()
	replace := slices.IndexFunc(entry.completions, func(completion media.AdvancedTemplateCompletion) bool {
		return completion.Name == "replace"
	})
	if replace < 0 {
		t.Fatal("replace completion missing from integer interpolation")
	}
	test.Tap(entry.completionButtons[replace])
	if entry.Text != `{" ($y.replace()` || entry.signature == nil || entry.signature.Name != "replace" {
		t.Fatalf("interpolation completion = %q with signature %#v", entry.Text, entry.signature)
	}
}

func TestAdvancedTemplateEntryFiltersAndTracksSignatureParameters(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("Advanced template")
	entry := newAdvancedTemplateEntry(media.Movie, func() bool { return true })
	window.SetContent(entry)
	window.Show()
	t.Cleanup(window.Close)

	test.Type(entry, "{n.repl")
	names := make([]string, len(entry.completions))
	for index, completion := range entry.completions {
		names[index] = completion.Name
	}
	if !slices.Equal(names, []string{"replace", "replaceAll"}) {
		t.Fatalf("filtered completions = %v", names)
	}

	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if entry.Text != "{n.replace()" || entry.CursorTextOffset() != len([]rune("{n.replace(")) {
		t.Fatalf("tab completion = %q at %d", entry.Text, entry.CursorTextOffset())
	}
	test.Type(entry, "'old', ")
	if entry.signature == nil || entry.signature.Name != "replace" || entry.signature.ActiveParameter != 1 {
		t.Fatalf("second parameter signature = %#v", entry.signature)
	}
	if len(entry.completions) != 0 {
		t.Fatalf("literal should not offer completions: %#v", entry.completions)
	}
	if entry.popup == nil || !strings.Contains(entry.signatureLabel.Text, "new") ||
		!strings.Contains(entry.signatureLabel.Text, "required") {
		t.Fatal("signature popup should highlight the required new parameter")
	}
	beforeEscape := entry.Text
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if entry.Text != beforeEscape || entry.popup != nil || entry.signature != nil {
		t.Fatal("escape should dismiss signature help without changing text")
	}
}

func hasNamingSyntax(rows []namingSyntaxRow, syntax string) bool {
	return slices.ContainsFunc(rows, func(row namingSyntaxRow) bool {
		return row.Syntax == syntax && row.Description != "" && row.Example != "" && row.Type != ""
	})
}

func expectedRows(files []media.File) (paired, unpaired int) {
	paired, unpaired = -1, -1
	for index, file := range files {
		if file.IsEpisodePairing() {
			paired = index
		}
		if file.IsExpectedEpisode() {
			unpaired = index
		}
	}
	return paired, unpaired
}

func fileCellParts(object fyne.CanvasObject) (*canvas.Rectangle, *widget.Label) {
	objects := object.(*fyne.Container).Objects
	return objects[0].(*canvas.Rectangle), objects[1].(*widget.Label)
}

func findLabelWithText(object fyne.CanvasObject, text string) *widget.Label {
	if label, ok := object.(*widget.Label); ok && label.Text == text {
		return label
	}
	container, ok := object.(*fyne.Container)
	if !ok {
		return nil
	}
	for _, child := range container.Objects {
		if label := findLabelWithText(child, text); label != nil {
			return label
		}
	}
	return nil
}

func rgba(value color.Color) [4]uint32 {
	red, green, blue, alpha := value.RGBA()
	return [4]uint32{red, green, blue, alpha}
}

func luminance(value color.Color) uint32 {
	red, green, blue, _ := value.RGBA()
	return (red*299 + green*587 + blue*114) / 1000
}

func menuContains(menu *fyne.MainMenu, label string) bool {
	if menu == nil {
		return false
	}
	for _, section := range menu.Items {
		for _, item := range section.Items {
			if item.Label == label {
				return true
			}
		}
	}
	return false
}
