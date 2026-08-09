package files

import (
	"path/filepath"
	"strings"
)

// ValidName reports whether name is acceptable as a file or directory name
// (non-empty, not "." or "..", and free of path separators). It is the single
// gate for user-supplied names across the file browser.
func ValidName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// UnderRoot reports whether path equals root or is strictly inside it, with a
// proper path-separator boundary so C:\Videos and C:\Videos2 stay distinct.
func UnderRoot(path, root string) bool {
	clean := filepath.Clean(path)
	base := filepath.Clean(root)
	if clean == base {
		return true
	}
	if strings.HasSuffix(base, string(filepath.Separator)) {
		return strings.HasPrefix(clean, base)
	}
	return strings.HasPrefix(clean, base+string(filepath.Separator))
}

// UnderAnyRoot reports whether path sits under any of the given roots.
func UnderAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if UnderRoot(path, root) {
			return true
		}
	}
	return false
}

// ContainingRoot returns the deepest (smallest) of the given roots that
// contains path — for nested media sources this picks the most specific one —
// and whether any matched.
func ContainingRoot(path string, roots []string) (string, bool) {
	best := ""
	bestLen := -1
	for _, root := range roots {
		if UnderRoot(path, root) && len(root) > bestLen {
			best = root
			bestLen = len(root)
		}
	}
	return best, bestLen >= 0
}
