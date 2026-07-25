package ui

import (
	_ "embed"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

//go:embed assets/tmdb.svg
var tmdbLogo []byte

func ShowAbout(parent fyne.Window) {
	logo := canvas.NewImageFromResource(fyne.NewStaticResource("tmdb.svg", tmdbLogo))
	logo.SetMinSize(fyne.NewSize(150, 90))
	logo.FillMode = canvas.ImageFillContain

	tmdbURL, _ := url.Parse("https://www.themoviedb.org")
	notice := widget.NewLabel("This product uses the TMDB API but is not endorsed or certified by TMDB.")
	notice.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		widget.NewLabelWithStyle("FileGot 0.1.0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("A safe movie and TV episode renamer."),
		logo,
		notice,
		widget.NewHyperlink("The Movie Database", tmdbURL),
	)
	dialog.NewCustom("About FileGot", "Close", content, parent).Show()
}
