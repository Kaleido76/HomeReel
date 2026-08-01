//go:build !windows

package files

import (
	"os"
	"syscall"
)

// FileID returns the inode of path (ADR-007 fingerprint identity).
func FileID(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Sys().(*syscall.Stat_t).Ino, nil
}
