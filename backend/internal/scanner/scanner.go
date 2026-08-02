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
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/media"
)

// ProbeFn and ThumbnailFn are injectable for tests.
type ProbeFn func(ctx context.Context, ffprobePath, path string) (media.Info, error)
type ThumbnailFn func(ctx context.Context, ffmpegPath, src, cover, thumb string, duration float64) error

// timeLayout matches the fixed-width timestamp used across the data layer so
// that lexicographic recency comparisons (scan vs last_scanned_at) are safe.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Service owns scanning, importing and job handling (ADR-012): everything that
// turns files on disk into indexed videos.
type Service struct {
	videos      domain.VideoRepo
	storages    domain.StorageRepo
	shows       domain.ShowRepo
	series      domain.SeriesRepo
	jobs        *jobs.Service
	files       *files.Service
	bus         *events.Bus
	ffprobePath string
	ffmpegPath  string
	coversDir   string
	thumbsDir   string
	debounce    time.Duration
	probe       ProbeFn
	thumbnail   ThumbnailFn
	now         func() time.Time
}

// New builds the scanner service. dataDir hosts covers/ and thumbs/ output.
func New(videos domain.VideoRepo, storages domain.StorageRepo, shows domain.ShowRepo,
	seriesRepo domain.SeriesRepo, jobsSvc *jobs.Service, filesSvc *files.Service,
	bus *events.Bus, ffprobePath, ffmpegPath, dataDir string) *Service {
	return &Service{
		videos:      videos,
		storages:    storages,
		shows:       shows,
		series:      seriesRepo,
		jobs:        jobsSvc,
		files:       filesSvc,
		bus:         bus,
		ffprobePath: ffprobePath,
		ffmpegPath:  ffmpegPath,
		coversDir:   filepath.Join(dataDir, "covers"),
		thumbsDir:   filepath.Join(dataDir, "thumbs"),
		debounce:    5 * time.Second,
		probe:       media.Probe,
		thumbnail:   media.Thumbnail,
		now:         time.Now,
	}
}

// ScanResult summarises one scan pass.
type ScanResult struct {
	StorageID string `json:"storage_id"`
	Added     int    `json:"added"`
	Updated   int    `json:"updated"`
	Unchanged int    `json:"unchanged"`
	Missing   int    `json:"missing"`
}

// Scan performs a full fingerprint pass over a storage volume (ADR-007 /
// ADR-012). If the volume root is unreachable the scan aborts without marking
// anything missing, so an unplugged drive never loses metadata (ADR-014).
func (s *Service) Scan(ctx context.Context, st domain.Storage) (ScanResult, error) {
	var res ScanResult
	res.StorageID = st.ID
	if !pathReachable(st.RootPath, 3*time.Second) {
		return res, fmt.Errorf("storage root unreachable: %s", st.RootPath)
	}
	scanStart := s.now().UTC().Format(timeLayout)

	existing, err := s.videos.ListByStorage(ctx, st.ID)
	if err != nil {
		return res, err
	}
	byPath := make(map[string]domain.Video, len(existing))
	byFile := make(map[string]domain.Video, len(existing))
	for _, v := range existing {
		byPath[v.RelativePath] = v
		byFile[v.FileID] = v
	}

	candidates, err := s.collect(st.RootPath)
	if err != nil {
		return res, err
	}

	// Videos whose grouping may have changed are re-grouped in one pass after
	// every candidate is indexed, so same-directory siblings are visible when
	// deciding whether a file belongs to a series.
	toGroup := []string{}

	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if cur, ok := byPath[c.rel]; ok {
			if cur.Size == c.size && cur.MTime == c.mtime {
				_ = s.videos.Touch(ctx, cur.ID, scanStart)
				if needsProbe(cur) {
					s.enqueueProbe(ctx, cur.ID)
				}
				// Backfill grouping for videos indexed before the series
				// reorganization: rows defaulted to kind=movie whose path is
				// really a series member.
				if cur.Kind == "movie" {
					_, isPart := ParseMoviePart(cur.RelativePath)
					if ParseEpisode(cur.RelativePath).HasSE || isPart {
						toGroup = append(toGroup, cur.ID)
					}
				}
				res.Unchanged++
			} else {
				_ = s.videos.UpdateFingerprint(ctx, cur.ID, c.path, c.rel, c.size, c.mtime, scanStart)
				s.enqueueProbe(ctx, cur.ID)
				s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": cur.ID}})
				toGroup = append(toGroup, cur.ID)
				res.Updated++
			}
			continue
		}
		if moved, ok := byFile[c.fileID]; ok {
			// Same file moved/renamed: keep the record, update the path.
			_ = s.videos.UpdateFingerprint(ctx, moved.ID, c.path, c.rel, c.size, c.mtime, scanStart)
			if moved.Size != c.size || moved.MTime != c.mtime || needsProbe(moved) {
				s.enqueueProbe(ctx, moved.ID)
				s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": moved.ID}})
			}
			toGroup = append(toGroup, moved.ID)
			res.Updated++
			continue
		}
		v := domain.Video{
			ID:            ulid.Make().String(),
			StorageID:     st.ID,
			FileID:        c.fileID,
			RelativePath:  c.rel,
			Path:          c.path,
			Size:          c.size,
			MTime:         c.mtime,
			Title:         titleFromPath(c.rel),
			CreatedAt:     scanStart,
			UpdatedAt:     scanStart,
			LastScannedAt: scanStart,
		}
		if err := s.videos.Create(ctx, v); err != nil {
			slog.Warn("create video", "path", c.path, "err", err)
			continue
		}
		toGroup = append(toGroup, v.ID)
		s.enqueueProbe(ctx, v.ID)
		res.Added++
	}

	for _, id := range dedupe(toGroup) {
		s.groupVideo(ctx, id)
	}

	missing, err := s.videos.MarkMissing(ctx, st.ID, scanStart)
	if err != nil {
		return res, err
	}
	for _, id := range missing {
		s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": id}})
	}
	res.Missing = len(missing)
	return res, nil
}

// HandleJob is the jobs.Worker handler for probe/thumbnail/rescan jobs.
func (s *Service) HandleJob(ctx context.Context, j jobs.Job) error {
	switch j.Type {
	case jobs.TypeProbe:
		return s.handleProbe(ctx, j)
	case jobs.TypeThumbnail:
		return s.handleThumbnail(ctx, j)
	case jobs.TypeRescan:
		return s.handleRescan(ctx, j)
	}
	return fmt.Errorf("unknown job type %q", j.Type)
}

// EnqueueRescan schedules a full scan of a storage volume.
func (s *Service) EnqueueRescan(ctx context.Context, storageID string) error {
	extra, _ := json.Marshal(map[string]string{"storage_id": storageID})
	_, err := s.jobs.Enqueue(ctx, jobs.TypeRescan, "", string(extra))
	return err
}

// EnqueueThumbnail schedules cover/thumb generation for a video.
func (s *Service) EnqueueThumbnail(ctx context.Context, videoID string) error {
	extra, _ := json.Marshal(map[string]string{"video_id": videoID})
	_, err := s.jobs.Enqueue(ctx, jobs.TypeThumbnail, "", string(extra))
	return err
}

// EnqueueProbe schedules a probe job for a video (manual refresh).
func (s *Service) EnqueueProbe(ctx context.Context, videoID string) error {
	extra, _ := json.Marshal(map[string]string{"video_id": videoID})
	_, err := s.jobs.Enqueue(ctx, jobs.TypeProbe, "", string(extra))
	return err
}

// ImportUploaded indexes a freshly assembled upload and schedules its probe.
func (s *Service) ImportUploaded(ctx context.Context, st domain.Storage, absPath, relPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	fid, err := files.FileID(absPath)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(timeLayout)
	v := domain.Video{
		ID:            ulid.Make().String(),
		StorageID:     st.ID,
		FileID:        fmt.Sprintf("%d", fid),
		RelativePath:  filepath.ToSlash(relPath),
		Path:          absPath,
		Size:          info.Size(),
		MTime:         info.ModTime().UnixMilli(),
		Title:         titleFromPath(relPath),
		CreatedAt:     now,
		UpdatedAt:     now,
		LastScannedAt: now,
	}
	if err := s.videos.Create(ctx, v); err != nil {
		// Already indexed (re-upload): fall back to a rescan which re-probes.
		slog.Warn("create imported video", "path", absPath, "err", err)
		return s.EnqueueRescan(ctx, st.ID)
	}
	s.groupVideo(ctx, v.ID)
	s.enqueueProbe(ctx, v.ID)
	return nil
}

// Watch starts fsnotify monitoring for a storage volume and re-scans it
// (debounced) on file changes.
func (s *Service) Watch(ctx context.Context, st domain.Storage) error {
	if !pathReachable(st.RootPath, 3*time.Second) {
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := addTree(watcher, st.RootPath); err != nil {
		_ = watcher.Close()
		return err
	}
	go s.watchLoop(ctx, watcher, st)
	return nil
}

func (s *Service) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, st domain.Storage) {
	defer func() { _ = watcher.Close() }()
	timer := time.NewTimer(s.debounce)
	timer.Stop()
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = addTree(watcher, ev.Name)
				}
			}
			timer.Reset(s.debounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("watch error", "storage_id", st.ID, "err", err)
		case <-timer.C:
			if err := s.EnqueueRescan(context.Background(), st.ID); err != nil {
				slog.Warn("enqueue rescan", "storage_id", st.ID, "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) handleProbe(ctx context.Context, j jobs.Job) error {
	var meta struct {
		VideoID string `json:"video_id"`
	}
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.VideoID == "" {
		return errors.New("probe job missing video_id")
	}
	v, err := s.videos.Get(ctx, meta.VideoID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(v.Path); err != nil {
		return fmt.Errorf("source missing: %w", err)
	}
	info, err := s.probe(ctx, s.ffprobePath, v.Path)
	if err != nil {
		return err
	}
	upd := v
	upd.Title = titleFromPath(v.RelativePath)
	upd.Duration = info.Duration
	upd.Codec = info.Codec
	upd.Container = info.Container
	upd.Width = info.Width
	upd.Height = info.Height
	if err := s.videos.UpdateProbe(ctx, upd); err != nil {
		return err
	}
	s.groupVideo(ctx, v.ID)
	s.bus.Publish(events.Event{Type: events.VideoImported, Data: map[string]string{"video_id": v.ID}})
	return nil
}

func (s *Service) handleThumbnail(ctx context.Context, j jobs.Job) error {
	var meta struct {
		VideoID string `json:"video_id"`
	}
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.VideoID == "" {
		return errors.New("thumbnail job missing video_id")
	}
	v, err := s.videos.Get(ctx, meta.VideoID)
	if err != nil {
		return err
	}
	cover := filepath.Join(s.coversDir, v.ID+".jpg")
	thumb := filepath.Join(s.thumbsDir, v.ID+".thumb.jpg")
	if err := s.thumbnail(ctx, s.ffmpegPath, v.Path, cover, thumb, v.Duration); err != nil {
		return err
	}
	relCover := filepath.ToSlash(filepath.Join("covers", v.ID+".jpg"))
	relThumb := filepath.ToSlash(filepath.Join("thumbs", v.ID+".thumb.jpg"))
	return s.videos.UpdateCovers(ctx, v.ID, relCover, relThumb)
}

func (s *Service) handleRescan(ctx context.Context, j jobs.Job) error {
	var meta struct {
		StorageID string `json:"storage_id"`
	}
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.StorageID == "" {
		return errors.New("rescan job missing storage_id")
	}
	st, err := s.storages.Get(ctx, meta.StorageID)
	if err != nil {
		return err
	}
	res, err := s.Scan(ctx, st)
	if err != nil {
		return err
	}
	slog.Info("scan done",
		"storage_id", st.ID, "added", res.Added,
		"updated", res.Updated, "missing", res.Missing)
	return nil
}

func (s *Service) enqueueProbe(ctx context.Context, videoID string) {
	extra, _ := json.Marshal(map[string]string{"video_id": videoID})
	if _, err := s.jobs.Enqueue(ctx, jobs.TypeProbe, "", string(extra)); err != nil {
		slog.Warn("enqueue probe", "video_id", videoID, "err", err)
	}
}

// groupVideo decides whether a video belongs to a series (a season of a show
// or a part of a movie franchise) or stays standalone, and updates kind/show/
// season/episode accordingly. Scan results default to standalone unless there
// is a clear series relationship: an explicit Season folder, or several videos
// in the same directory sharing a similar title (see groupCount).
func (s *Service) groupVideo(ctx context.Context, videoID string) {
	v, err := s.videos.Get(ctx, videoID)
	if err != nil {
		slog.Warn("group video", "video_id", videoID, "err", err)
		return
	}

	var hint *EpisodeHint
	var kind string
	if h := ParseEpisode(v.RelativePath); h.HasSE {
		if inSeasonDir(v.RelativePath) || s.groupCount(ctx, v) >= 2 {
			hint = &h
			kind = "tv"
		}
	} else if h, ok := ParseMoviePart(v.RelativePath); ok {
		if s.groupCount(ctx, v) >= 2 {
			hint = &h
			kind = "movie"
		}
	}
	if hint == nil {
		if err := s.videos.AssignMovie(ctx, v.ID); err != nil {
			slog.Warn("assign standalone", "video_id", v.ID, "err", err)
		}
		return
	}

	show, err := s.shows.FindByName(ctx, hint.Show)
	if errors.Is(err, domain.ErrNotFound) {
		now := s.now().UTC().Format(timeLayout)
		show = domain.Show{
			ID:             ulid.Make().String(),
			Name:           hint.Show,
			MetadataSource: "manual",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.shows.Create(ctx, show); err != nil {
			slog.Warn("create show", "name", hint.Show, "err", err)
			return
		}
	} else if err != nil {
		slog.Warn("find show", "name", hint.Show, "err", err)
		return
	}
	if _, err := s.shows.EnsureSeason(ctx, show.ID, hint.Season, kind); err != nil {
		slog.Warn("ensure season", "show", show.ID, "season", hint.Season, "err", err)
		return
	}
	if err := s.videos.AssignEpisode(ctx, v.ID, show.ID, hint.Season, hint.Episode, titleFromPath(v.RelativePath)); err != nil {
		slog.Warn("assign episode", "video_id", v.ID, "err", err)
		return
	}
	if err := s.series.SyncShowLinks(ctx, show.ID); err != nil {
		slog.Warn("sync series links", "show", show.ID, "err", err)
	}
}

// groupCount returns how many videos in the same directory share this video's
// title key (exact match or small edit distance), so a lone file with an
// SxxEyy name stays standalone while a multi-part folder becomes a series.
func (s *Service) groupCount(ctx context.Context, v domain.Video) int {
	all, err := s.videos.ListByStorage(ctx, v.StorageID)
	if err != nil {
		return 0
	}
	dir := dirOf(v.RelativePath)
	keys := []string{}
	for _, o := range all {
		if o.ID == v.ID || dirOf(o.RelativePath) != dir {
			continue
		}
		if k := seriesKeyOf(o.RelativePath); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return 1
	}
	myKey := seriesKeyOf(v.RelativePath)
	for _, g := range clusterKeys(append(keys, myKey)) {
		if containsString(g, myKey) {
			return len(g)
		}
	}
	return 1
}

func dirOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// dedupe removes duplicate entries while preserving order.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// clusterKeys groups keys whose titles are equal (exact or edit-distance
// similar), used to detect a multi-part series in one directory.
func clusterKeys(keys []string) [][]string {
	groups := [][]string{}
	for _, k := range keys {
		placed := false
		for i := range groups {
			if sameTitle(groups[i][0], k) {
				groups[i] = append(groups[i], k)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []string{k})
		}
	}
	return groups
}

func sameTitle(a, b string) bool {
	if a == b {
		return true
	}
	if absInt(len([]rune(a))-len([]rune(b))) > 2 {
		return false
	}
	return editDistance(a, b) <= 2
}

// editDistance is the Levenshtein distance over runes (used for fuzzy title
// matching of loosely-similar episode files).
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type candidate struct {
	path   string
	rel    string
	fileID string
	size   int64
	mtime  int64
}

func (s *Service) collect(root string) ([]candidate, error) {
	var out []candidate
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || !files.IsVideo(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
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

func addTree(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if err := watcher.Add(path); err != nil {
				slog.Debug("watch add", "path", path, "err", err)
			}
		}
		return nil
	})
}

func titleFromPath(rel string) string {
	base := filepath.Base(rel)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// needsProbe reports whether a video has no probe metadata yet, meaning its
// probe job never succeeded (e.g. ffprobe was unavailable at scan time).
func needsProbe(v domain.Video) bool {
	return v.Duration == 0 && v.Codec == ""
}

func pathReachable(path string, timeout time.Duration) bool {
	done := make(chan struct{})
	var ok bool
	go func() {
		_, err := os.Stat(path)
		ok = err == nil
		close(done)
	}()
	select {
	case <-done:
		return ok
	case <-time.After(timeout):
		return false
	}
}
