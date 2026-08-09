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
	// that series (filename order). BindMembers re-points each member at the
	// series' show, so a file moved between series lands in the right one.
	for root, se := range roots {
		members := byDir[root]
		if len(members) == 0 {
			continue
		}
		sort.Slice(members, func(i, j int) bool {
			return filepath.Base(members[i].Path) < filepath.Base(members[j].Path)
		})
		assigns := make([]domain.EpisodeAssign, 0, len(members))
		for i, m := range members {
			assigns = append(assigns, domain.EpisodeAssign{
				VideoID:       m.ID,
				EpisodeNumber: i + 1,
				Title:         titleFromPath(m.RelativePath),
			})
		}
		if err := s.series.BindMembers(ctx, se.ID, assigns); err != nil {
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
