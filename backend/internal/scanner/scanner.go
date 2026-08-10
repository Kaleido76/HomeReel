package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Service owns scanning, importing and job handling (ADR-012): everything that
// turns files on disk into indexed videos. Videos enter the library through
// media sources — lightweight user-declared scan units — rather than managed
// storage volumes.
type Service struct {
	mu          sync.Mutex // 库写长任务串行化：scan/mark/probe/sync 互斥执行
	videos      domain.VideoRepo
	sources     domain.SourceRepo
	shows       domain.ShowRepo
	series      domain.SeriesRepo
	jobs        *jobs.Service
	bus         *events.Bus
	ffprobePath string
	ffmpegPath  string
	coversDir   string
	thumbsDir   string
	probe       ProbeFn
	thumbnail   ThumbnailFn
	now         func() time.Time
}

// New builds the scanner service. dataDir hosts covers/ and thumbs/.
func New(videos domain.VideoRepo, sources domain.SourceRepo,
	shows domain.ShowRepo, seriesRepo domain.SeriesRepo, jobsSvc *jobs.Service, bus *events.Bus,
	ffprobePath, ffmpegPath, dataDir string) *Service {
	return &Service{
		videos:      videos,
		sources:     sources,
		shows:       shows,
		series:      seriesRepo,
		jobs:        jobsSvc,
		bus:         bus,
		ffprobePath: ffprobePath,
		ffmpegPath:  ffmpegPath,
		coversDir:   filepath.Join(dataDir, "covers"),
		thumbsDir:   filepath.Join(dataDir, "thumbs"),
		probe:       media.Probe,
		thumbnail:   media.Thumbnail,
		now:         time.Now,
	}
}

// ScanResult summarises one scan pass.
type ScanResult struct {
	SourceID  string `json:"source_id"`
	Added     int    `json:"added"`
	Updated   int    `json:"updated"`
	Unchanged int    `json:"unchanged"`
	Missing   int    `json:"missing"`
}

// ScanSource performs a full fingerprint pass over a media source (ADR-007 /
// ADR-012). If the source root is unreachable the scan aborts without touching
// the library, so an unplugged drive never loses metadata. Descendant source
// roots are skipped entirely (the routing table: a child source owns its
// subtree). Files are matched globally by file_id, so a file moved between
// sources keeps its identity.
func (s *Service) ScanSource(ctx context.Context, src domain.MediaSource) (ScanResult, error) {
	return s.scan(ctx, src, nil, nil)
}

// subtaskFn reports the current serial sub-task: a status line (replaced in
// place by the frontend) and an optional within-file percentage in [0,100]
// (negative means unknown).
type subtaskFn func(text string, pct float64)

func (s *Service) scan(ctx context.Context, src domain.MediaSource, progress func(done, total int), subtask subtaskFn) (ScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res ScanResult
	res.SourceID = src.ID
	if !pathReachable(src.Path, 3*time.Second) {
		return res, fmt.Errorf("media source root unreachable: %s", src.Path)
	}
	scanStart := s.now().UTC().Format(domain.TimeLayout)

	// Routing table: skip the subtrees of any descendant source during this
	// scan — those directories are scanned by their own source.
	all, err := s.sources.List(ctx)
	if err != nil {
		return res, err
	}
	skipSet := map[string]bool{}
	for _, o := range all {
		if o.ID != src.ID && files.UnderRoot(o.Path, src.Path) {
			skipSet[filepath.Clean(o.Path)] = true
		}
	}

	// Global file_id matching lets a file that moved into this source (from
	// another source or an unmanaged path) be recognised as the same video.
	existing, err := s.videos.ListAll(ctx)
	if err != nil {
		return res, err
	}
	byPath := make(map[string]domain.Video, len(existing))
	byFile := make(map[string]domain.Video, len(existing))
	for _, v := range existing {
		byPath[v.Path] = v
		byFile[v.FileID] = v
	}

	candidates, err := s.collect(src.Path, skipSet)
	if err != nil {
		return res, err
	}
	total := len(candidates)

	for i, c := range candidates {
		if progress != nil {
			progress(i, total)
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if cur, ok := byPath[c.path]; ok {
			if cur.Size == c.size && cur.MTime == c.mtime {
				_ = s.videos.Touch(ctx, cur.ID, scanStart)
				if needsProbe(cur) {
					s.processInline(ctx, cur.ID, subtask)
				}
				res.Unchanged++
			} else {
				_ = s.videos.UpdateFingerprint(ctx, cur.ID, src.ID, c.path, c.rel, c.size, c.mtime, scanStart)
				s.processInline(ctx, cur.ID, subtask)
				s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": cur.ID}})
				res.Updated++
			}
			continue
		}
		if moved, ok := byFile[c.fileID]; ok {
			// Same file moved/renamed (possibly across sources): keep the
			// record, update path and ownership.
			_ = s.videos.UpdateFingerprint(ctx, moved.ID, src.ID, c.path, c.rel, c.size, c.mtime, scanStart)
			if moved.Size != c.size || moved.MTime != c.mtime || needsProbe(moved) {
				s.processInline(ctx, moved.ID, subtask)
				s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": moved.ID}})
			}
			res.Updated++
			continue
		}

		// New video: probe first, then create the row with its full metadata in
		// one statement — a video never appears in the library half-filled. A
		// failed probe still creates a base row (self-healing on the next scan).
		v := domain.Video{
			ID:            ulid.Make().String(),
			SourceID:      src.ID,
			FileID:        c.fileID,
			RelativePath:  c.rel,
			Path:          c.path,
			Size:          c.size,
			MTime:         c.mtime,
			Kind:          "movie",
			Title:         titleFromPath(c.rel),
			CreatedAt:     scanStart,
			UpdatedAt:     scanStart,
			LastScannedAt: scanStart,
		}
		if subtask != nil {
			subtask("探测 "+filepath.Base(c.path), 5)
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
			slog.Warn("inline probe", "path", c.path, "err", perr)
		}
		if err := s.videos.Create(ctx, v); err != nil {
			slog.Warn("create video", "path", c.path, "err", err)
			continue
		}
		if subtask != nil {
			subtask("生成 "+filepath.Base(c.path)+" 的缩略图", 60)
		}
		s.thumbnailFor(ctx, v.ID, c.path, v.Duration)
		if subtask != nil {
			subtask("", -1)
		}
		res.Added++
	}

	// Only prune rows this source owns that were not seen, and never rows that
	// now physically sit under a child source (the child claims those).
	var missingIDs []string
	missingIDs, err = s.videos.MarkMissingBySource(ctx, src.ID, scanStart, childRootsOf(all, src.ID, src.Path))
	if err != nil {
		return res, err
	}
	for _, id := range missingIDs {
		s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": id}})
	}
	res.Missing = len(missingIDs)

	// Phase 2: maintain membership of existing series (bind files that now sit
	// under a series root, detach files that no longer do, prune empty series).
	// Scans never create series — only manual marking and explicit syncs do.
	if err := s.maintainSeriesMembers(ctx); err != nil {
		slog.Warn("maintain series members", "err", err)
	}

	_ = s.sources.TouchLastScan(ctx, src.ID, s.now().UTC().Format(domain.TimeLayout))
	return res, nil
}

// childRootsOf lists the roots of descendant sources (the routing skip set) so
// MarkMissing can leave those rows alone.
func childRootsOf(all []domain.MediaSource, selfID, root string) []string {
	var out []string
	for _, o := range all {
		if o.ID != selfID && files.UnderRoot(o.Path, root) {
			out = append(out, o.Path)
		}
	}
	return out
}

// processInline re-probes an existing video and regenerates its cover/thumb
// synchronously, reporting each step as a serial sub-task of the enclosing
// scan. Per-file failure is non-fatal: metadata stays and the next scan
// retries. The scan runs these strictly serially so the host never saturates
// under a burst of ffprobe/ffmpeg processes.
func (s *Service) processInline(ctx context.Context, videoID string, subtask subtaskFn) {
	v, err := s.videos.Get(ctx, videoID)
	if err != nil {
		slog.Warn("inline get video", "video_id", videoID, "err", err)
		return
	}
	name := filepath.Base(v.Path)

	if subtask != nil {
		subtask("探测 "+name, 5)
	}
	info, err := s.probe(ctx, s.ffprobePath, v.Path)
	if err != nil {
		slog.Warn("inline probe", "video_id", videoID, "err", err)
		return
	}
	upd := v
	upd.Title = titleFromPath(v.RelativePath)
	upd.Duration = info.Duration
	upd.Codec = info.Codec
	upd.AudioCodec = info.AudioCodec
	upd.Container = info.Container
	upd.Segmented = info.Segmented
	upd.FastStart = info.FastStart
	upd.Width = info.Width
	upd.Height = info.Height
	if err := s.videos.UpdateProbe(ctx, upd); err != nil {
		slog.Warn("inline update probe", "video_id", videoID, "err", err)
		return
	}

	if subtask != nil {
		subtask("生成 "+name+" 的缩略图", 60)
	}
	s.thumbnailFor(ctx, v.ID, v.Path, info.Duration)
	if subtask != nil {
		subtask("", -1)
	}
}

// thumbnailFor generates and records the cover/thumb for a video (best effort).
func (s *Service) thumbnailFor(ctx context.Context, videoID, src string, duration float64) {
	cover := filepath.Join(s.coversDir, videoID+".jpg")
	thumb := filepath.Join(s.thumbsDir, videoID+".thumb.jpg")
	if err := s.thumbnail(ctx, s.ffmpegPath, src, cover, thumb, duration); err != nil {
		slog.Warn("inline thumbnail", "video_id", videoID, "err", err)
		return
	}
	relCover := filepath.ToSlash(filepath.Join("covers", videoID+".jpg"))
	relThumb := filepath.ToSlash(filepath.Join("thumbs", videoID+".thumb.jpg"))
	if err := s.videos.UpdateCovers(ctx, videoID, relCover, relThumb); err != nil {
		slog.Warn("inline update covers", "video_id", videoID, "err", err)
	}
}

// HandleJob is the jobs.Worker handler for probe/thumbnail/scan_source/remux/mark_resource jobs.
func (s *Service) HandleJob(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	switch j.Type {
	case jobs.TypeProbe:
		return s.handleProbe(ctx, j, report)
	case jobs.TypeThumbnail:
		return s.handleThumbnail(ctx, j, report)
	case jobs.TypeScanSource:
		return s.handleScanSource(ctx, j, report)
	case jobs.TypeMarkResource:
		return s.handleMarkResource(ctx, j, report)
	}
	return fmt.Errorf("unknown job type %q", j.Type)
}

// EnqueueScanSource schedules a full scan of a media source as a user-facing
// long task.
func (s *Service) EnqueueScanSource(ctx context.Context, sourceID string) (string, error) {
	src, err := s.sources.Get(ctx, sourceID)
	if err != nil {
		return "", err
	}
	extra, _ := json.Marshal(map[string]string{"source_id": sourceID})
	return s.jobs.Enqueue(ctx, jobs.TypeScanSource, sourceID,
		"扫描多媒体源 · "+filepath.Base(filepath.Clean(src.Path)), string(extra))
}

func (s *Service) handleScanSource(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	var meta struct {
		SourceID string `json:"source_id"`
	}
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.SourceID == "" {
		return errors.New("scan source job missing source_id")
	}
	src, err := s.sources.Get(ctx, meta.SourceID)
	if err != nil {
		return err
	}
	res, err := s.scan(ctx, src,
		func(done, total int) {
			if total > 0 {
				report.Progress(float64(done) / float64(total))
			}
		},
		func(text string, pct float64) {
			report.Subtask(text)
			if pct >= 0 {
				report.SubtaskProgress(pct)
			}
		})
	if err != nil {
		return err
	}
	slog.Info("media source scan done",
		"source_id", src.ID, "added", res.Added,
		"updated", res.Updated, "missing", res.Missing)
	return nil
}

// EnqueueThumbnail schedules cover/thumb generation for a video (internal).
func (s *Service) EnqueueThumbnail(ctx context.Context, videoID string) error {
	extra, _ := json.Marshal(map[string]string{"video_id": videoID})
	_, err := s.jobs.EnqueueInternal(ctx, jobs.TypeThumbnail, videoID, string(extra))
	return err
}

// EnqueueProbe schedules a probe job for a video (internal, manual refresh).
func (s *Service) EnqueueProbe(ctx context.Context, videoID string) error {
	extra, _ := json.Marshal(map[string]string{"video_id": videoID})
	_, err := s.jobs.EnqueueInternal(ctx, jobs.TypeProbe, videoID, string(extra))
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (s *Service) handleProbe(ctx context.Context, j jobs.Job, _ jobs.Reporter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	upd.AudioCodec = info.AudioCodec
	upd.Container = info.Container
	upd.Segmented = info.Segmented
	upd.FastStart = info.FastStart
	upd.Width = info.Width
	upd.Height = info.Height
	if err := s.videos.UpdateProbe(ctx, upd); err != nil {
		return err
	}
	s.maintainSeriesMembers(ctx)
	s.bus.Publish(events.Event{Type: events.VideoImported, Data: map[string]string{"video_id": v.ID}})
	return nil
}

func (s *Service) handleThumbnail(ctx context.Context, j jobs.Job, _ jobs.Reporter) error {
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
	s.thumbnailFor(ctx, v.ID, v.Path, v.Duration)
	return nil
}

// SourceStatus describes whether a video's source file is still findable.
type SourceStatus struct {
	Status string `json:"status"` // ok | moved | missing
	Path   string `json:"path,omitempty"`
}

// CheckVideo inspects a single video's source file (single-episode detail
// check): it first stats the recorded path, and when the path is gone it walks
// the owning media source looking for the file by file_id — a match means the
// file was renamed/moved (syncable), no match means it is gone.
func (s *Service) CheckVideo(ctx context.Context, videoID string) (SourceStatus, error) {
	v, err := s.videos.Get(ctx, videoID)
	if err != nil {
		return SourceStatus{}, err
	}
	if _, err := os.Stat(v.Path); err == nil {
		return SourceStatus{Status: "ok"}, nil
	}
	if v.SourceID == "" {
		return SourceStatus{Status: "missing"}, nil
	}
	src, err := s.sources.Get(ctx, v.SourceID)
	if err != nil {
		return SourceStatus{Status: "missing"}, nil
	}
	cands, err := s.collect(src.Path, nil)
	if err != nil {
		return SourceStatus{Status: "missing"}, nil
	}
	for _, c := range cands {
		if c.fileID == v.FileID {
			return SourceStatus{Status: "moved", Path: c.path}, nil
		}
	}
	return SourceStatus{Status: "missing"}, nil
}

// ErrVideoMissing reports that a video's source file could not be found
// anywhere in its media source.
var ErrVideoMissing = errors.New("video source file not found")

// SyncVideo re-locates a single video's source file by file_id inside its media
// source (handles a rename/move) and then converges its series membership. It
// returns ErrVideoMissing when the file is gone.
func (s *Service) SyncVideo(ctx context.Context, videoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.videos.Get(ctx, videoID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(v.Path); err == nil {
		s.maintainSeriesMembers(ctx)
		return nil
	}
	if v.SourceID == "" {
		return ErrVideoMissing
	}
	src, err := s.sources.Get(ctx, v.SourceID)
	if err != nil {
		return err
	}
	cands, err := s.collect(src.Path, nil)
	if err != nil {
		return err
	}
	for _, c := range cands {
		if c.fileID != v.FileID {
			continue
		}
		now := s.now().UTC().Format(domain.TimeLayout)
		if err := s.videos.UpdateFingerprint(ctx, v.ID, src.ID, c.path, c.rel, c.size, c.mtime, now); err != nil {
			return err
		}
		s.processInline(ctx, v.ID, nil)
		s.bus.Publish(events.Event{Type: events.VideoUpdated, Data: map[string]string{"video_id": v.ID}})
		if err := s.maintainSeriesMembers(ctx); err != nil {
			return err
		}
		return nil
	}
	return ErrVideoMissing
}

// SeriesCheck describes how a series' folder compares to its database members.
type SeriesCheck struct {
	RootExists bool     `json:"root_exists"`
	OutOfSync  bool     `json:"out_of_sync"`
	Missing    []string `json:"missing,omitempty"` // 磁盘上已消失的成员（相对路径）
	New        []string `json:"new,omitempty"`     // 文件夹里尚未入库的文件（相对路径）
}

// CheckSeries compares a series' root folder against its database members
// (series detail check): root existence, members still on disk, and any new
// files that have not been indexed yet.
func (s *Service) CheckSeries(ctx context.Context, seriesID string) (SeriesCheck, error) {
	check := SeriesCheck{RootExists: true}
	se, err := s.series.Get(ctx, seriesID)
	if err != nil {
		return check, err
	}
	if se.RootPath == "" {
		check.OutOfSync = true
		check.RootExists = false
		return check, nil
	}
	if _, err := os.Stat(se.RootPath); err != nil {
		check.RootExists = false
		check.OutOfSync = true
		check.Missing = append(check.Missing, "系列根目录已不存在")
		return check, nil
	}
	members, err := s.series.GetMembers(ctx, seriesID)
	if err != nil {
		return check, err
	}
	// 成员的 relative_path 基准不固定（标记入库=文件名；扫描入库=相对媒体源
	// 根的完整路径）。成员必是系列根目录直接子文件，因此用文件名（Base）作磁盘
	// 对比键，两边一致。
	onDisk := map[string]bool{}
	for _, m := range members {
		name := filepath.Base(m.RelativePath)
		abs := filepath.Join(se.RootPath, name)
		if _, err := os.Stat(abs); err != nil {
			check.Missing = append(check.Missing, name)
		}
		onDisk[name] = true
	}
	cands, err := s.collectDirectChildren(se.RootPath)
	if err != nil {
		return check, err
	}
	for _, c := range cands {
		if !onDisk[c.rel] {
			check.New = append(check.New, c.rel)
		}
	}
	check.OutOfSync = len(check.Missing) > 0 || len(check.New) > 0
	return check, nil
}

// SyncSeries re-runs the manual "add series" logic for an existing series: it
// imports new direct children, binds them in file-name order, removes vanished
// members, and prunes empty series (series-level sync).
func (s *Service) SyncSeries(ctx context.Context, seriesID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, err := s.series.Get(ctx, seriesID)
	if err != nil {
		return err
	}
	if se.RootPath == "" {
		return errors.New("series has no root path")
	}
	if _, err := os.Stat(se.RootPath); err != nil {
		return fmt.Errorf("系列根目录不存在: %w", err)
	}
	srcID, ok := s.containingSource(ctx, se.RootPath)
	if !ok {
		return errors.New("系列根目录不在任何媒体源内")
	}
	cands, err := s.collectDirectChildren(se.RootPath)
	if err != nil {
		return err
	}
	ids, err := s.importCandidates(ctx, cands, srcID, nil)
	if err != nil {
		return err
	}
	_, err = s.syncSeriesFolder(ctx, se.RootPath, cands, ids, false)
	return err
}

type candidate struct {
	path   string
	rel    string
	fileID string
	size   int64
	mtime  int64
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
