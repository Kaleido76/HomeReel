package streaming

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/media"
)

// Errors surfaced to the API layer.
var (
	// ErrUnavailable reports that the source storage is offline / file gone.
	ErrUnavailable = errors.New("storage unavailable")
	// ErrNotFound reports a missing cache or sidecar resource.
	ErrNotFound = errors.New("not found")
)

// Service streams indexed videos (ADR-006, 2026-08 修订): HTTP Range direct
// play for formats the browser decodes natively, a container-only remux to a
// cached MP4 for browser-decodable streams in a foreign container (MKV h264+aac),
// and an on-demand HLS transcode for everything else. Whether a file actually
// plays directly on a given device is decided at runtime by the frontend
// (canPlayType against the probe metadata it receives); DirectPlayable here is a
// conservative fallback for browsers where that probe is unavailable.
type Service struct {
	videos      domain.VideoRepo
	dataDir     string
	ffmpegPath  string
	ffprobePath string
	subDir      string
	remuxDir    string
	remuxLocks  remuxLock
	hls         *hlsManager
	// extractSubtitle extracts the subtitle stream streamIndex of src into out
	// (WebVTT). Injected for tests.
	extractSubtitle func(ctx context.Context, src string, streamIndex int, out string) error
	// remuxVideo stream-copies a video into a playable MP4. Injected for tests.
	remuxVideo func(ctx context.Context, v domain.Video, out string, audio int) error
}

// New builds the streaming service. dataDir hosts covers/, thumbs/, remux/,
// hls/ and the extracted-subtitle cache (subtitles/).
func New(videos domain.VideoRepo, dataDir, ffmpegPath, ffprobePath string) *Service {
	s := &Service{
		videos:      videos,
		dataDir:     dataDir,
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		subDir:      filepath.Join(dataDir, "subtitles"),
		remuxDir:    filepath.Join(dataDir, "remux"),
		remuxLocks:  remuxLock{m: map[string]*sync.Mutex{}},
		hls:         newHLSManager(dataDir),
	}
	s.extractSubtitle = func(ctx context.Context, src string, streamIndex int, out string) error {
		return media.ExtractTextSubtitle(ctx, ffmpegPath, src, streamIndex, out)
	}
	s.remuxVideo = s.defaultRemuxVideo
	return s
}

// DirectPlayable reports a conservative browser-native playability (Chromium's
// dependable set: native containers plus widely-decodable video/audio codecs).
// It deliberately excludes MKV and HEVC — whether those play depends on the
// target browser (Chromium ships a Matroska demuxer on recent desktop builds;
// HEVC needs a hardware/extension decode path), so the frontend decides them at
// runtime via canPlayType against the probed codec metadata. This method is the
// fallback when that runtime probe cannot run.
func (s *Service) DirectPlayable(v domain.Video) bool {
	if v.Codec == "" {
		// Unprobed: extension-based guess so a fresh file still plays.
		if v.Container != "" {
			return false
		}
		switch strings.ToLower(filepath.Ext(v.Path)) {
		case ".mp4", ".m4v", ".mov", ".webm", ".ogv", ".ogg":
			return true
		}
		return false
	}
	if v.Segmented || !nativeCodecs[v.Codec] || !containerNative(v.Container) {
		return false
	}
	if v.AudioCodec != "" && !nativeAudioCodecs[v.AudioCodec] {
		return false
	}
	return true
}

// Direct serves the source file with HTTP Range support.
func (s *Service) Direct(w http.ResponseWriter, r *http.Request, v domain.Video) error {
	f, err := os.Open(v.Path)
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
	http.ServeContent(w, r, filepath.Base(v.Path), info.ModTime(), f)
	return nil
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

// SubtitleTrack is one subtitle source the player can pick from: a sidecar file
// next to the video or an embedded text subtitle track (stream index).
type SubtitleTrack struct {
	Kind  string `json:"kind"` // sidecar | embedded
	Index int    `json:"index,omitempty"`
	Codec string `json:"codec,omitempty"`
	Label string `json:"label,omitempty"`
}

// ListSubtitles enumerates the playable subtitle sources of a video, ordered
// with the sidecar first. Embedded bitmap tracks (PGS/VobSub) are skipped.
func (s *Service) ListSubtitles(ctx context.Context, v domain.Video) []SubtitleTrack {
	var out []SubtitleTrack
	if p := sidecarPath(v); p != "" {
		out = append(out, SubtitleTrack{
			Kind:  "sidecar",
			Label: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)),
		})
	}
	subs, err := media.ProbeSubtitles(ctx, s.ffprobePath, v.Path)
	if err != nil {
		slog.Warn("probe subtitles", "video_id", v.ID, "err", err)
		return out
	}
	n := 0
	for _, st := range subs {
		if !media.TextSubtitleCodecs[st.Codec] {
			continue
		}
		n++
		out = append(out, SubtitleTrack{
			Kind:  "embedded",
			Index: st.Index,
			Codec: st.Codec,
			Label: subtitleLabel(st.Language, st.Title, n),
		})
	}
	return out
}

// AudioTrack is one audio track of a video the player can switch to (mirrors
// SubtitleTrack). Index is the 0-based AUDIO-stream ordinal: both the ?audio=
// query param and ffmpeg's -map 0:a:<index> / -select_streams a:<index> select
// audio streams by ordinal (NOT the container's absolute stream index, which is
// offset by the video/subtitle streams).
type AudioTrack struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec,omitempty"`
	Language string `json:"language,omitempty"`
	Label    string `json:"label,omitempty"`
	Channels int    `json:"channels,omitempty"`
}

// ListAudioTracks enumerates the audio tracks of a video via ffprobe, ordered
// by stream index. The first track is the container's default (what playback
// uses today). A single track is still returned so the UI can show it (or hide
// the menu when the length is 1).
func (s *Service) ListAudioTracks(ctx context.Context, v domain.Video) []AudioTrack {
	tracks, err := media.ProbeAudioStreams(ctx, s.ffprobePath, v.Path)
	if err != nil {
		slog.Warn("probe audio tracks", "video_id", v.ID, "err", err)
		return nil
	}
	out := make([]AudioTrack, 0, len(tracks))
	for i, t := range tracks {
		out = append(out, AudioTrack{
			Index:    i, // audio ordinal, matches -map 0:a:<i>
			Codec:    t.Codec,
			Language: t.Language,
			Channels: t.Channels,
			Label:    audioLabel(t.Language, t.Title, i+1),
		})
	}
	return out
}

// subtitleLangLabels map ISO 639 language tags to short Chinese labels.
var subtitleLangLabels = map[string]string{
	"chi": "中文", "zho": "中文", "zh": "中文", "chs": "中文", "cht": "中文",
	"eng": "英文", "en": "英文",
	"jpn": "日文", "ja": "日文",
	"kor": "韩文", "ko": "韩文",
	"fre": "法文", "fra": "法文", "fr": "法文",
	"spa": "西语", "es": "西语",
}

func subtitleLabel(lang, title string, fallback int) string {
	if title != "" {
		return title
	}
	if l, ok := subtitleLangLabels[strings.ToLower(lang)]; ok {
		return l
	}
	return fmt.Sprintf("字幕 %d", fallback)
}

// audioLabel builds an audio track's display label: the track title when set
// (e.g. "国语"/"粤语"), else a language label, else "音轨 N".
func audioLabel(lang, title string, fallback int) string {
	if title != "" {
		return title
	}
	if l, ok := subtitleLangLabels[strings.ToLower(lang)]; ok {
		return l
	}
	return fmt.Sprintf("音轨 %d", fallback)
}

// Subtitle serves a subtitle for the player. trackIndex selects an embedded
// text subtitle stream (its stream index); a negative value means "default":
// the sidecar file when present, otherwise the first embedded text track.
// Extracted WebVTT files are cached under subtitles/<video_id>-<index>.vtt.
func (s *Service) Subtitle(w http.ResponseWriter, r *http.Request, v domain.Video, trackIndex int) error {
	// A sidecar wins on the default path (explicit embedded picks ignore it).
	if trackIndex < 0 {
		if p := sidecarPath(v); p != "" {
			return serveFile(w, r, p, sidecarMime(p))
		}
	}
	index := trackIndex
	if index < 0 {
		subs, err := media.ProbeSubtitles(r.Context(), s.ffprobePath, v.Path)
		if err != nil {
			slog.Warn("probe subtitles", "video_id", v.ID, "err", err)
			return ErrNotFound
		}
		for _, st := range subs {
			if media.TextSubtitleCodecs[st.Codec] {
				index = st.Index
				break
			}
		}
		if index < 0 {
			return ErrNotFound
		}
	}
	if s.ffmpegPath == "" {
		return ErrNotFound
	}
	cached := filepath.Join(s.subDir, fmt.Sprintf("%s-%d.vtt", v.ID, index))
	if _, err := os.Stat(cached); err == nil {
		return serveFile(w, r, cached, "text/vtt; charset=utf-8")
	}
	if err := os.MkdirAll(s.subDir, 0o755); err != nil {
		return err
	}
	if err := s.extractSubtitle(r.Context(), v.Path, index, cached); err != nil {
		_ = os.Remove(cached + ".tmp")
		slog.Warn("extract subtitle", "video_id", v.ID, "track", index, "err", err)
		return ErrNotFound
	}
	return serveFile(w, r, cached, "text/vtt; charset=utf-8")
}

// sidecarPath returns the sidecar subtitle file (.vtt/.srt/.ass/.ssa) sitting
// next to the source, or "" when none exists.
func sidecarPath(v domain.Video) string {
	base := strings.TrimSuffix(filepath.Base(v.Path), filepath.Ext(v.Path))
	dir := filepath.Dir(v.Path)
	for _, ext := range []string{".vtt", ".srt", ".ass", ".ssa"} {
		p := filepath.Join(dir, base+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func sidecarMime(p string) string {
	if strings.EqualFold(filepath.Ext(p), ".vtt") {
		return "text/vtt; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

// RemoveCache deletes a video's generated cover/thumb files, extracted
// subtitles, and cached remux MP4 (called when the video is deleted so stale
// images or media are never served).
func (s *Service) RemoveCache(videoID string) {
	covers := filepath.Join(s.dataDir, "covers")
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		_ = os.Remove(filepath.Join(covers, videoID+ext))
	}
	_ = os.Remove(filepath.Join(s.dataDir, "thumbs", videoID+".thumb.jpg"))
	s.RemoveSubtitles(videoID)
	s.RemoveRemux(videoID)
}

// RemoveSubtitles deletes a video's extracted-subtitle cache. It is what an
// update event clears: the cover/thumb survive a content change because the
// scanner regenerates them (processInline) whenever it re-probes; a pure
// rename/move does not even change them (ADR-017).
func (s *Service) RemoveSubtitles(videoID string) {
	matches, _ := filepath.Glob(filepath.Join(s.subDir, videoID+"-*.vtt"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
	_ = os.Remove(filepath.Join(s.subDir, videoID+".vtt"))
}

func serveFile(w http.ResponseWriter, r *http.Request, path, contentType string) error {
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
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	return nil
}

// nativeContainers / nativeCodecs describe the container/codec set every
// Chromium browser decodes for <video src> without any platform extension.
var (
	nativeContainers = map[string]bool{
		"mp4": true, "m4v": true, "mov": true, "qt": true,
		"webm": true, "ogg": true, "ogv": true,
	}
	nativeCodecs = map[string]bool{
		"h264": true, "avc1": true, "avc3": true,
		"vp8": true, "vp9": true, "av1": true, "theora": true,
	}
	// nativeAudioCodecs are audio codecs Chromium decodes inside the native
	// containers. AC3/EAC3 are excluded: Chromium and Firefox do not ship a
	// Dolby decoder, so a file with AC3 audio would play silently; such files
	// are served through the remux tier (audio re-encoded to AAC) instead.
	// DTS and lossless PCM are excluded for the same reason.
	nativeAudioCodecs = map[string]bool{
		"aac": true, "mp3": true, "opus": true, "vorbis": true, "flac": true,
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
