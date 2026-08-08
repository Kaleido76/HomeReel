// Package fservice powers the generic machine-wide file browser ("文件"
// tab): it lists drives and directories by absolute path and performs
// cut/copy/paste/rename/delete against the host filesystem. It never touches
// the database for file contents — no indexing, no scanning, no watchers.
// Only user pins (favorite paths) and multimedia-source markers persist, in
// the settings/DB layer.
package fservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
)

// ErrInvalidName is returned for a user-supplied name that is not a plain
// file/dir name (empty, dot segments, or containing separators).
var ErrInvalidName = errors.New("invalid name")

// Disk describes a local drive on the host (fixed or removable).
type Disk struct {
	Path  string `json:"path"`  // e.g. "C:\"
	Label string `json:"label"` // volume label, may be empty
	Type  string `json:"type"`  // fixed | removable
	Total int64  `json:"total"` // total bytes, 0 if unknown
	Free  int64  `json:"free"`  // free bytes, 0 if unknown
}

// Entry is a single filesystem entry listed by absolute path.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // absolute
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mtime"` // unix seconds
	IsVideo bool   `json:"is_video"`
}

// OpError records a per-item failure during a batch operation.
type OpError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// OpResult is the summary of a batch filesystem operation.
type OpResult struct {
	Done   int       `json:"done"`
	Errors []OpError `json:"errors,omitempty"`
}

// Service implements the generic file browser operations.
type Service struct {
	jobs    *jobs.Service
	pins    domain.SettingsRepo
	sources domain.SourceRepo
}

// New builds the generic file service. jobsSvc backs background copy/move; pins
// persists favorite paths and sources the multimedia-source markers, both in the
// settings/DB layer.
func New(jobsSvc *jobs.Service, pins domain.SettingsRepo, sources domain.SourceRepo) *Service {
	return &Service{jobs: jobsSvc, pins: pins, sources: sources}
}

// ListDisks enumerates the host's local drives (Windows) or the root (unix).
func (s *Service) ListDisks(ctx context.Context) []Disk {
	return listDisks()
}

// ListDir lists the directory at the absolute path, following symlinks/junctions
// so junctioned folders are navigable. Entries whose stat fails (broken link,
// permission) are skipped rather than failing the whole listing, as are Windows
// hidden/system entries ($RECYCLE.BIN, desktop.ini, …). The entry's own
// attributes are read with Lstat first (so a junction's attributes count, not
// its target's), then Stat decides directory/size for the visible result.
func (s *Service) ListDir(_ context.Context, path string) ([]Entry, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("empty path")
	}
	clean := filepath.Clean(path)
	dirEntries, err := os.ReadDir(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read dir %s: %w", clean, err)
	}
	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		full := filepath.Join(clean, de.Name())
		linkInfo, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if files.IsHiddenOrSystem(linkInfo) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		e := Entry{
			Name:    de.Name(),
			Path:    full,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		}
		if !e.IsDir {
			e.IsVideo = files.IsVideo(e.Name)
		}
		out = append(out, e)
	}
	return out, nil
}

// Rename renames the entry at path to newName within the same directory.
func (s *Service) Rename(_ context.Context, path, newName string) error {
	if !files.ValidName(newName) {
		return ErrInvalidName
	}
	return os.Rename(filepath.Clean(path), filepath.Join(filepath.Dir(filepath.Clean(path)), newName))
}

// Delete permanently removes each path (files and directories recursively).
// It is the caller's duty to have confirmed with the user first.
func (s *Service) Delete(_ context.Context, paths []string) OpResult {
	var res OpResult
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			res.Errors = append(res.Errors, OpError{Path: p, Message: err.Error()})
			continue
		}
		res.Done++
	}
	return res
}

const pinsKey = "files.pins"

// legacyPinsKey is the settings key used before the fs2→files rename; kept for
// a one-time migration so pins saved under the old name are not lost.
const legacyPinsKey = "fs2.pins"

// GetPins returns the pinned favorite paths (order preserved), never nil. When
// the current key is absent it migrates any pins stored under the legacy key.
func (s *Service) GetPins(ctx context.Context) ([]string, error) {
	raw, err := s.pins.Get(ctx, pinsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return s.migrateLegacyPins(ctx)
	}
	if err != nil {
		return nil, err
	}
	return parsePins(raw)
}

// migrateLegacyPins moves pins saved under the pre-rename key to the current
// key and returns them, or an empty (non-nil) list when there are none.
func (s *Service) migrateLegacyPins(ctx context.Context) ([]string, error) {
	raw, err := s.pins.Get(ctx, legacyPinsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.pins.Set(ctx, pinsKey, raw); err != nil {
		return nil, err
	}
	return parsePins(raw)
}

func parsePins(raw string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse pins: %w", err)
	}
	return out, nil
}

// AddPin pins a path, deduplicating. A non-empty existing pin is replaced in
// place so ordering stays stable.
func (s *Service) AddPin(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return domain.ErrInvalid
	}
	pins, err := s.GetPins(ctx)
	if err != nil {
		return err
	}
	for _, p := range pins {
		if p == path {
			return nil
		}
	}
	pins = append(pins, path)
	return s.savePins(ctx, pins)
}

// RemovePin unpins a path.
func (s *Service) RemovePin(ctx context.Context, path string) error {
	pins, err := s.GetPins(ctx)
	if err != nil {
		return err
	}
	out := pins[:0]
	for _, p := range pins {
		if p != path {
			out = append(out, p)
		}
	}
	return s.savePins(ctx, out)
}

func (s *Service) savePins(ctx context.Context, pins []string) error {
	raw, err := json.Marshal(pins)
	if err != nil {
		return err
	}
	return s.pins.Set(ctx, pinsKey, string(raw))
}
