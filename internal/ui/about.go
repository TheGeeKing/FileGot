package ui

import (
	_ "embed"
	"fmt"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

//go:embed assets/tmdb.svg
var tmdbLogo []byte

func ShowAbout(a fyne.App, parent fyne.Window) {
	logo := canvas.NewImageFromResource(fyne.NewStaticResource("tmdb.svg", tmdbLogo))
	logo.SetMinSize(fyne.NewSize(150, 90))
	logo.FillMode = canvas.ImageFillContain

	tmdbURL, _ := url.Parse("https://www.themoviedb.org")
	notice := widget.NewLabel("This product uses the TMDB API but is not endorsed or certified by TMDB.")
	notice.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		widget.NewLabelWithStyle(aboutTitle(a), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("A safe movie and TV episode renamer."),
		logo,
		notice,
		widget.NewHyperlink("The Movie Database", tmdbURL),
	)
	dialog.NewCustom("About FileGot", "Close", content, parent).Show()
}

func aboutTitle(a fyne.App) string {
	meta := a.Metadata()
	return formatAboutTitle(meta.Name, meta.Version)
}

func formatAboutTitle(name, version string) string {
	if name == "" {
		name = "FileGot"
	}
	if version == "" {
		version = "dev"
	}
	return fmt.Sprintf("%s %s", name, version)
}
