package fservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
)

// MediaSource CRUD for the generic file browser. A multimedia source is a
// lightweight, persistent marker on a directory: it never becomes a browsable
// object and is only ever a scan unit for the video library. Removing the
// marker never touches the library — only the real files under it change it.

// normalizeSourcePath canonicalizes a user-supplied source root so path prefix
// comparisons (routing) are consistent. Drive roots keep their trailing
// separator (C:\), everything else is a clean absolute path.
func normalizeSourcePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	clean := filepath.Clean(p)
	if len(clean) == 2 && clean[1] == ':' {
		return clean + string(filepath.Separator)
	}
	return clean
}

// AddSource marks a directory as a multimedia source, deduplicating by path.
func (s *Service) AddSource(ctx context.Context, path string) (domain.MediaSource, error) {
	path = normalizeSourcePath(path)
	if path == "" {
		return domain.MediaSource{}, domain.ErrInvalid
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.MediaSource{}, domain.ErrNotFound
		}
		return domain.MediaSource{}, err
	}
	if !info.IsDir() {
		return domain.MediaSource{}, domain.ErrInvalid
	}
	if src, err := s.sources.GetByPath(ctx, path); err == nil {
		return src, nil
	} else if err != domain.ErrNotFound {
		return domain.MediaSource{}, err
	}
	src := domain.MediaSource{
		ID:        ulid.Make().String(),
		Path:      path,
		CreatedAt: domain.Now(),
	}
	if err := s.sources.Create(ctx, src); err != nil {
		return domain.MediaSource{}, err
	}
	return src, nil
}

// RemoveSource removes a source marker only; the library keeps every video it
// already indexed until real files change.
func (s *Service) RemoveSource(ctx context.Context, path string) error {
	path = normalizeSourcePath(path)
	src, err := s.sources.GetByPath(ctx, path)
	if err != nil {
		if err == domain.ErrNotFound {
			return domain.ErrNotFound
		}
		return err
	}
	return s.sources.Delete(ctx, src.ID)
}

// ListSources returns every multimedia source marker, ordered by creation time.
func (s *Service) ListSources(ctx context.Context) ([]domain.MediaSource, error) {
	return s.sources.List(ctx)
}

// SourceByPath resolves a source marker by its normalized path (ErrNotFound
// when the path is not a source).
func (s *Service) SourceByPath(ctx context.Context, path string) (domain.MediaSource, error) {
	return s.sources.GetByPath(ctx, normalizeSourcePath(path))
}
