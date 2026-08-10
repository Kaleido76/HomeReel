package streaming

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
)

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
		{"h264 mp4 ac3", domain.Video{Container: "mp4", Codec: "h264", AudioCodec: "ac3"}, true},
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
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
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
	s := New(nil, dataDir, "ffmpeg", "ffprobe")
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
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	s.extractSubtitle = func(context.Context, string, int, string) error { return errors.New("no such stream") }
	v := domain.Video{ID: "v1", Path: filepath.Join("C:\\", "Media", "a.mkv")}
	rec := httptest.NewRecorder()
	if err := s.Subtitle(rec, httptest.NewRequest("GET", "/api/stream/v1/subtitle?track=9", nil), v, 9); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
