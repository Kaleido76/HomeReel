//go:build windows

package files

import (
	"os"
	"syscall"
)

// FileID returns the NTFS file index of path, which stays stable across
// renames/moves (ADR-007 fingerprint identity).
func FileID(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &data); err != nil {
		return 0, err
	}
	return uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow), nil
}
