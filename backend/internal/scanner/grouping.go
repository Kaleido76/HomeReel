package scanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"

	"homereel/backend/internal/domain"
)

// Series membership rules (管理面定稿, 2026-08):
//
//   - A series is a user-created container bound to a root directory
//     (seasons.root_path). Its members are exactly the videos that live as
//     direct children of that folder, ordered by file name 1..N.
//   - Scans never create, delete or rename series definitions — they only
//     maintain membership of existing series (bind files that now sit under a
//     series root, detach files that no longer do, prune empty series).
//   - Series are created only by manual marking (mark.go) or an explicit
//     series sync.
//
// Every maintenance pass converges the library towards "path + file_id decide
// everything": a video is a member of the series whose root is its parent
// folder, and standalone otherwise.
func (s *Service) maintainSeriesMembers(ctx context.Context) error {
	all, err := s.videos.ListAll(ctx)
	if err != nil {
		return err
	}
	seriesList, err := s.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		return err
	}
	roots := make(map[string]domain.Series, len(seriesList))
	for _, se := range seriesList {
		if se.RootPath != "" {
			roots[filepath.Clean(se.RootPath)] = se
		}
	}
	byDir := make(map[string][]domain.Video, len(all))
	for _, v := range all {
		d := filepath.Dir(v.Path)
		byDir[d] = append(byDir[d], v)
	}

	// Bind every video that currently lives directly under a series root to
	// that series (filename order, or the preserved manual order). BindMembers
	// re-points each member at the series' show, so a file moved between series
	// lands in the right one.
	for root, se := range roots {
		members := byDir[root]
		if len(members) == 0 {
			continue
		}
		files := make([]seriesFile, 0, len(members))
		for _, m := range members {
			files = append(files, seriesFile{id: m.ID, rel: filepath.Base(m.Path)})
		}
		if err := s.bindSeriesMembers(ctx, se, files); err != nil {
			return err
		}
	}

	// Detach any video still bound to a series whose root is no longer its
	// parent folder (moved out of the series / into a plain directory).
	var strays []string
	for _, v := range all {
		if v.Kind != "episode" || v.ShowID == "" {
			continue
		}
		if _, ok := roots[filepath.Dir(v.Path)]; !ok {
			strays = append(strays, v.ID)
		}
	}
	if len(strays) > 0 {
		if err := s.videos.AssignStandalone(ctx, strays); err != nil {
			return err
		}
	}

	s.pruneEmptyShows(ctx)
	return nil
}

// seriesFile pairs a video id with its path (relative or base) for order
// convergence. rel drives the default file-name ordering.
type seriesFile struct {
	id  string
	rel string
}

// bindSeriesMembers converges a series' member order and titles. A series with
// sort_manual (拖拽重排, ADR-015 修订) keeps its existing member order and only
// appends newly imported files at the end (in file-name order); a regular series
// is re-sorted by file name 1..N. Titles follow the file name (title_source is
// reset to 'file' by BindMembers) EXCEPT for members whose title was manually
// edited (title_source='manual'): those keep their current title so user edits
// survive scans (ADR-015/017 同语义).
func (s *Service) bindSeriesMembers(ctx context.Context, se domain.Series, files []seriesFile) error {
	existing, err := s.series.GetMembers(ctx, se.ID)
	if err != nil {
		return err
	}
	// 手动编辑过标题的成员（title_source='manual'）跨扫描保留当前标题；
	// 其余成员标题始终随文件名刷新。episode_title 一并保留（member list
	// 显示 episode_title || title，二者在 BindMembers 中始终同值）。
	manualTitles := make(map[string]string, len(existing))
	for _, m := range existing {
		if m.TitleSource == domain.TitleSourceManual {
			manualTitles[m.VideoID] = m.Title
		}
	}
	titleFor := func(id string, rel string) (string, string) {
		if t, ok := manualTitles[id]; ok {
			return t, domain.TitleSourceManual
		}
		return titleFromPath(rel), domain.TitleSourceFile
	}

	if se.SortManual {
		known := make(map[string]bool, len(existing))
		maxEp := 0
		for _, m := range existing {
			known[m.VideoID] = true
			if m.EpisodeNumber > maxEp {
				maxEp = m.EpisodeNumber
			}
		}
		// 新文件按文件名序追加到手动顺序末尾；既有成员保持原 episode_number。
		var fresh []seriesFile
		for _, f := range files {
			if !known[f.id] {
				fresh = append(fresh, f)
			}
		}
		sort.Slice(fresh, func(i, j int) bool { return fresh[i].rel < fresh[j].rel })
		assigns := make([]domain.EpisodeAssign, 0, len(existing)+len(fresh))
		for _, m := range existing {
			title, titleSource := titleFor(m.VideoID, relOf(files, m.VideoID))
			assigns = append(assigns, domain.EpisodeAssign{
				VideoID:       m.VideoID,
				EpisodeNumber: m.EpisodeNumber,
				Title:         title,
				TitleSource:   titleSource,
			})
		}
		for _, f := range fresh {
			maxEp++
			title, titleSource := titleFor(f.id, f.rel)
			assigns = append(assigns, domain.EpisodeAssign{
				VideoID:       f.id,
				EpisodeNumber: maxEp,
				Title:         title,
				TitleSource:   titleSource,
			})
		}
		return s.series.BindMembers(ctx, se.ID, assigns)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	assigns := make([]domain.EpisodeAssign, 0, len(files))
	for i, f := range files {
		title, titleSource := titleFor(f.id, f.rel)
		assigns = append(assigns, domain.EpisodeAssign{
			VideoID:       f.id,
			EpisodeNumber: i + 1,
			Title:         title,
			TitleSource:   titleSource,
		})
	}
	return s.series.BindMembers(ctx, se.ID, assigns)
}

// relOf looks up a file's rel by video id (existing members keep their title
// derived from the current path).
func relOf(files []seriesFile, id string) string {
	for _, f := range files {
		if f.id == id {
			return f.rel
		}
	}
	return ""
}

// pruneEmptyShows removes any show that no longer has videos (empty series).
func (s *Service) pruneEmptyShows(ctx context.Context) {
	shows, err := s.shows.List(ctx)
	if err != nil {
		slog.Warn("prune empty shows list", "err", err)
		return
	}
	for _, sh := range shows {
		if err := s.shows.RemoveEmptyShow(ctx, sh.ID); err != nil {
			slog.Warn("prune empty show", "show_id", sh.ID, "err", err)
		}
	}
}
