package ui

import (
	"image/color"
	"math"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thegeeking/FileGot/internal/media"
	"github.com/thegeeking/FileGot/internal/rename"
	"github.com/thegeeking/FileGot/internal/settings"
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
