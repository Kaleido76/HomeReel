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

	"videomesh/backend/internal/domain"
	"videomesh/backend/internal/events"
	"videomesh/backend/internal/files"
	"videomesh/backend/internal/jobs"
	"videomesh/backend/internal/media"
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
func New(videos domain.VideoRepo, storages domain.StorageRepo, jobsSvc *jobs.Service,
	filesSvc *files.Service, bus *events.Bus, ffprobePath, ffmpegPath, dataDir string) *Service {
	return &Service{
		videos:      videos,
		storages:    storages,
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
				res.Unchanged++
			} else {
				_ = s.videos.UpdateFingerprint(ctx, cur.ID, c.path, c.rel, c.size, c.mtime, scanStart)
				s.enqueueProbe(ctx, cur.ID)
				res.Updated++
			}
			continue
		}
		if moved, ok := byFile[c.fileID]; ok {
			// Same file moved/renamed: keep the record, update the path.
			_ = s.videos.UpdateFingerprint(ctx, moved.ID, c.path, c.rel, c.size, c.mtime, scanStart)
			if moved.Size != c.size || moved.MTime != c.mtime || needsProbe(moved) {
				s.enqueueProbe(ctx, moved.ID)
			}
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
		s.enqueueProbe(ctx, v.ID)
		res.Added++
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
