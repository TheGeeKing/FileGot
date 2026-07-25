//go:build windows

package media

import (
	"io/fs"
	"strings"
	"syscall"
)

func isHidden(path string, entry fs.DirEntry) bool {
	if strings.HasPrefix(entry.Name(), ".") {
		return true
	}
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attributes, err := syscall.GetFileAttributes(pointer)
	return err == nil && attributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
