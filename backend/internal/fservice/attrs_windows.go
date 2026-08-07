//go:build windows

package fservice

import (
	"io/fs"
	"syscall"
)

// isHiddenOrSystem reports whether the entry carries the Windows HIDDEN or
// SYSTEM attribute. This is the attribute-based way to exclude special
// directories such as $RECYCLE.BIN / System Volume Information and helper files
// like desktop.ini — deliberately not a name-based filter. The attribute is
// read via Lstat (FILE_FLAG_OPEN_REPARSE_POINT) so junctions' own attributes
// are considered, not their targets'.
func isHiddenOrSystem(info fs.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return data.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
}
