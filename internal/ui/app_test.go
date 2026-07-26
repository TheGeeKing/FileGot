package ui

import (
	"context"
	"image/color"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/media"
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
	if !menuContains(application.window.MainMenu(), "Settings") ||
		!menuContains(application.window.MainMenu(), "About") {
		t.Fatal("main menu should contain Settings and About")
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
	remaining := remainingAfterRename(application.files)
	if len(remaining) != 1 || remaining[0].Path != "" || remaining[0].Status != media.Expected {
		t.Fatalf("remaining rows = %#v", remaining)
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
