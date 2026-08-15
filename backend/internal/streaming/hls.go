package streaming

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/media"
)

// HLS segment seconds — each VOD segment covers this much source time (the
// actual boundary snaps to the next keyframe, so durations vary slightly).
const hlsSegmentSeconds = 4.0

// hlsSessionTTL is how long an HLS session lives without any segment/playlist
// request before its cached segments are swept.
const hlsSessionTTL = 10 * time.Minute

// hlsSession is one active HLS transcode stream for one video: its keyframe
// scan (the VOD segment boundaries), the audio channel count (drives the AAC
// layout choice) and the temp dir holding generated segments.
type hlsSession struct {
	videoID   string
	dir       string
	keyframes []float64
	duration  float64
	// audio is the audio stream index this session transcode (default 0). It
	// selects which of the container's audio tracks to re-encode to AAC, and
	// therefore also which track's channel layout drives the AAC encoding.
	audio    int
	channels int
	lastUsed time.Time
	mu       sync.Mutex
	inflight map[int]*sync.Mutex // per-segment locks so two requests never generate twice
}

// hlsKey caches the per-video keyframe scan across sessions (the scan reads
// packet headers only, but a large library would still benefit from not
// re-scanning on every play).
type hlsKey struct {
	videoID string
}

type hlsManager struct {
	sessions map[string]*hlsSession
	keys     map[hlsKey]*hlsSession
	mu       sync.Mutex
	dir      string
}

func newHLSManager(dataDir string) *hlsManager {
	return &hlsManager{
		sessions: map[string]*hlsSession{},
		keys:     map[hlsKey]*hlsSession{},
		dir:      filepath.Join(dataDir, "hls"),
	}
}

// key returns a session-scoped cache key for a video (one session per viewer,
// so concurrent viewers never share segment files mid-write).
func (m *hlsManager) key(videoID string) hlsKey { return hlsKey{videoID: videoID} }

// session returns the session for videoID+token+audio, creating it on first use
// and caching the (cheap) keyframe scan under the video. The audio index picks
// which track to transcode; the keyframe scan and duration are shared across
// tracks, but the channel count is per-track and probed on the session.
func (m *hlsManager) session(v domain.Video, token string, audio int) *hlsSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(v.ID)
	if s, ok := m.sessions[token]; ok && s.videoID == v.ID {
		s.touch()
		return s
	}
	if s, ok := m.keys[k]; ok {
		// Clone the cached keyframes into a fresh session (dir is per-session).
		s2 := &hlsSession{
			videoID:   v.ID,
			dir:       m.sessionDir(token),
			keyframes: append([]float64(nil), s.keyframes...),
			duration:  s.duration,
			audio:     audio,
			inflight:  map[int]*sync.Mutex{},
		}
		s2.touch()
		m.sessions[token] = s2
		return s2
	}
	s := &hlsSession{
		videoID:   v.ID,
		dir:       m.sessionDir(token),
		duration:  v.Duration,
		audio:     audio,
		inflight:  map[int]*sync.Mutex{},
	}
	s.touch()
	m.sessions[token] = s
	m.keys[k] = s
	return s
}

func (m *hlsManager) sessionDir(token string) string {
	return filepath.Join(m.dir, token)
}

// find returns an existing session by token (for segment serving; the video id
// guard prevents a token from leaking across videos).
func (m *hlsManager) find(v domain.Video, token string) *hlsSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || s.videoID != v.ID {
		return nil
	}
	s.touch()
	return s
}

// sweep removes sessions idle past the TTL and their cached segments.
func (m *hlsManager) sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for token, s := range m.sessions {
		if now.Sub(s.lastUsed) > hlsSessionTTL {
			_ = os.RemoveAll(s.dir)
			delete(m.sessions, token)
			if cached, ok := m.keys[m.key(s.videoID)]; ok && cached == s {
				delete(m.keys, m.key(s.videoID))
			}
		}
	}
}

func (s *hlsSession) touch() { s.lastUsed = time.Now() }

// segLock returns (creating if needed) the per-segment lock so concurrent
// requests for the same segment coalesce into one ffmpeg run.
func (s *hlsSession) segLock(n int) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.inflight[n]; ok {
		return l
	}
	l := &sync.Mutex{}
	s.inflight[n] = l
	return l
}

// Playlist serves the VOD playlist for the HLS transcode stream of a video
// (ADR-006 修订): every segment is declared up front (target duration snapped
// to keyframes) with #EXT-X-ENDLIST, so hls.js shows a full seekable timeline
// and the player is never in "live" mode. Segments are generated on demand by
// Segment().
func (s *Service) Playlist(w http.ResponseWriter, r *http.Request, v domain.Video, token string, audio int) error {
	if !s.TranscodePlayable(v) {
		return ErrUnavailable
	}
	hs := s.hls.session(v, token, audio)
	if err := s.ensureKeyframes(r.Context(), hs, v); err != nil {
		return err
	}
	if len(hs.keyframes) == 0 {
		return fmt.Errorf("no keyframes found in source")
	}
	segs := s.segmentBounds(hs)
	if len(segs) == 0 {
		return fmt.Errorf("source too short for HLS")
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Segment URIs are absolute paths carrying the session token: hls.js resolves
	// relative segment URIs against the playlist URL and would otherwise drop the
	// ?session= query string, so the backend could not map segments to a session.
	segDir := path.Dir(r.URL.Path)
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:3\n")
	b.WriteString("#EXT-X-TARGETDURATION:" + strconv.Itoa(int(math.Ceil(hlsSegmentSeconds))) + "\n")
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	for i, d := range segs {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", d)
		fmt.Fprintf(&b, "%s/seg-%d.ts?session=%s\n", segDir, i, url.QueryEscape(token))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	_, _ = w.Write([]byte(b.String()))
	return nil
}

// segmentBounds returns each segment's duration. Boundaries snap to keyframes:
// a segment runs from keyframe[i] to keyframe[i+1] (or to the video end), so a
// stream copy is frame-exact and hls.js's timeline (sum of EXTINF) matches the
// source PTS.
func (s *Service) segmentBounds(hs *hlsSession) []float64 {
	if len(hs.keyframes) == 0 {
		return nil
	}
	var segs []float64
	for i := 0; i < len(hs.keyframes); i++ {
		end := hs.duration
		if i+1 < len(hs.keyframes) {
			end = hs.keyframes[i+1]
		}
		if d := end - hs.keyframes[i]; d > 0 {
			segs = append(segs, d)
		}
	}
	return segs
}

// Segment serves one on-demand HLS segment (seg-N.ts), generating it with
// ffmpeg into the session dir when it is not already cached. The transcode path
// re-encodes the segment's GOP (libx264 + AAC) and starts each segment at its
// keyframe with its source PTS preserved (-mpegts_copyts 1 + -output_ts_offset),
// so adjacent segments concatenate without gaps or overlaps.
func (s *Service) Segment(w http.ResponseWriter, r *http.Request, v domain.Video, token string, n int) error {
	if !s.TranscodePlayable(v) {
		return ErrUnavailable
	}
	hs := s.hls.find(v, token)
	if hs == nil {
		return ErrNotFound
	}
	if err := s.ensureKeyframes(r.Context(), hs, v); err != nil {
		return err
	}
	if n < 0 || n >= len(hs.keyframes) {
		return ErrNotFound
	}
	path := filepath.Join(hs.dir, fmt.Sprintf("seg-%d.ts", n))
	if _, err := os.Stat(path); err == nil {
		return serveFile(w, r, path, "video/mp2t")
	}
	if err := os.MkdirAll(hs.dir, 0o755); err != nil {
		return err
	}
	lock := hs.segLock(n)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return serveFile(w, r, path, "video/mp2t")
	}
	if err := s.generateSegment(r.Context(), v, hs, n, path); err != nil {
		_ = os.Remove(path)
		return err
	}
	return serveFile(w, r, path, "video/mp2t")
}

// ensureKeyframes runs the keyframe scan once per session (results are cached
// on the session, which itself is cached per video), so the playlist and every
// segment share the same boundaries. The audio channel count is fetched for the
// session's selected track (per-track, since different tracks may differ).
func (s *Service) ensureKeyframes(ctx context.Context, hs *hlsSession, v domain.Video) error {
	if len(hs.keyframes) > 0 {
		return nil
	}
	kfs, err := media.ScanKeyframes(ctx, s.ffprobePath, v.Path)
	if err != nil {
		return err
	}
	hs.mu.Lock()
	hs.keyframes = kfs
	hs.duration = v.Duration
	hs.channels = media.ProbeAudioChannels(ctx, s.ffprobePath, v.Path, hs.audio)
	hs.mu.Unlock()
	return nil
}

// generateSegment runs ffmpeg to produce one transcode segment covering exactly
// the segment's GOP [kf[n], kf[n+1]). The input -ss target is the keyframe
// itself: for re-encodes ffmpeg seeks accurately (decodes the source from the
// previous keyframe and discards frames to the target), so the output always
// starts exactly at kf[n] regardless of container seek quirks. The mpegts
// copyts + output_ts_offset pair keeps the output PTS on the source timeline so
// hls.js can seek across segments.
func (s *Service) generateSegment(ctx context.Context, v domain.Video, hs *hlsSession, n int, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	start := hs.keyframes[n]
	end := hs.duration
	if n+1 < len(hs.keyframes) {
		end = hs.keyframes[n+1]
	}
	// Audio: a >2-channel source is remapped to the standard 5.1 layout (with a
	// bit more headroom) so the native AAC encoder writes an ADTS chanCfg=6
	// stream instead of chanCfg=0+PCE. hls.js derives a 0-channel esds from the
	// latter, which Chromium's MSE rejects with a bufferAppendError that stalls
	// the player and loops the same fragment forever.
	audioArgs := []string{"-c:a", "aac", "-b:a", "128k"}
	if hs.channels > 2 {
		audioArgs = []string{"-c:a", "aac", "-b:a", "192k", "-channel_layout", "5.1"}
	}
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-ss", strconv.FormatFloat(start, 'f', 3, 64),
		"-i", v.Path,
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", hs.audio),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
	}
	args = append(args, audioArgs...)
	args = append(args,
		"-mpegts_copyts", "1",
		"-output_ts_offset", strconv.FormatFloat(start, 'f', 3, 64),
		"-t", strconv.FormatFloat(end-start, 'f', 3, 64),
		"-f", "mpegts",
		out+".tmp",
	)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("hls segment failed", "video_id", v.ID, "seg", n, "err", err, "ffmpeg", truncate(string(output), 300))
		return fmt.Errorf("hls segment %d: %w", n, err)
	}
	return os.Rename(out+".tmp", out)
}

// Sweep runs the HLS session sweeper until ctx is cancelled (called from the
// server main loop). It is a no-op when HLS is disabled (no ffmpeg).
func (s *Service) Sweep(ctx context.Context) {
	if s.hls == nil {
		return
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.hls.sweep()
		}
	}
}

// errHLSUnsupported reports a video that cannot be streamed dynamically.
var errHLSUnsupported = errors.New("hls unavailable")

// truncate shortens a long string (ffmpeg stderr) for log readability.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
