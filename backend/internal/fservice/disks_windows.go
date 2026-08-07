//go:build windows

package fservice

import (
	"golang.org/x/sys/windows"
)

// listDisks enumerates the host's local drives. Only fixed and removable
// volumes are reported; network-mapped drives (DRIVE_REMOTE), CD-ROMs, floppies
// and phantom/unknown entries are excluded. Each disk carries its volume label
// and capacity when the host exposes them.
func listDisks() []Disk {
	var disks []Disk
	for c := 'A'; c <= 'Z'; c++ {
		path := string(c) + `:\`
		driveType := windows.GetDriveType(windows.StringToUTF16Ptr(path))
		var typ string
		switch driveType {
		case windows.DRIVE_FIXED:
			typ = "fixed"
		case windows.DRIVE_REMOVABLE:
			typ = "removable"
		default:
			continue
		}
		disk := Disk{Path: path, Type: typ, Label: volumeLabel(path)}
		var freeBytesAvailable, totalBytes, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(path),
			&freeBytesAvailable, &totalBytes, &totalFree); err == nil {
			disk.Total = int64(totalBytes)
			disk.Free = int64(freeBytesAvailable)
		}
		disks = append(disks, disk)
	}
	return disks
}

func volumeLabel(root string) string {
	var volName [261]uint16
	var serial, maxLen, flags uint32
	var fsName [261]uint16
	err := windows.GetVolumeInformation(
		windows.StringToUTF16Ptr(root),
		&volName[0], uint32(len(volName)), &serial, &maxLen, &flags,
		&fsName[0], uint32(len(fsName)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(volName[:])
}
