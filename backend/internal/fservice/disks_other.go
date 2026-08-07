//go:build !windows

package fservice

// listDisks is a placeholder for non-Windows hosts: the machine browser is
// Windows-first, so on other platforms we expose the filesystem root.
func listDisks() []Disk {
	return []Disk{{Path: "/", Type: "fixed"}}
}
