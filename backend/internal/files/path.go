package files

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrOutsideRoot is returned when a requested path escapes the volume root.
var ErrOutsideRoot = errors.New("path outside storage root")

// ErrInvalidName is returned when a user-supplied name is not a plain
// file/dir name (empty, dot segments, or containing separators).
var ErrInvalidName = errors.New("invalid name")

// ErrMissingChunk is returned when an upload cannot be assembled because one
// or more chunk parts are missing.
var ErrMissingChunk = errors.New("missing upload chunk")

// resolve joins rel onto root and verifies the result stays inside root.
func resolve(root, rel string) (string, error) {
	if root == "" {
		return "", errors.New("empty root")
	}
	if rel == "" {
		rel = "."
	} else {
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
			return "", ErrOutsideRoot
		}
		rel = filepath.Clean(rel)
	}
	full := filepath.Join(root, rel)
	if !within(root, full) {
		return "", ErrOutsideRoot
	}
	return full, nil
}

func within(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

// validName reports whether name is acceptable as a file or directory name.
func validName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}
