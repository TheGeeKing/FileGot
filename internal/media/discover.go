package media

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var videoExtensions = map[string]struct{}{
	".mkv": {}, ".mp4": {}, ".m4v": {}, ".avi": {}, ".mov": {}, ".wmv": {},
	".ts": {}, ".m2ts": {}, ".webm": {}, ".mpg": {}, ".mpeg": {},
}

type DiscoverOptions struct {
	Recursive    bool
	IgnoreHidden bool
}

func Discover(paths []string, options DiscoverOptions) ([]File, error) {
	seen := make(map[string]struct{})
	var files []File

	add := func(path string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		absolute = filepath.Clean(absolute)
		key := absolute
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			return nil
		}
		if !IsVideo(absolute) {
			return nil
		}
		seen[key] = struct{}{}
		parsed := Parse(absolute)
		status := Unmatched
		message := ""
		if parsed.MultiEpisode {
			status = Unsupported
			message = "multi-episode files are not supported yet"
		}
		files = append(files, File{Path: absolute, Parsed: parsed, Status: status, Message: message})
		return nil
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if !info.IsDir() {
			if err := add(path); err != nil {
				return nil, err
			}
			continue
		}

		root := filepath.Clean(path)
		err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current != root && entry.IsDir() && !options.Recursive {
				return filepath.SkipDir
			}
			if current != root && options.IgnoreHidden && isHidden(current, entry) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			return add(current)
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", root, err)
		}
	}

	return files, nil
}

func IsVideo(path string) bool {
	_, ok := videoExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}
