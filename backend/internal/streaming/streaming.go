package streaming

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"homereel/backend/internal/domain"
)

// Errors surfaced to the API layer.
var (
	// ErrUnavailable reports that the source storage is offline / file gone.
	ErrUnavailable = errors.New("storage unavailable")
	// ErrNotFound reports a missing cache or sidecar resource.
	ErrNotFound = errors.New("not found")
)

// Service streams indexed videos (ADR-006): HTTP Range direct play for formats
// the browser decodes natively. Whether a file actually plays on a given device
// is decided at runtime by the frontend (canPlayType against the probe metadata
// it receives); DirectPlayable here is a conservative fallback for browsers
// where that probe is unavailable. There is no transcoding: a video that cannot
// play directly must be converted by the user (格式工厂) first.
type Service struct {
	videos  domain.VideoRepo
	dataDir string
}

// New builds the streaming service. dataDir hosts covers/ and thumbs/.
func New(videos domain.VideoRepo, dataDir string) *Service {
	return &Service{
		videos:  videos,
		dataDir: dataDir,
	}
}

// DirectPlayable reports a conservative browser-native playability (Chromium's
// dependable set: native containers plus widely-decodable video/audio codecs).
// It deliberately excludes MKV and HEVC — whether those play depends on the
// target browser (Chromium handles Matroska since 87; HEVC needs a hardware/
// extension decode path), so the frontend decides them at runtime via
// canPlayType against the probed codec metadata. This method is the fallback
// when that runtime probe cannot run.
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

// RemoveCache deletes a video's generated cover/thumb files (called when the
// video is deleted or its file changes so stale images are never served).
func (s *Service) RemoveCache(videoID string) {
	covers := filepath.Join(s.dataDir, "covers")
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		_ = os.Remove(filepath.Join(covers, videoID+ext))
	}
	_ = os.Remove(filepath.Join(s.dataDir, "thumbs", videoID+".thumb.jpg"))
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
	// containers. AC3/EAC3 decode in Chromium (unlike DTS); DTS and lossless PCM
	// are excluded so a file never plays silently.
	nativeAudioCodecs = map[string]bool{
		"aac": true, "mp3": true, "opus": true, "vorbis": true, "flac": true,
		"ac3": true, "eac3": true,
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
