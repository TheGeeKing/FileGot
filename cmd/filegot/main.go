package main

import (
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"

	"github.com/thegeeking/FileGot/internal/rename"
	"github.com/thegeeking/FileGot/internal/settings"
	"github.com/thegeeking/FileGot/internal/ui"
)

const appID = "com.github.thegeeking.filegot"

func main() {
	a := app.NewWithID(appID)

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("resolve configuration directory: %v", err)
		configDir = "."
	}

	store := settings.NewStore(a.Preferences())
	renamer := rename.NewManager(filepath.Join(configDir, "FileGot", "last-rename.json"))
	ui.New(a, store, renamer).Run()
}
