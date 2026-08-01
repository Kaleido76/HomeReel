package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Service wraps filesystem operations for the Explorer (ADR-011): browse,
// and now rename/move/delete/mkdir/download plus chunked upload.
type Service struct {
	uploadsDir string
}

// NewService builds a filesystem service. uploadsDir is where chunk parts are
// staged before assembly (data_dir/uploads).
func NewService(uploadsDir string) *Service {
	if uploadsDir == "" {
		uploadsDir = os.TempDir()
	}
	return &Service{uploadsDir: uploadsDir}
}

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".ts": true, ".m2ts": true, ".3gp": true, ".ogv": true,
}

// IsVideo reports whether a file name has a video extension.
func IsVideo(name string) bool {
	return videoExts[strings.ToLower(filepath.Ext(name))]
}

// Entry is a single filesystem entry inside a storage volume.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // slash-separated, relative to the volume root
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mtime"` // unix seconds
	IsVideo bool   `json:"is_video"`
}

// ListDir lists the directory at rel ("" = volume root) under root.
// Directories sort first, then entries by name; each within group is
// case-insensitive sorted.
func (s *Service) ListDir(root, rel string) ([]Entry, error) {
	full, err := resolve(root, rel)
	if err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read dir %s: %w", full, err)
	}
	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		e := Entry{
			Name:  de.Name(),
			IsDir: de.IsDir(),
		}
		if info, err := de.Info(); err == nil {
			e.Size = info.Size()
			e.ModTime = info.ModTime().Unix()
		}
		if !e.IsDir {
			e.IsVideo = IsVideo(e.Name)
		}
		e.Path = entryPath(rel, e.Name)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// entryPath joins rel and name into a slash-separated relative path, using ""
// for the volume root.
func entryPath(rel, name string) string {
	if rel == "" {
		return name
	}
	return filepath.ToSlash(filepath.Join(rel, name))
}
