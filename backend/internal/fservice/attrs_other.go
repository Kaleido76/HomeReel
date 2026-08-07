//go:build !windows

package fservice

import "io/fs"

// isHiddenOrSystem is a no-op on non-Windows hosts.
func isHiddenOrSystem(fs.FileInfo) bool { return false }
