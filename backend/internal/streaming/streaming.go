package streaming

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"homereel/backend/internal/domain"
)

// Errors surfaced to the API layer.
var (
	// ErrUnavailable reports that the source storage is offline / file gone.
	ErrUnavailable = errors.New("storage unavailable")
	// ErrNotFound reports a missing cache or sidecar resource.
	ErrNotFound = errors.New("not found")
)

// Service streams indexed videos (ADR-006): direct HTTP Range for browser
// native formats, on-demand HLS transcoding (single-flight, cached) otherwise,
// plus cover/thumb and sidecar subtitle serving.
type Service struct {
	videos     domain.VideoRepo
	dataDir    string
	ffmpegPath string
	enableHLS  string // auto | true | false
	hlsPreset  string
	hlsDir     string
	remuxDir   string

	mu     sync.Mutex
	active map[string]*transcode
}

// New builds the streaming service. dataDir hosts covers/ and the hls/ and
// remux/ caches.
func New(videos domain.VideoRepo, dataDir, ffmpegPath, enableHLS, hlsPreset string) *Service {
	return &Service{
		videos:     videos,
		dataDir:    dataDir,
		ffmpegPath: ffmpegPath,
		enableHLS:  enableHLS,
		hlsPreset:  hlsPreset,
		hlsDir:     filepath.Join(dataDir, "hls"),
		remuxDir:   filepath.Join(dataDir, "remux"),
		active:     make(map[string]*transcode),
	}
}

// DirectPlayable reports whether the browser can play the file natively via
// HTTP Range (ADR-006 first layer). Containers/codecs outside the native set
// must go through HLS transcoding. Older probe rows may store a comma-separated
// format_name (e.g. "mov,mp4,..."), so container matching splits on commas.
func (s *Service) DirectPlayable(v domain.Video) bool {
	if nativeCodecs[v.Codec] {
		return containerNative(v.Container)
	}
	// Unknown codec: fall back to extension-based guess so unprobed files
	// still play in the common case.
	if v.Container != "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(v.Path))
	switch ext {
	case ".mp4", ".m4v", ".mov", ".webm", ".ogv", ".ogg":
		return true
	}
	return false
}

// HLSEnabled decides whether on-demand transcoding is active for a video.
func (s *Service) HLSEnabled(v domain.Video) bool {
	switch s.enableHLS {
	case "true":
		return true
	case "false":
		return false
	default: // auto
		return !s.DirectPlayable(v)
	}
}

// Direct serves the source file with HTTP Range support. Segmented MP4 files
// (hls.js-assembled, detected at probe time) are served from their remuxed
// faststart copy when one has been produced — otherwise the raw source is
// served and the browser may download it in full before playing (acceptable as
// a fallback; the user can request a remux to fix it).
func (s *Service) Direct(w http.ResponseWriter, r *http.Request, v domain.Video) error {
	path := v.Path
	if v.Segmented {
		if remuxed, err := s.remuxed(v.ID); err == nil {
			path = remuxed
		}
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrUnavailable
		}
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType(v))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	return nil
}

// remuxed returns the remuxed faststart copy path for a video when it exists.
func (s *Service) remuxed(videoID string) (string, error) {
	p := filepath.Join(s.remuxDir, videoID+".mp4")
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// Remuxed reports whether a remuxed copy is available for a video (used by the
// remux management API to show per-file state).
func (s *Service) Remuxed(videoID string) bool {
	_, err := s.remuxed(videoID)
	return err == nil
}

// Cover serves the generated cover or thumb image from data_dir.
func (s *Service) Cover(w http.ResponseWriter, r *http.Request, v domain.Video, thumb bool) error {
	rel := v.CoverPath
	if thumb {
		rel = v.ThumbPath
	}
	if rel == "" {
		return ErrNotFound
	}
	p := filepath.Join(s.dataDir, filepath.FromSlash(rel))
	if !within(s.dataDir, p) {
		return ErrNotFound
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	http.ServeContent(w, r, filepath.Base(p), info.ModTime(), f)
	return nil
}

// MasterM3U8 ensures the on-demand transcode for v is running (starting it if
// needed) and serves its playlist once it contains playable segments. The
// playlist is read into memory and served as a snapshot (ffmpeg rewrites the
// file in place, so streaming the live file can race a partial write) with
// Cache-Control: no-store so a stale cached copy never stalls hls.js.
func (s *Service) MasterM3U8(w http.ResponseWriter, r *http.Request, v domain.Video) error {
	dir := filepath.Join(s.hlsDir, v.ID)
	playlist := filepath.Join(dir, "master.m3u8")
	serveMaster := func() (bool, error) {
		data, err := os.ReadFile(playlist)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		if !bytes.Contains(data, []byte("#EXTINF")) {
			// Playlist exists but has no segments yet — not ready.
			return false, nil
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(data)
		return true, err
	}

	s.mu.Lock()
	tr, running := s.active[v.ID]
	if !running {
		if hlsComplete(dir) {
			s.mu.Unlock()
			_, err := serveMaster()
			return err
		}
		// A stale partial transcode (previous process died mid-run) has no
		// ENDLIST marker; discard it and transcode fresh.
		_ = os.RemoveAll(dir)
		tctx, cancel := context.WithCancel(context.Background())
		tr = &transcode{cancel: cancel, done: make(chan struct{})}
		s.active[v.ID] = tr
		s.mu.Unlock()
		go s.runTranscode(tctx, v, dir, tr.done)
	} else {
		s.mu.Unlock()
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case <-tr.done:
			if _, err := os.Stat(playlist); err != nil {
				return ErrUnavailable
			}
			_, err := serveMaster()
			return err
		case <-ticker.C:
			ready, err := serveMaster()
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

var segmentRe = regexp.MustCompile(`^segment-\d{5}\.ts$`)

// Segment serves a cached HLS segment produced by the transcode.
func (s *Service) Segment(w http.ResponseWriter, r *http.Request, v domain.Video, name string) error {
	if !segmentRe.MatchString(name) {
		return ErrNotFound
	}
	p := filepath.Join(s.hlsDir, v.ID, name)
	if !within(s.hlsDir, p) {
		return ErrNotFound
	}
	return serveFile(w, r, p, "video/mp2t")
}

// Subtitle serves a sidecar subtitle (.srt/.vtt/.ass) sitting next to the
// source file. Embedded-track extraction is a later enhancement.
func (s *Service) Subtitle(w http.ResponseWriter, r *http.Request, v domain.Video) error {
	base := strings.TrimSuffix(filepath.Base(v.Path), filepath.Ext(v.Path))
	dir := filepath.Dir(v.Path)
	types := []struct {
		ext  string
		mime string
	}{
		{".vtt", "text/vtt; charset=utf-8"},
		{".srt", "text/plain; charset=utf-8"},
		{".ass", "text/plain; charset=utf-8"},
		{".ssa", "text/plain; charset=utf-8"},
	}
	for _, t := range types {
		p := filepath.Join(dir, base+t.ext)
		if _, err := os.Stat(p); err == nil {
			return serveFile(w, r, p, t.mime)
		}
	}
	return ErrNotFound
}

// RemoveCache deletes a video's HLS transcode and remuxed copy (called when the
// video is deleted or its file changes so stale caches are never served).
func (s *Service) RemoveCache(videoID string) {
	s.mu.Lock()
	if tr, ok := s.active[videoID]; ok {
		tr.cancel()
		delete(s.active, videoID)
	}
	s.mu.Unlock()
	_ = os.RemoveAll(filepath.Join(s.hlsDir, videoID))
	_ = os.Remove(filepath.Join(s.remuxDir, videoID+".mp4"))
	_ = os.Remove(filepath.Join(s.remuxDir, videoID+".mp4.tmp"))
}

type transcode struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// runTranscode transcodes the whole source into an HLS playlist in dir,
// removing itself from the single-flight map when done (success or failure).
// On success it writes a .done marker so a restart can tell complete from
// partial caches.
func (s *Service) runTranscode(ctx context.Context, v domain.Video, dir string, done chan struct{}) {
	defer close(done)
	defer func() {
		s.mu.Lock()
		delete(s.active, v.ID)
		s.mu.Unlock()
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("hls mkdir", "video_id", v.ID, "err", err)
		return
	}
	preset := s.hlsPreset
	if preset == "" {
		preset = "fast"
	}
	cmd := newTranscodeCommand(ctx, s.ffmpegPath, v.Path,
		filepath.Join(dir, "segment-%05d.ts"), filepath.Join(dir, "master.m3u8"), preset)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("hls transcode failed",
				"video_id", v.ID, "err", err, "output", truncate(string(out), 500))
		}
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "master.m3u8")); err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, ".done"), []byte("ok\n"), 0o644); err != nil {
		slog.Warn("hls done marker", "video_id", v.ID, "err", err)
	}
}

// hlsComplete reports whether a cache dir holds a fully transcoded playlist.
func hlsComplete(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "master.m3u8")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".done"))
	return err == nil
}

func serveFile(w http.ResponseWriter, r *http.Request, path, contentType string, noCache ...bool) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	if len(noCache) > 0 && noCache[0] {
		w.Header().Set("Cache-Control", "no-store")
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	return nil
}

// nativeContainers / nativeCodecs describe what browsers play via Range.
var (
	nativeContainers = map[string]bool{
		"mp4": true, "m4v": true, "mov": true, "qt": true,
		"webm": true, "ogg": true, "ogv": true,
	}
	nativeCodecs = map[string]bool{
		"h264": true, "avc1": true, "avc3": true,
		"vp8": true, "vp9": true, "av1": true, "theora": true,
	}
)

// contentType maps a video to the Content-Type used for Range serving. The file
// extension wins because ffprobe reports the whole MP4 family as "mov,mp4,..."
// (the MOV demuxer handles MP4), so demuxer-token matching first would mislabel
// real .mp4 files as video/quicktime — desktop browsers then refuse to treat the
// media as MP4 and stall buffering. The probed container is only a fallback for
// files whose extension is unrecognized.
func contentType(v domain.Video) string {
	switch strings.ToLower(filepath.Ext(v.Path)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	}
	for _, c := range strings.Split(strings.ToLower(v.Container), ",") {
		c = strings.TrimSpace(c)
		if t, ok := containerTypes[c]; ok {
			return t
		}
	}
	return "application/octet-stream"
}

var containerTypes = map[string]string{
	"mp4": "video/mp4", "m4v": "video/mp4",
	"mov": "video/quicktime", "qt": "video/quicktime",
	"matroska": "video/x-matroska", "mkv": "video/x-matroska",
	"webm": "video/webm",
	"avi":  "video/x-msvideo",
	"wmv":  "video/x-ms-wmv",
	"flv":  "video/x-flv",
	"mpeg": "video/mp2t", "mpg": "video/mp2t", "mpegts": "video/mp2t",
	"ts": "video/mp2t", "m2ts": "video/mp2t",
	"ogg": "video/ogg", "ogv": "video/ogg",
	"3gp": "video/3gpp",
}

// containerNative reports whether any comma-separated container token is in
// the browser-native set.
func containerNative(c string) bool {
	for _, part := range strings.Split(c, ",") {
		if nativeContainers[strings.TrimSpace(part)] {
			return true
		}
	}
	return false
}

func within(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
