//go:build !windows

package media

import (
	"io/fs"
	"strings"
)

func isHidden(_ string, entry fs.DirEntry) bool {
	return strings.HasPrefix(entry.Name(), ".")
}
