//go:build !windows

package files

import "io/fs"

// IsHiddenOrSystem is a no-op on non-Windows hosts.
func IsHiddenOrSystem(fs.FileInfo) bool { return false }
