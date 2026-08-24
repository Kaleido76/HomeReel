package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/media"
)

// remuxVideoCodecs / remuxAudioCodecs live in the media package
// (media.RemuxVideoCodecs / media.UniversalAudioCodecs) as the single shared
// white lists also used by the format factory.

// RemuxPlayable reports whether a video the frontend could not play directly
// can be made playable by a container remux to MP4 (e.g. MKV h264+aac → MP4).
// Only browser-decodable audio (aac/mp3) is stream-copied; a video whose audio
// cannot be copied (AC3/EAC3/DTS/PCM) is not remuxable and is handled by the HLS
// transcode tier instead. The remuxed MP4 is served over HTTP Range like a
// native file.
func (s *Service) RemuxPlayable(v domain.Video) bool {
	if s.media.FFmpeg == "" || !media.RemuxVideoCodecs[v.Codec] {
		return false
	}
	if v.AudioCodec == "" {
		return true
	}
	return media.UniversalAudioCodecs[v.AudioCodec]
}

// TranscodePlayable reports whether the HLS transcode endpoint can serve the
// video: an ffmpeg is configured and the duration is known (the VOD playlist
// needs the full timeline). Transcoding re-encodes incompatible streams.
func (s *Service) TranscodePlayable(v domain.Video) bool {
	return s.media.FFmpeg != "" && v.Duration > 0
}

// remuxLocks serializes remux generation per video so two concurrent play
// requests never run ffmpeg twice for the same file.
type remuxLock struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func (l *remuxLock) get(id string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.m[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	l.m[id] = m
	return m
}

// remuxCacheName returns the cached remux MP4 file name for a video and audio
// track. The default track (0) keeps the plain "<id>.mp4" name (backward
// compatible with existing caches); other tracks use "<id>-a<N>.mp4" so each
// track remuxes into its own cached file.
func remuxCacheName(videoID string, audio int) string {
	if audio <= 0 {
		return videoID + ".mp4"
	}
	return fmt.Sprintf("%s-a%d.mp4", videoID, audio)
}

// remuxPath returns the cached remuxed MP4 for a video and audio track, remuxing
// the source with a stream copy on first use. The result is keyed by a
// size+mtime fingerprint so a replaced source file is remuxed again instead of
// serving stale bytes; the audio track is carried in the file name. Callers
// should only reach here when RemuxPlayable is true.
func (s *Service) remuxPath(ctx context.Context, v domain.Video, audio int) (string, error) {
	if _, err := os.Stat(v.Path); err != nil {
		return "", ErrUnavailable
	}
	cached := filepath.Join(s.remuxDir, remuxCacheName(v.ID, audio))
	meta := cached + ".meta"
	fingerprint := fmt.Sprintf("%d %d", v.Size, v.MTime)
	if b, err := os.ReadFile(meta); err == nil && string(b) == fingerprint {
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}
	lock := s.remuxLocks.get(v.ID)
	lock.Lock()
	defer lock.Unlock()
	// Re-check after acquiring the lock: another request may have remuxed it.
	if b, err := os.ReadFile(meta); err == nil && string(b) == fingerprint {
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}
	if err := s.generateRemux(ctx, v, cached, audio); err != nil {
		return "", err
	}
	return cached, nil
}

// generateRemux converts the video's streams into a faststart MP4 (video and
// the selected audio track always copied) and writes the fingerprint sidecar
// once the rename lands.
func (s *Service) generateRemux(ctx context.Context, v domain.Video, out string, audio int) error {
	if err := os.MkdirAll(s.remuxDir, 0o755); err != nil {
		return err
	}
	tmp := out + ".tmp"
	if err := s.remuxVideo(ctx, v, tmp, audio); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.WriteFile(out+".meta", []byte(fmt.Sprintf("%d %d", v.Size, v.MTime)), 0o644)
}

// remuxVideo stream-copies the video's browser-decodable streams into the MP4
// at out (faststart layout so Range seeking works). Audio is always aac/mp3 or
// absent here (RemuxPlayable gates the endpoint), so it is stream-copied too.
// audio selects which of the container's audio tracks is mapped. Injected for
// tests.
func (s *Service) defaultRemuxVideo(ctx context.Context, v domain.Video, out string, audio int) error {
	if err := media.RemuxVideo(ctx, s.media, media.RemuxOpts{Src: v.Path, Out: out, Audio: audio}); err != nil {
		slog.Warn("remux failed", "video_id", v.ID, "err", err)
		return fmt.Errorf("remux %s: %w", v.Path, err)
	}
	return nil
}

// RemoveRemux deletes a video's cached remux MP4s (all audio tracks and their
// fingerprint sidecars) so a deleted or replaced file is never served stale.
func (s *Service) RemoveRemux(videoID string) {
	for _, p := range []string{
		filepath.Join(s.remuxDir, videoID+".mp4"),
		filepath.Join(s.remuxDir, videoID+".mp4.meta"),
	} {
		_ = os.Remove(p)
	}
	matches, _ := filepath.Glob(filepath.Join(s.remuxDir, videoID+"-a*.mp4*"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// Remux serves the cached remuxed MP4 of a video with HTTP Range support,
// remuxing the source with a stream copy on first play. It is the backend half
// of the ADR-006 remux tier: the frontend requests it when canPlayType rejected
// the original container but every stream is browser-decodable, so a pure
// container conversion makes the file playable natively (full seek). audio
// selects which audio track is mapped (default 0).
func (s *Service) Remux(w http.ResponseWriter, r *http.Request, v domain.Video, audio int) error {
	if !s.RemuxPlayable(v) {
		return ErrUnavailable
	}
	path, err := s.remuxPath(r.Context(), v, audio)
	if err != nil {
		return err
	}
	return serveFile(w, r, path, "video/mp4")
}

// remuxIDFromName recovers the owning video id from a remux cache base name
// ("<id>" from "<id>.mp4", "<id>" from "<id>-a<N>.mp4", or "<id>.mp4" from the
// "<id>.mp4.meta" sidecar whose extension was trimmed first), so the cache
// scanner can classify both files.
func remuxIDFromName(base string) (string, bool) {
	if id := strings.TrimSuffix(base, ".mp4"); id != "" {
		if m := remuxTrackRe.FindStringSubmatch(id); m != nil {
			return m[1], true
		}
		return id, true
	}
	return "", false
}

// remuxTrackRe matches the per-track remux cache name "<id>-a<N>".
var remuxTrackRe = regexp.MustCompile(`^(.*)-a\d+$`)
