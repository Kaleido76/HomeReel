package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// OpError records a per-item failure during a batch filesystem operation.
type OpError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// OpResult is the summary of a batch filesystem operation.
type OpResult struct {
	Done   int       `json:"done"`
	Errors []OpError `json:"errors,omitempty"`
}

// Mkdir creates a subdirectory named name inside rel ("" = volume root).
func (s *Service) Mkdir(root, rel, name string) error {
	if !validName(name) {
		return ErrInvalidName
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	return os.Mkdir(filepath.Join(full, name), 0o755)
}

// Rename renames the entry at rel to newName within the same directory.
func (s *Service) Rename(root, rel, newName string) error {
	if !validName(newName) {
		return ErrInvalidName
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	return os.Rename(full, filepath.Join(filepath.Dir(full), newName))
}

// Move moves each entry in rels into the existing directory destRel.
func (s *Service) Move(root string, rels []string, destRel string) OpResult {
	var res OpResult
	destFull, err := resolve(root, destRel)
	if err != nil {
		res.Errors = append(res.Errors, OpError{Path: destRel, Message: err.Error()})
		return res
	}
	info, err := os.Stat(destFull)
	if err != nil || !info.IsDir() {
		res.Errors = append(res.Errors, OpError{Path: destRel, Message: "目标不存在或不是目录"})
		return res
	}
	for _, rel := range rels {
		full, err := resolve(root, rel)
		if err != nil {
			res.Errors = append(res.Errors, OpError{Path: rel, Message: err.Error()})
			continue
		}
		target := filepath.Join(destFull, filepath.Base(full))
		if _, err := os.Stat(target); err == nil {
			res.Errors = append(res.Errors, OpError{Path: rel, Message: "目标已存在"})
			continue
		}
		if err := os.Rename(full, target); err != nil {
			res.Errors = append(res.Errors, OpError{Path: rel, Message: err.Error()})
			continue
		}
		res.Done++
	}
	return res
}

// Delete removes each entry in rels (files and directories recursively).
func (s *Service) Delete(root string, rels []string) OpResult {
	var res OpResult
	for _, rel := range rels {
		full, err := resolve(root, rel)
		if err != nil {
			res.Errors = append(res.Errors, OpError{Path: rel, Message: err.Error()})
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			res.Errors = append(res.Errors, OpError{Path: rel, Message: err.Error()})
			continue
		}
		res.Done++
	}
	return res
}

// OpenFile opens the entry at rel for reading (download).
func (s *Service) OpenFile(root, rel string) (*os.File, error) {
	full, err := resolve(root, rel)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

// SaveChunk stages one upload chunk under <uploadsDir>/<uploadID>/.
func (s *Service) SaveChunk(uploadID string, index int, r io.Reader) error {
	if uploadID == "" || index < 0 {
		return ErrInvalidName
	}
	dir := filepath.Join(s.uploadsDir, uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := os.Create(filepath.Join(dir, fmt.Sprintf("%05d.part", index)))
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

// CompleteUpload assembles chunkTotal staged parts into filename inside
// destRel, then cleans up the staging directory. It must only be called once
// all chunks have been saved. When the destination already exists but parts
// are missing, it is treated as an already-completed upload (the final
// response was lost and the client retried the last chunk).
func (s *Service) CompleteUpload(uploadID, filename, root, destRel string, chunkTotal int) error {
	if !validName(filename) {
		return ErrInvalidName
	}
	dir := filepath.Join(s.uploadsDir, uploadID)
	destFull, err := resolve(root, destRel)
	if err != nil {
		return err
	}
	destPath := filepath.Join(destFull, filename)
	for i := 0; i < chunkTotal; i++ {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%05d.part", i))); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, statErr := os.Stat(destPath); statErr == nil {
					_ = os.RemoveAll(dir)
					return nil
				}
				return ErrMissingChunk
			}
			return err
		}
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	for i := 0; i < chunkTotal; i++ {
		part, err := os.Open(filepath.Join(dir, fmt.Sprintf("%05d.part", i)))
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, part); err != nil {
			part.Close()
			return err
		}
		part.Close()
	}
	_ = os.RemoveAll(dir)
	return nil
}

// CleanupStaleUploads removes upload staging directories that have not been
// touched for maxAge (interrupted uploads leave orphaned chunks behind).
func (s *Service) CleanupStaleUploads(maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(s.uploadsDir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
