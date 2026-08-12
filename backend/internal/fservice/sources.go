package fservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/files"
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

// ErrNestedSource reports marking a directory that nests inside (or contains)
// an existing media source — media sources may not overlap (ADR-017).
var ErrNestedSource = errors.New("nested media source")

// AddSource marks a directory as a multimedia source, deduplicating by path.
// Media sources may not nest: a directory inside another source (or containing
// one) is rejected, so every file in the library belongs to exactly one source.
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
	existing, err := s.sources.List(ctx)
	if err != nil {
		return domain.MediaSource{}, err
	}
	for _, e := range existing {
		if files.UnderRoot(path, e.Path) {
			return domain.MediaSource{}, fmt.Errorf("%w: 该目录位于多媒体源「%s」内部，多媒体源不允许嵌套", ErrNestedSource, e.Path)
		}
		if files.UnderRoot(e.Path, path) {
			return domain.MediaSource{}, fmt.Errorf("%w: 该目录包含了多媒体源「%s」，多媒体源不允许嵌套", ErrNestedSource, e.Path)
		}
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

// MediaSource CRUD for the generic file browser. A multimedia source is a
// lightweight, persistent marker on a directory: it never becomes a browsable
// object and is only ever a scan unit for the video library. This service only
// manages the marker itself; the api layer deletes the source's library rows
// when the marker is removed (取消多媒体源 → 其下所有单集与系列从库中消失).
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
