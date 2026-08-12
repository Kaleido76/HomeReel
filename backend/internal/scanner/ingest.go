package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
)

// IngestPaths normalizes files/folders that entered the library's maintenance
// scope through any HomeReel route (copy/move/rename in the file browser,
// format-factory output, future uploads and file watching). Every video is
// probed and indexed at this moment: existing rows are relocated by global
// file_id (identity and history survive moves), new files get a full probe and
// row, and series membership is converged — nothing is left stale until the
// next scan (ADR-017). Paths outside every media source are ignored: the
// library's scope is exactly the media sources.
func (s *Service) IngestPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.videos.ListAll(ctx)
	if err != nil {
		return err
	}
	byFile := make(map[string]domain.Video, len(all))
	for _, v := range all {
		byFile[v.FileID] = v
	}

	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.ingestPath(ctx, p, byFile)
	}
	return s.maintainSeriesMembers(ctx)
}

func (s *Service) ingestPath(ctx context.Context, p string, byFile map[string]domain.Video) {
	clean := filepath.Clean(p)
	info, err := os.Stat(clean)
	if err != nil {
		return // already gone — EvictPaths handles removal
	}
	if info.IsDir() {
		s.ingestDir(ctx, clean, byFile)
		return
	}
	if !files.IsVideo(clean) {
		return
	}
	srcID, srcRoot, ok := s.containingSourceRoot(ctx, clean)
	if !ok {
		return // outside every media source: outside the library's scope
	}
	rel, err := filepath.Rel(srcRoot, clean)
	if err != nil {
		return
	}
	c, ok := candidateFor(clean, filepath.ToSlash(rel), info)
	if !ok {
		return
	}
	if _, err := s.normalizeCandidate(ctx, c, srcID, byFile, false, nil); err != nil {
		slog.Warn("ingest file", "path", clean, "err", err)
	}
}

// ingestDir walks a folder (recursively) and ingests every video under it,
// skipping any descendant source subtree (a legacy nested source owns its
// subtree, matching the scan routing table).
func (s *Service) ingestDir(ctx context.Context, dir string, byFile map[string]domain.Video) {
	srcID, srcRoot, ok := s.containingSourceRoot(ctx, dir)
	if !ok {
		return
	}
	all, err := s.sources.List(ctx)
	if err != nil {
		slog.Warn("ingest list sources", "err", err)
		return
	}
	cands, err := s.collect(dir, descendantSourceSkipSet(all, dir, srcID))
	if err != nil {
		slog.Warn("ingest collect", "path", dir, "err", err)
		return
	}
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return
		}
		rel, rerr := filepath.Rel(srcRoot, c.path)
		if rerr != nil {
			continue
		}
		c.rel = filepath.ToSlash(rel)
		if _, err := s.normalizeCandidate(ctx, c, srcID, byFile, false, nil); err != nil {
			slog.Warn("ingest dir file", "path", c.path, "err", err)
		}
	}
}

// EvictPaths removes library rows whose source file vanished from disk under
// the given paths (files deleted in the browser, or moved out of every media
// source). Deleted rows publish VideoDeleted; remaining membership is converged
// (strays detached, empty shows pruned). Moves keep their identity because
// IngestPaths runs first and relocates rows by file_id, so they no longer sit
// under the evicted paths.
func (s *Service) EvictPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	roots := make([]string, 0, len(paths))
	for _, p := range paths {
		roots = append(roots, filepath.Clean(p))
	}
	all, err := s.videos.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, v := range all {
		if !files.UnderAnyRoot(v.Path, roots) {
			continue
		}
		if _, err := os.Stat(v.Path); err == nil {
			continue // still on disk (Ingest relocated it, or it was untouched)
		}
		if err := s.videos.Delete(ctx, v.ID); err == nil {
			s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": v.ID}})
		} else {
			slog.Warn("evict video", "video_id", v.ID, "err", err)
		}
	}
	return s.maintainSeriesMembers(ctx)
}

// normalizeCandidate is the single per-file ingestion step shared by scans,
// mark series, syncs and ad-hoc IngestPaths: an existing row matched globally
// by file_id is relocated/refreshed, a new file is probed and created. When
// inlineThumb is false a new row publishes VideoImported (the async thumbnail
// listener covers covers); bulk scans pass true to keep the serial thumbnail
// progress UX and skip the event. VideoUpdated is published only when the file
// content fingerprint changed — a pure rename/move must not churn caches.
// Manually edited titles (title_source=manual) are preserved.
func (s *Service) normalizeCandidate(ctx context.Context, c candidate, srcID string, byFile map[string]domain.Video, inlineThumb bool, subtask subtaskFn) (string, error) {
	now := s.now().UTC().Format(domain.TimeLayout)
	if cur, ok := byFile[c.fileID]; ok {
		contentChanged := cur.Size != c.size || cur.MTime != c.mtime
		if contentChanged || cur.Path != c.path || cur.SourceID != srcID {
			if err := s.videos.UpdateFingerprint(ctx, cur.ID, srcID, c.path, c.rel, c.size, c.mtime, now); err != nil {
				return "", err
			}
		}
		if contentChanged || needsProbe(cur) {
			s.processInline(ctx, cur.ID, subtask)
		}
		if contentChanged {
			s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": cur.ID}})
		}
		return cur.ID, nil
	}

	base := filepath.Base(c.path)
	if subtask != nil {
		subtask("探测 "+base, 5)
	}
	v := domain.Video{
		ID:            ulid.Make().String(),
		SourceID:      srcID,
		FileID:        c.fileID,
		RelativePath:  c.rel,
		Path:          c.path,
		Size:          c.size,
		MTime:         c.mtime,
		Kind:          "movie",
		Title:         titleFromPath(c.rel),
		TitleSource:   domain.TitleSourceFile,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastScannedAt: now,
	}
	if info, perr := s.probe(ctx, s.ffprobePath, c.path); perr == nil {
		v.Duration = info.Duration
		v.Codec = info.Codec
		v.AudioCodec = info.AudioCodec
		v.Container = info.Container
		v.Segmented = info.Segmented
		v.FastStart = info.FastStart
		v.Width = info.Width
		v.Height = info.Height
	} else {
		slog.Warn("ingest probe", "path", c.path, "err", perr)
	}
	if err := s.videos.Create(ctx, v); err != nil {
		return "", err
	}
	byFile[c.fileID] = v
	if inlineThumb {
		if subtask != nil {
			subtask("生成 "+base+" 的缩略图", 60)
		}
		s.thumbnailFor(ctx, v.ID, c.path, v.Duration)
		if subtask != nil {
			subtask("", -1)
		}
	} else {
		s.bus.Publish(events.Event{Type: events.VideoImported, Data: map[string]string{"video_id": v.ID}})
	}
	return v.ID, nil
}

// candidateFor builds a scan candidate for one file.
func candidateFor(path, rel string, info os.FileInfo) (candidate, bool) {
	fid, err := files.FileID(path)
	if err != nil {
		return candidate{}, false
	}
	return candidate{
		path:   path,
		rel:    rel,
		fileID: fmt.Sprintf("%d", fid),
		size:   info.Size(),
		mtime:  info.ModTime().UnixMilli(),
	}, true
}

// containingSourceRoot resolves the deepest media source containing path,
// returning its id and root path.
func (s *Service) containingSourceRoot(ctx context.Context, path string) (string, string, bool) {
	all, err := s.sources.List(ctx)
	if err != nil {
		slog.Warn("list sources for ingest", "err", err)
		return "", "", false
	}
	roots := make([]string, 0, len(all))
	for _, src := range all {
		roots = append(roots, src.Path)
	}
	root, ok := files.ContainingRoot(path, roots)
	if !ok {
		return "", "", false
	}
	for _, src := range all {
		if src.Path == root {
			return src.ID, root, true
		}
	}
	return "", "", false
}

// descendantSourceSkipSet is the scan routing table: the roots of sources
// strictly inside root (excluding selfID) whose subtrees the walk must skip —
// a child source owns its subtree. It is what keeps legacy nested sources
// correct; new sources reject nesting at creation (ADR-017).
func descendantSourceSkipSet(all []domain.MediaSource, root, selfID string) map[string]bool {
	skip := map[string]bool{}
	for _, o := range all {
		if o.ID != selfID && files.UnderRoot(o.Path, root) {
			skip[filepath.Clean(o.Path)] = true
		}
	}
	return skip
}
