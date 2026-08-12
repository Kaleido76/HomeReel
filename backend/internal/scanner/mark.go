package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
)

// handleMarkResource imports a user-marked folder as a series (文件页签
// 「标记为系列」). The path must lie inside a media source — discrete resources
// no longer exist (管理面定稿 2026-08). Running the same job again is an
// idempotent refresh: files are imported/updated by file_id and membership is
// re-derived from the folder.
func (s *Service) handleMarkResource(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var meta struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.Path == "" || meta.Kind != "series" {
		return errors.New("mark resource job requires kind=series and a path")
	}
	if _, err := os.Stat(meta.Path); err != nil {
		return err
	}
	srcID, ok := s.containingSource(ctx, meta.Path)
	if !ok {
		return errors.New("标记的路径不在任何媒体源内，请先将该目录添加为多媒体源")
	}
	return s.markSeries(ctx, meta.Path, srcID, report)
}

// containingSource resolves the deepest media source that contains path.
func (s *Service) containingSource(ctx context.Context, path string) (string, bool) {
	id, _, ok := s.containingSourceRoot(ctx, path)
	return id, ok
}

// markSeries turns one folder into one series: its direct video children become
// the members (filename order). Nested folders are not series themselves and
// their videos stay standalone. Creating a series inside an existing series (or
// containing one) is rejected — membership is strictly the direct children.
func (s *Service) markSeries(ctx context.Context, path, srcID string, report jobs.Reporter) error {
	if err := s.validateSeriesRoot(ctx, path); err != nil {
		return err
	}
	cands, err := s.collectDirectChildren(path)
	if err != nil {
		return err
	}
	ids, err := s.importCandidates(ctx, cands, srcID, func(done, total int) {
		report.Progress(float64(done) / float64(total))
	})
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("该文件夹下没有可直接作为系列成员的视频文件")
	}
	if _, err := s.syncSeriesFolder(ctx, path, cands, ids, true); err != nil {
		return err
	}
	return nil
}

// validateSeriesRoot rejects marking a path that nests inside an existing
// series root or that contains one (a series' members must be its direct
// children, so series cannot be nested). Marking the same root is allowed —
// that is an idempotent refresh.
func (s *Service) validateSeriesRoot(ctx context.Context, path string) error {
	clean := filepath.Clean(path)
	seriesList, err := s.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		return err
	}
	for _, se := range seriesList {
		if se.RootPath == "" {
			continue
		}
		root := filepath.Clean(se.RootPath)
		switch {
		case root == clean:
			continue
		case files.UnderRoot(clean, root):
			return fmt.Errorf("该目录位于系列「%s」内部，系列不能嵌套", se.Name)
		case files.UnderRoot(root, clean):
			return fmt.Errorf("该目录包含了系列「%s」，无法作为系列根目录", se.Name)
		}
	}
	return nil
}

// syncSeriesFolder binds a folder's members to its series (creating the series
// when create is true), refreshes every member's file-derived title, removes
// members whose source file vanished, and cleans up empty series. It returns
// the series' show id. candidates and ids must correspond (ids[i] is the video
// id of candidates[i]).
func (s *Service) syncSeriesFolder(ctx context.Context, dir string, cands []candidate, ids []string, create bool) (string, error) {
	clean := filepath.Clean(dir)
	series, err := s.series.FindByRoot(ctx, clean)
	if errors.Is(err, domain.ErrNotFound) {
		if !create {
			return "", errors.New("series root not registered")
		}
		name := filepath.Base(clean)
		if name == "" || name == "." {
			return "", domain.ErrInvalid
		}
		series, err = s.series.CreateAtRoot(ctx, name, clean)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	type member struct {
		id  string
		rel string
	}
	byID := make(map[string]string, len(ids))
	for i, id := range ids {
		byID[id] = cands[i].rel
	}
	members := make([]member, 0, len(ids))
	for _, id := range ids {
		members = append(members, member{id: id, rel: byID[id]})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].rel < members[j].rel })
	assigns := make([]domain.EpisodeAssign, 0, len(members))
	for i, m := range members {
		assigns = append(assigns, domain.EpisodeAssign{
			VideoID:       m.id,
			EpisodeNumber: i + 1,
			Title:         titleFromPath(m.rel),
		})
	}
	if len(assigns) > 0 {
		if err := s.series.BindMembers(ctx, series.ID, assigns); err != nil {
			return "", err
		}
	}
	if err := s.pruneVanishedMembers(ctx, series.ShowID); err != nil {
		return "", err
	}
	if err := s.series.SyncShowLinks(ctx, series.ShowID); err != nil {
		return "", err
	}
	s.pruneEmptyShows(ctx)
	return series.ShowID, nil
}

// pruneVanishedMembers deletes a show's members whose source file is no longer
// on disk (deleted or moved away), so manually syncing a series refreshes its
// episode list.
func (s *Service) pruneVanishedMembers(ctx context.Context, showID string) error {
	all, err := s.videos.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, v := range all {
		if v.ShowID != showID {
			continue
		}
		if _, err := os.Stat(v.Path); err == nil {
			continue
		}
		if err := s.videos.Delete(ctx, v.ID); err == nil {
			s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": v.ID}})
		}
	}
	return nil
}

// collectDirectChildren lists the video files that are direct children of root
// (one level, no recursion). Series membership is exactly these files.
func (s *Service) collectDirectChildren(root string) ([]candidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]candidate, 0, len(entries))
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		if !files.IsVideo(d.Name()) {
			continue
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if files.IsHiddenOrSystem(info) {
			continue
		}
		path := filepath.Join(root, d.Name())
		fid, err := files.FileID(path)
		if err != nil {
			continue
		}
		out = append(out, candidate{
			path:   path,
			rel:    d.Name(),
			fileID: fmt.Sprintf("%d", fid),
			size:   info.Size(),
			mtime:  info.ModTime().UnixMilli(),
		})
	}
	return out, nil
}

// importCandidates imports every candidate (creating the row with full probe
// metadata, or re-binding an existing row by global file_id), and returns the
// imported video ids in candidate order (ids match cands[i]). New rows publish
// VideoImported so the async thumbnail listener covers covers (ADR-010).
func (s *Service) importCandidates(ctx context.Context, cands []candidate, srcID string, progress func(done, total int)) ([]string, error) {
	all, err := s.videos.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byFile := make(map[string]domain.Video, len(all))
	for _, v := range all {
		byFile[v.FileID] = v
	}
	ids := make([]string, 0, len(cands))
	for i, c := range cands {
		if progress != nil {
			progress(i, len(cands))
		}
		if err := ctx.Err(); err != nil {
			return ids, err
		}
		id, err := s.normalizeCandidate(ctx, c, srcID, byFile, false, nil)
		if err != nil {
			slog.Warn("import candidate", "path", c.path, "err", err)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// collect walks a source root for video candidates, skipping the subtrees of
// descendant sources and hidden/system entries.
func (s *Service) collect(root string, skipSet map[string]bool) ([]candidate, error) {
	var out []candidate
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if skipSet[path] {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if info, ierr := d.Info(); ierr == nil && files.IsHiddenOrSystem(info) {
				return filepath.SkipDir
			}
			return nil
		}
		if !files.IsVideo(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if files.IsHiddenOrSystem(info) {
			return nil
		}
		fid, err := files.FileID(path)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, candidate{
			path:   path,
			rel:    filepath.ToSlash(rel),
			fileID: fmt.Sprintf("%d", fid),
			size:   info.Size(),
			mtime:  info.ModTime().UnixMilli(),
		})
		return nil
	})
	return out, err
}
