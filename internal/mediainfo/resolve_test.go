package mediainfo

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestExecutablePrecedence(t *testing.T) {
	root := t.TempDir()
	bundled := filepath.Join(root, "tools", executableName())
	lookPath := func(name string) (string, error) {
		switch name {
		case `C:\Custom\MediaInfo.exe`:
			return `C:\Custom\MediaInfo.exe`, nil
		case "mediainfo":
			return `C:\PATH\mediainfo.exe`, nil
		default:
			return "", errors.New("not found")
		}
	}

	if got, err := resolveExecutable(`C:\Custom\MediaInfo.exe`, root, lookPath, func(string) bool { return false }); err != nil || got != `C:\Custom\MediaInfo.exe` {
		t.Fatalf("configured executable = %q, %v", got, err)
	}
	if got, err := resolveExecutable("", root, lookPath, func(string) bool { return true }); err != nil || got != `C:\PATH\mediainfo.exe` {
		t.Fatalf("PATH executable = %q, %v", got, err)
	}
	if got, err := resolveExecutable("", root, func(string) (string, error) { return "", errors.New("not found") }, func(path string) bool { return path == bundled }); err != nil || got != bundled {
		t.Fatalf("bundled executable = %q, %v", got, err)
	}
}
