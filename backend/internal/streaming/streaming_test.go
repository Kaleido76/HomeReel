package streaming

import (
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
		{"segmented mp4", domain.Video{Container: "mp4", Codec: "h264", Segmented: true}, true},
		{"hevc mp4", domain.Video{Container: "mp4", Codec: "hevc"}, false},
		{"h264 mkv", domain.Video{Container: "matroska", Codec: "h264"}, false},
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

func TestHLSEnabled(t *testing.T) {
	direct := domain.Video{Container: "mp4", Codec: "h264"}
	needs := domain.Video{Container: "matroska", Codec: "hevc"}

	auto := &Service{enableHLS: "auto"}
	if auto.HLSEnabled(direct) {
		t.Error("auto: direct-playable should not need HLS")
	}
	if !auto.HLSEnabled(needs) {
		t.Error("auto: non-playable should need HLS")
	}
	on := &Service{enableHLS: "true"}
	if !on.HLSEnabled(direct) {
		t.Error("true: always HLS")
	}
	off := &Service{enableHLS: "false"}
	if off.HLSEnabled(needs) {
		t.Error("false: never HLS")
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

// TestDirectServesRemuxed verifies a segmented video is served from its
// remuxed faststart copy when one exists, and from the raw source otherwise.
func TestDirectServesRemuxed(t *testing.T) {
	dataDir := t.TempDir()
	s := &Service{dataDir: dataDir, remuxDir: filepath.Join(dataDir, "remux")}
	if err := os.MkdirAll(s.remuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(dataDir, "src.mp4")
	remuxPath := filepath.Join(s.remuxDir, "v1.mp4")
	if err := os.WriteFile(srcPath, []byte("SOURCE-BYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remuxPath, []byte("REMUXED-BYTES"), 0o644); err != nil {
		t.Fatal(err)
	}

	serve := func(v domain.Video) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stream/v", nil)
		if err := s.Direct(rec, req, v); err != nil {
			t.Fatalf("direct: %v", err)
		}
		raw, _ := io.ReadAll(rec.Result().Body)
		return string(raw)
	}

	segmented := domain.Video{ID: "v1", Path: srcPath, Container: "mp4", Codec: "h264", Segmented: true}
	if got := serve(segmented); got != "REMUXED-BYTES" {
		t.Errorf("segmented with remux copy = %q, want REMUXED-BYTES", got)
	}

	_ = os.Remove(remuxPath)
	if got := serve(segmented); got != "SOURCE-BYTES" {
		t.Errorf("segmented without remux copy = %q, want SOURCE-BYTES", got)
	}

	if s.Remuxed("v1") {
		t.Error("Remuxed(v1) = true after removal, want false")
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
