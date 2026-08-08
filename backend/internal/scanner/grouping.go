package scanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"homereel/backend/internal/domain"
)

// dirMember is one video in a series group, with its sort number within the
// season (the number extracted from the file name for numbered files, or a
// filename-order position for unnumbered ones).
type dirMember struct {
	videoID string
	ep      int
}

// dirGroup is a directory-level series decision: one show, mapping season
// number to its member videos. All series are alike — there is no movie/tv
// structural type; any such distinction lives on tags instead.
type dirGroup struct {
	show    string
	seasons map[int][]dirMember
}

// classifyDir decides whether a directory holding ≥2 videos is a series, and
// only then. A series strictly corresponds to its physical folder: every video
// in the folder must match one obvious pattern (numbered files under one title,
// or a common title prefix with distinct unnumbered suffixes) — a single
// non-matching video keeps the whole folder standalone, it is never grouped
// while also being part of the series. Non-multimedia files (subtitles,
// artwork, nfo, …) are never imported as videos, so they do not participate in
// this decision. Existing groupings are never un-grouped, so future manual
// grouping by the user is never undone.
//
// This is a deliberately small set of robust rules (AGENTS.md §3.6): do not
// grow it with more patterns/fuzzy matching — a missed case is left standalone
// and fixed by the user manually, which is cheaper than more heuristics.
func classifyDir(dir string, videos []domain.Video) *dirGroup {
	if len(videos) < 2 {
		return nil
	}
	if g := classifyNumbered(dir, videos); g != nil {
		return g
	}
	return classifyUnnumbered(videos)
}

// episodeMarker returns the (season, episode) numeric sort key of a video when
// its name carries a numeric marker — a TV episode (SxxEyy / 第x集 / a trailing
// number inside a "Season N" folder) or a part (Part N / 第N部 / trailing
// number). ok=false when the name has no usable number.
func episodeMarker(rel string) (season, ep int, ok bool) {
	if hint := ParseEpisode(rel); hint.HasSE && hint.Episode > 0 {
		season = hint.Season
		if season <= 0 {
			season = 1
		}
		return season, hint.Episode, true
	}
	if part, isPart := ParseMoviePart(rel); isPart && part.Season > 0 {
		return part.Season, 1, true
	}
	return 0, 0, false
}

// classifyNumbered groups numbered videos under one series. Inside a "Season N"
// folder the folder is the relationship: every video must carry a numeric
// marker and is grouped under the parent show name in that folder's season. In
// a flat directory every video must share one title key and carry a numeric
// marker — the folder is the series and may hold no other video object.
func classifyNumbered(dir string, videos []domain.Video) *dirGroup {
	if seasonNum, inSeason := parseSeasonDir(filepath.Base(dir)); inSeason {
		show := cleanShowName(filepath.Base(filepath.Dir(dir)))
		if show == "" {
			return nil
		}
		members := make([]dirMember, 0, len(videos))
		for _, v := range videos {
			if _, ep, ok := episodeMarker(v.RelativePath); ok {
				members = append(members, dirMember{videoID: v.ID, ep: ep})
			} else {
				return nil
			}
		}
		if len(members) >= 2 {
			return &dirGroup{show: show, seasons: map[int][]dirMember{seasonNum: members}}
		}
		return nil
	}

	// Flat directory: all videos must match the one numbered pattern. A single
	// non-matching video keeps the whole folder standalone.
	type keyed struct {
		videoID string
		season  int
		ep      int
	}
	var key string
	members := make([]keyed, 0, len(videos))
	for _, v := range videos {
		k := titleKeyOf(v.RelativePath)
		if k == "" {
			return nil
		}
		season, ep, ok := episodeMarker(v.RelativePath)
		if !ok {
			return nil
		}
		if key == "" {
			key = k
		} else if key != k {
			return nil
		}
		members = append(members, keyed{videoID: v.ID, season: season, ep: ep})
	}
	if len(members) < 2 {
		return nil
	}
	seasons := map[int][]dirMember{}
	for _, m := range members {
		seasons[m.season] = append(seasons[m.season], dirMember{videoID: m.videoID, ep: m.ep})
	}
	return &dirGroup{show: key, seasons: seasons}
}

// classifyUnnumbered groups videos that carry no numbers but still share an
// obvious pattern: a common title prefix ending in a separator (：/·/-/_/space),
// with every file adding a distinct non-empty suffix (e.g. "XX冒险记：北京" /
// "XX冒险记：上海"). The prefix must be long enough to not match unrelated
// titles, and every file must fit — this pattern is strict. Members are ordered
// by file name (season 1); the order is adjustable by other means later.
func classifyUnnumbered(videos []domain.Video) *dirGroup {
	names := make([]string, len(videos))
	for i, v := range videos {
		names[i] = strings.TrimSuffix(filepath.Base(v.Path), filepath.Ext(filepath.Base(v.Path)))
	}
	prefix := lcp(names)
	lastRune, _ := utf8.DecodeLastRuneInString(prefix)
	if prefix == "" || !isDelimiter(lastRune) {
		return nil
	}
	title := strings.TrimRight(prefix, delimiters)
	if len(title) < 4 {
		return nil
	}
	shortest := len(names[0])
	for _, n := range names {
		if len(n) < shortest {
			shortest = len(n)
		}
	}
	if len(title)*2 < shortest {
		return nil
	}

	// Every file must differ from the title by a distinct, non-empty suffix.
	tails := map[string]bool{}
	for _, n := range names {
		if len(n) <= len(prefix) {
			return nil
		}
		tail := n[len(prefix):]
		if tail == "" || tails[tail] {
			return nil
		}
		tails[tail] = true
	}

	sorted := make([]string, len(videos))
	for i, v := range videos {
		sorted[i] = v.ID
	}
	sort.Slice(sorted, func(i, j int) bool {
		return names[i] < names[j]
	})
	members := make([]dirMember, 0, len(videos))
	for i, id := range sorted {
		members = append(members, dirMember{videoID: id, ep: i + 1})
	}
	return &dirGroup{show: title, seasons: map[int][]dirMember{1: members}}
}

const delimiters = "：:·—-_ "

func isDelimiter(r rune) bool {
	return strings.ContainsRune(delimiters, r)
}

// lcp returns the longest common prefix of the given strings ("" for none).
func lcp(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	base := strs[0]
	for i := 1; i < len(strs); i++ {
		j := 0
		for j < len(base) && j < len(strs[i]) && base[j] == strs[i][j] {
			j++
		}
		base = base[:j]
		if base == "" {
			break
		}
	}
	return base
}

// reconcileGroups is the second scan phase: after every video is imported
// (phase 1), it walks the directories that hold ≥2 videos and groups those
// where every video fits one strict pattern. Assignment is additive only —
// videos are grouped when their directory shows an obvious pattern, never
// un-grouped, so future manual grouping by the user is never undone. Empty
// shows/seasons are cleaned up by DB triggers when their videos disappear.
func (s *Service) reconcileGroups(ctx context.Context) {
	all, err := s.videos.ListAll(ctx)
	if err != nil {
		slog.Warn("reconcile groups list all", "err", err)
		return
	}
	byID := make(map[string]domain.Video, len(all))
	byDir := make(map[string][]domain.Video)
	for _, v := range all {
		byID[v.ID] = v
		d := filepath.Dir(v.Path)
		byDir[d] = append(byDir[d], v)
	}

	for dir, videos := range byDir {
		if len(videos) < 2 {
			continue
		}
		g := classifyDir(dir, videos)
		if g == nil {
			continue
		}
		for season, members := range g.seasons {
			s.assignSeason(ctx, g.show, season, members, byID)
		}
	}
}

// assignSeason groups one season/part's members under a show in a single
// transaction, skipping the write when every member is already assigned to the
// same season and position (so unchanged series and their titles are preserved).
func (s *Service) assignSeason(ctx context.Context, showName string, season int, members []dirMember, byID map[string]domain.Video) {
	if len(members) == 0 {
		return
	}
	changed := false
	for _, m := range members {
		v, ok := byID[m.videoID]
		if !ok {
			continue
		}
		if v.Kind != "episode" || v.ShowID == "" || v.SeasonNumber != season || v.EpisodeNumber != m.ep {
			changed = true
			break
		}
	}
	if !changed {
		return
	}

	assigns := make([]domain.EpisodeAssign, 0, len(members))
	for _, m := range members {
		assigns = append(assigns, domain.EpisodeAssign{
			VideoID:       m.videoID,
			EpisodeNumber: m.ep,
			Title:         titleFromPath(byID[m.videoID].RelativePath),
		})
	}
	showID, err := s.shows.AssignSeason(ctx, showName, season, assigns)
	if err != nil {
		slog.Warn("assign season", "show", showName, "season", season, "err", err)
		return
	}
	if err := s.series.SyncShowLinks(ctx, showID); err != nil {
		slog.Warn("sync series links", "show", showID, "err", err)
	}
}
