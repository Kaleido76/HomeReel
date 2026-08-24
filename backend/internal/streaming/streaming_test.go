package streaming

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/media"
)

// testMediaPaths returns the binary paths used by tests that must exercise the
// ffmpeg/ffprobe invocation paths without actually resolving them.
func testMediaPaths() media.Paths {
	return media.Paths{FFmpeg: "ffmpeg", FFprobe: "ffprobe"}
}

func TestDirectPlayable(t *testing.T) {
	s := &Service{}
	cases := []struct {
		name string
		v    domain.Video
		want bool
	}{
		{"h264 mp4", domain.Video{Container: "mp4", Codec: "h264"}, true},
		{"comma list mp4", domain.Video{Container: "mov,mp4,m4a,3gp,3g2,mj2", Codec: "h264"}, true},
		{"comma list mov", domain.Video{Container: "mov,mp4,m4a,3gp,3g2,mj2", Codec: "h264"}, true},
		{"h264 mp4 aac", domain.Video{Container: "mp4", Codec: "h264", AudioCodec: "aac"}, true},
		{"h264 mp4 ac3", domain.Video{Container: "mp4", Codec: "h264", AudioCodec: "ac3"}, false},
		{"h264 mp4 dts", domain.Video{Container: "mp4", Codec: "h264", AudioCodec: "dts"}, false},
		{"segmented mp4", domain.Video{Container: "mp4", Codec: "h264", Segmented: true}, false},
		{"hevc mp4", domain.Video{Container: "mp4", Codec: "hevc"}, false},
		{"h264 mkv", domain.Video{Container: "matroska", Codec: "h264"}, false},
		{"h264 mkv aac", domain.Video{Container: "matroska", Codec: "h264", AudioCodec: "aac"}, false},
		{"h264 mov", domain.Video{Container: "mov", Codec: "h264"}, true},
		{"vp9 webm", domain.Video{Container: "webm", Codec: "vp9"}, true},
		{"av1 mp4", domain.Video{Container: "mp4", Codec: "av1"}, true},
		{"unprobed mp4 by ext", domain.Video{Path: "x/y.mp4"}, true},
		{"unprobed mkv by ext", domain.Video{Path: "x/y.mkv"}, false},
		{"unprobed unknown container+codec", domain.Video{Container: "wmv", Codec: "wmv3"}, false},
	}
	for _, c := range cases {
		if got := s.DirectPlayable(c.v); got != c.want {
			t.Errorf("%s: DirectPlayable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestContentType(t *testing.T) {
	cases := []struct {
		name string
		v    domain.Video
		want string
	}{
		{"mp4", domain.Video{Container: "mp4", Path: "a.mp4"}, "video/mp4"},
		{"comma mp4 list", domain.Video{Container: "mov,mp4,m4a,3gp,3g2,mj2", Path: "a.mp4"}, "video/mp4"},
		{"mov extension", domain.Video{Container: "mov,mp4,m4a,3gp,3g2,mj2", Path: "a.mov"}, "video/quicktime"},
		{"matroska", domain.Video{Container: "matroska", Path: "a.mkv"}, "video/x-matroska"},
		{"empty falls back to ext", domain.Video{Path: "a.webm"}, "video/webm"},
		{"container fallback", domain.Video{Container: "matroska", Path: "a.bin"}, "video/x-matroska"},
		{"unknown", domain.Video{Path: "a.xyz"}, "application/octet-stream"},
	}
	for _, c := range cases {
		if got := contentType(c.v); got != c.want {
			t.Errorf("%s: contentType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDirectServesRange(t *testing.T) {
	s := &Service{}
	content := "0123456789"
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	v := domain.Video{Path: path, Container: "mp4", Codec: "h264"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v", nil)
	req.Header.Set("Range", "bytes=2-5")
	if err := s.Direct(rec, req, v); err != nil {
		t.Fatalf("direct: %v", err)
	}
	res := rec.Result()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", res.StatusCode)
	}
	if string(raw) != "2345" {
		t.Fatalf("range body = %q, want 2345", raw)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("content-type = %q, want video/mp4", ct)
	}
}

func TestCoverServesFromDataDir(t *testing.T) {
	dataDir := t.TempDir()
	s := &Service{dataDir: dataDir}
	coverRel := "covers/v1.jpg"
	if err := os.MkdirAll(filepath.Join(dataDir, "covers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "covers", "v1.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := domain.Video{CoverPath: coverRel}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v/cover", nil)
	if err := s.Cover(rec, req, v, false); err != nil {
		t.Fatalf("cover: %v", err)
	}
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("cover status = %d", rec.Result().StatusCode)
	}

	v.CoverPath = ""
	if err := s.Cover(rec, req, v, false); err != ErrNotFound {
		t.Fatalf("missing cover err = %v, want ErrNotFound", err)
	}
}

func TestCoverPathTraversalRejected(t *testing.T) {
	dataDir := t.TempDir()
	s := &Service{dataDir: dataDir}
	v := domain.Video{CoverPath: "../../outside.jpg"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v/cover", nil)
	if err := s.Cover(rec, req, v, false); err != ErrNotFound {
		t.Fatalf("traversal err = %v, want ErrNotFound", err)
	}
}

func TestSubtitleServesEmbeddedTrack(t *testing.T) {
	s := New(nil, t.TempDir(), testMediaPaths())
	v := domain.Video{ID: "v1", Path: filepath.Join("C:\\", "Media", "a.mkv")}
	calls := 0
	s.extractSubtitle = func(_ context.Context, src string, streamIndex int, out string) error {
		calls++
		if err := os.WriteFile(out, []byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nhello"), 0o644); err != nil {
			return err
		}
		return nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/subtitle?track=2", nil)
	if err := s.Subtitle(rec, req, v, 2); err != nil {
		t.Fatalf("subtitle: %v", err)
	}
	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/vtt; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/vtt", ct)
	}
	if calls != 1 {
		t.Fatalf("extract calls = %d, want 1", calls)
	}

	// Cached: a second request reuses the extracted file without re-extracting.
	rec2 := httptest.NewRecorder()
	if err := s.Subtitle(rec2, httptest.NewRequest("GET", "/api/stream/v1/subtitle?track=2", nil), v, 2); err != nil {
		t.Fatalf("subtitle cached: %v", err)
	}
	if calls != 1 {
		t.Fatalf("extract calls after cache = %d, want 1", calls)
	}
}

func TestSubtitleSidecarWinsAndSkipsExtract(t *testing.T) {
	dataDir := t.TempDir()
	s := New(nil, dataDir, testMediaPaths())
	src := filepath.Join(dataDir, "clip.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "clip.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nhi"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.extractSubtitle = func(context.Context, string, int, string) error {
		calls++
		return nil
	}
	v := domain.Video{ID: "v1", Path: src}
	rec := httptest.NewRecorder()
	if err := s.Subtitle(rec, httptest.NewRequest("GET", "/api/stream/v1/subtitle", nil), v, -1); err != nil {
		t.Fatalf("subtitle: %v", err)
	}
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Result().StatusCode)
	}
	if calls != 0 {
		t.Fatalf("extract calls = %d, want 0 (sidecar wins)", calls)
	}
}

func TestSubtitleNotFoundWithoutTrack(t *testing.T) {
	s := New(nil, t.TempDir(), testMediaPaths())
	s.extractSubtitle = func(context.Context, string, int, string) error { return errors.New("no such stream") }
	v := domain.Video{ID: "v1", Path: filepath.Join("C:\\", "Media", "a.mkv")}
	rec := httptest.NewRecorder()
	if err := s.Subtitle(rec, httptest.NewRequest("GET", "/api/stream/v1/subtitle?track=9", nil), v, 9); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCacheOverviewAndClear(t *testing.T) {
	s := New(nil, t.TempDir(), media.Paths{})
	writeTestFile(t, filepath.Join(s.dataDir, "covers", "v1.jpg"), "jpg")
	writeTestFile(t, filepath.Join(s.dataDir, "covers", "gone.webp"), "webp")
	writeTestFile(t, filepath.Join(s.dataDir, "thumbs", "v1.thumb.jpg"), "thumb")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1-1.vtt"), "vtt")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1.vtt"), "vtt-old")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "gone-2.vtt"), "vtt")

	ids := map[string]struct{}{"v1": {}}
	overview := s.CacheOverview(ids)
	if c := overview["cover"]; c.Files != 2 || c.Orphans != 1 {
		t.Fatalf("cover stat = files %d orphans %d, want 2/1", c.Files, c.Orphans)
	}
	if th := overview["thumb"]; th.Files != 1 || th.Orphans != 0 {
		t.Fatalf("thumb stat = files %d orphans %d, want 1/0", th.Files, th.Orphans)
	}
	if sub := overview["subtitle"]; sub.Files != 3 || sub.Orphans != 1 {
		t.Fatalf("subtitle stat = files %d orphans %d, want 3/1", sub.Files, sub.Orphans)
	}

	if n := s.ClearAllSubtitles(); n != 3 {
		t.Fatalf("clear all subtitles removed %d, want 3", n)
	}
}

func TestListSubtitleCacheAndClearTracks(t *testing.T) {
	s := New(nil, t.TempDir(), media.Paths{})
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1-1.vtt"), "vtt")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1-3.vtt"), "vtt")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1.vtt"), "vtt-old")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v9-2.vtt"), "vtt")

	files := s.ListSubtitleCache()
	byName := map[string]SubtitleCacheFile{}
	for _, f := range files {
		byName[f.Name] = f
	}
	if f, ok := byName["v1-3.vtt"]; !ok || f.VideoID != "v1" || f.Track != 3 {
		t.Fatalf("v1-3 parsed as %+v, want video v1 track 3", f)
	}
	if f, ok := byName["v1.vtt"]; !ok || f.VideoID != "v1" || f.Track != -1 {
		t.Fatalf("legacy v1.vtt parsed as %+v, want track -1", f)
	}

	if n := s.ClearSubtitleTrack("v1", 3); n != 1 {
		t.Fatalf("clear track removed %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "subtitles", "v1-3.vtt")); err == nil {
		t.Fatal("v1-3.vtt should be removed")
	}
	if n := s.ClearSubtitles("v1"); n != 2 {
		t.Fatalf("clear video removed %d, want 2 (v1-1 + legacy)", n)
	}
	if n := s.ClearSubtitles("v9"); n != 1 {
		t.Fatalf("clear v9 removed %d, want 1", n)
	}
	if n := s.ClearSubtitles("v1"); n != 0 {
		t.Fatalf("re-clear v1 removed %d, want 0", n)
	}
}

func TestClearOrphans(t *testing.T) {
	s := New(nil, t.TempDir(), media.Paths{})
	writeTestFile(t, filepath.Join(s.dataDir, "covers", "v1.jpg"), "jpg")
	writeTestFile(t, filepath.Join(s.dataDir, "covers", "gone.jpg"), "jpg")
	writeTestFile(t, filepath.Join(s.dataDir, "thumbs", "v1.thumb.jpg"), "thumb")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "v1-1.vtt"), "vtt")
	writeTestFile(t, filepath.Join(s.dataDir, "subtitles", "gone-3.vtt"), "vtt")

	ids := map[string]struct{}{"v1": {}}
	if n := s.ClearOrphans(ids); n != 2 {
		t.Fatalf("clear orphans removed %d, want 2", n)
	}
	for _, keep := range []string{"covers/v1.jpg", "thumbs/v1.thumb.jpg", "subtitles/v1-1.vtt"} {
		if _, err := os.Stat(filepath.Join(s.dataDir, keep)); err != nil {
			t.Fatalf("%s should be kept: %v", keep, err)
		}
	}
	for _, gone := range []string{"covers/gone.jpg", "subtitles/gone-3.vtt"} {
		if _, err := os.Stat(filepath.Join(s.dataDir, gone)); err == nil {
			t.Fatalf("%s should have been removed", gone)
		}
	}
}

func TestRemuxPlayable(t *testing.T) {
	withFFmpeg := &Service{media: media.Paths{FFmpeg: "ffmpeg"}}
	cases := []struct {
		name string
		v    domain.Video
		want bool
	}{
		{"mkv h264 aac", domain.Video{Codec: "h264", AudioCodec: "aac", Container: "matroska"}, true},
		{"mkv h264 mp3", domain.Video{Codec: "h264", AudioCodec: "mp3", Container: "matroska"}, true},
		{"h264 no audio", domain.Video{Codec: "h264"}, true},
		{"h264 ac3", domain.Video{Codec: "h264", AudioCodec: "ac3"}, false},
		{"h264 eac3", domain.Video{Codec: "h264", AudioCodec: "eac3"}, false},
		{"h264 dts", domain.Video{Codec: "h264", AudioCodec: "dts"}, false},
		{"h264 pcm", domain.Video{Codec: "h264", AudioCodec: "pcm_s16le"}, false},
		{"h264 truehd", domain.Video{Codec: "h264", AudioCodec: "truehd"}, false},
		{"hevc aac", domain.Video{Codec: "hevc", AudioCodec: "aac"}, false},
		{"rv40 aac", domain.Video{Codec: "rv40", AudioCodec: "aac"}, false},
	}
	for _, c := range cases {
		if got := withFFmpeg.RemuxPlayable(c.v); got != c.want {
			t.Errorf("%s: RemuxPlayable = %v, want %v", c.name, got, c.want)
		}
	}
	if (&Service{}).RemuxPlayable(domain.Video{Codec: "h264", AudioCodec: "aac"}) {
		t.Fatal("no ffmpeg should not be remuxable")
	}
}

func TestTranscodePlayable(t *testing.T) {
	withFFmpeg := &Service{media: media.Paths{FFmpeg: "ffmpeg"}}
	noFFmpeg := &Service{}
	if !withFFmpeg.TranscodePlayable(domain.Video{Duration: 100}) {
		t.Fatal("ffmpeg + duration should be transcodeable")
	}
	if withFFmpeg.TranscodePlayable(domain.Video{Duration: 0}) {
		t.Fatal("unknown duration should not be transcodeable")
	}
	if noFFmpeg.TranscodePlayable(domain.Video{Duration: 100}) {
		t.Fatal("no ffmpeg should not be transcodeable")
	}
}

// TestRemuxCacheReusedAndFingerprintInvalidates verifies the cached remux MP4 is
// reused for an unchanged source and regenerated when the source's size or mtime
// changes (a replaced file must never serve stale bytes).
func TestRemuxCacheReusedAndFingerprintInvalidates(t *testing.T) {
	base := t.TempDir()
	s := New(nil, base, testMediaPaths())
	remuxed := 0
	s.remuxVideo = func(_ context.Context, _ domain.Video, out string, _ int) error {
		remuxed++
		return os.WriteFile(out, []byte("mp4"), 0o644)
	}
	src := filepath.Join(base, "a.mkv")
	if err := os.WriteFile(src, []byte("mkv"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := domain.Video{ID: "v1", Path: src, Size: 3, MTime: 10, Codec: "h264", AudioCodec: "aac"}

	if _, err := s.remuxPath(context.Background(), v, 0); err != nil {
		t.Fatalf("first remux: %v", err)
	}
	if _, err := s.remuxPath(context.Background(), v, 0); err != nil {
		t.Fatalf("second remux: %v", err)
	}
	if remuxed != 1 {
		t.Fatalf("remux calls = %d, want 1 (cached)", remuxed)
	}

	v.MTime = 11 // source replaced
	if _, err := s.remuxPath(context.Background(), v, 0); err != nil {
		t.Fatalf("remux after change: %v", err)
	}
	if remuxed != 2 {
		t.Fatalf("remux calls after change = %d, want 2", remuxed)
	}

	// A different audio track is cached separately from the default track.
	if _, err := s.remuxPath(context.Background(), v, 1); err != nil {
		t.Fatalf("track-1 remux: %v", err)
	}
	if _, err := s.remuxPath(context.Background(), v, 1); err != nil {
		t.Fatalf("track-1 remux cached: %v", err)
	}
	if remuxed != 3 {
		t.Fatalf("remux calls after track switch = %d, want 3", remuxed)
	}
	for _, name := range []string{"v1.mp4", "v1-a1.mp4"} {
		if _, err := os.Stat(filepath.Join(base, "remux", name)); err != nil {
			t.Fatalf("%s cache missing: %v", name, err)
		}
	}
}

// TestRemuxServesRange verifies the Remux handler serves the cached MP4 with
// HTTP Range support and video/mp4 content type.
func TestRemuxServesRange(t *testing.T) {
	base := t.TempDir()
	s := New(nil, base, testMediaPaths())
	content := []byte("0123456789")
	s.remuxVideo = func(_ context.Context, _ domain.Video, out string, _ int) error {
		return os.WriteFile(out, content, 0o644)
	}
	src := filepath.Join(base, "a.mkv")
	if err := os.WriteFile(src, []byte("mkv"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := domain.Video{ID: "v1", Path: src, Size: 3, MTime: 1, Codec: "h264", AudioCodec: "aac"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/remux", nil)
	req.Header.Set("Range", "bytes=2-5")
	if err := s.Remux(rec, req, v, 0); err != nil {
		t.Fatalf("remux: %v", err)
	}
	res := rec.Result()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", res.StatusCode)
	}
	if string(raw) != "2345" {
		t.Fatalf("range body = %q, want 2345", raw)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("content-type = %q, want video/mp4", ct)
	}
}

// TestRemuxUnavailable gates the remux endpoint for files that must transcode
// instead (codec incompatible) and when no ffmpeg is configured.
func TestRemuxUnavailable(t *testing.T) {
	s := New(nil, t.TempDir(), media.Paths{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/remux", nil)
	if err := s.Remux(rec, req, domain.Video{ID: "v1", Codec: "hevc", AudioCodec: "aac"}, 0); err != ErrUnavailable {
		t.Fatalf("remux err = %v, want ErrUnavailable", err)
	}
}

// TestHLSSegmentBounds validates the keyframe-aligned boundary math that keeps a
// transcode frame-exact and the hls.js timeline continuous.
func TestHLSSegmentBounds(t *testing.T) {
	s := &Service{}
	hs := &hlsSession{
		keyframes: []float64{0, 4, 8, 12},
		duration:  15,
	}
	segs := s.segmentBounds(hs)
	want := []float64{4, 4, 4, 3}
	if len(segs) != len(want) {
		t.Fatalf("segment count = %d, want %d", len(segs), len(want))
	}
	for i, d := range want {
		if segs[i] != d {
			t.Errorf("segment %d duration = %v, want %v", i, segs[i], d)
		}
	}
}

// TestHLSPlaylistContent verifies the served VOD playlist covers the whole
// timeline with an ENDLIST (so hls.js renders a draggable progress bar, not a
// live window) and declares one entry per keyframe-aligned segment.
func TestHLSPlaylistContent(t *testing.T) {
	s := New(nil, t.TempDir(), testMediaPaths())
	keyframes := []float64{0, 2, 4, 6, 8}
	v := domain.Video{ID: "v1", Path: "x.mkv", Duration: 10, Codec: "hevc", AudioCodec: "aac"}
	// Playlist uses the session's cached keyframes directly via ensureKeyframes,
	// so override the manager to return a pre-filled session.
	s.hls.sessions["tok"] = &hlsSession{
		videoID:   v.ID,
		dir:       filepath.Join(s.hls.dir, "tok"),
		keyframes: keyframes,
		duration:  v.Duration,
		inflight:  map[int]*sync.Mutex{},
	}
	s.hls.keys[s.hls.key(v.ID)] = s.hls.sessions["tok"]

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/hls/playlist.m3u8?session=tok", nil)
	if err := s.Playlist(rec, req, v, "tok", 0); err != nil {
		t.Fatalf("playlist: %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{"#EXTM3U", "#EXT-X-ENDLIST", "#EXTINF:2.000,", "seg-0.ts", "seg-4.ts"} {
		if !strings.Contains(body, want) {
			t.Errorf("playlist missing %q\n%s", want, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/vnd.apple.mpegurl") {
		t.Errorf("content-type = %q, want application/vnd.apple.mpegurl", ct)
	}
}

func TestHLSUnavailable(t *testing.T) {
	s := New(nil, t.TempDir(), media.Paths{})
	v := domain.Video{ID: "v1", Path: "x.mkv", Duration: 10}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/hls/playlist.m3u8?session=tok", nil)
	if err := s.Playlist(rec, req, v, "tok", 0); err != ErrUnavailable {
		t.Fatalf("playlist err = %v, want ErrUnavailable", err)
	}
}
