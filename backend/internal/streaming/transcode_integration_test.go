//go:build integration

package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"homereel/backend/internal/domain"
)

// segmentRefRe matches segment file references inside an HLS playlist.
var segmentRefRe = regexp.MustCompile(`(?m)^([A-Za-z0-9_.-]+\.ts)$`)

// ffmpegPath returns the configured ffmpeg binary (env override for CI).
func ffmpegPath() string {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	return "ffmpeg"
}

func makeSample(t *testing.T, dir string) string {
	t.Helper()
	return makeSampleDur(t, dir, 3)
}

func makeSampleDur(t *testing.T, dir string, secs int) string {
	t.Helper()
	src := filepath.Join(dir, "sample.mkv")
	cmd := exec.Command(ffmpegPath(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+strconv.Itoa(secs),
		"-t", strconv.Itoa(secs), "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate sample: %v\n%s", err, out)
	}
	return src
}

func TestTranscodeProducesPlaylist(t *testing.T) {
	base := t.TempDir()
	src := makeSample(t, base)
	s := &Service{
		ffmpegPath: ffmpegPath(),
		hlsPreset:  "ultrafast",
		hlsDir:     filepath.Join(base, "hls"),
		active:     make(map[string]*transcode),
	}
	dir := filepath.Join(s.hlsDir, "v1")
	done := make(chan struct{})
	s.runTranscode(context.Background(), domain.Video{ID: "v1", Path: src}, dir, done)
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("transcode timed out")
	}

	if _, err := os.Stat(filepath.Join(dir, "master.m3u8")); err != nil {
		t.Fatalf("playlist missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".done")); err != nil {
		t.Fatalf("done marker missing: %v", err)
	}
	if !hlsComplete(dir) {
		t.Fatal("hlsComplete should be true after transcode")
	}
}

func TestMasterM3U8ServesCachedPlaylist(t *testing.T) {
	base := t.TempDir()
	src := makeSample(t, base)
	s := &Service{
		ffmpegPath: ffmpegPath(),
		hlsPreset:  "ultrafast",
		hlsDir:     filepath.Join(base, "hls"),
	}
	dir := filepath.Join(s.hlsDir, "v1")
	done := make(chan struct{})
	s.runTranscode(context.Background(), domain.Video{ID: "v1", Path: src}, dir, done)
	<-done

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/v1/hls/master.m3u8", nil)
	if err := s.MasterM3U8(rec, req, domain.Video{ID: "v1"}); err != nil {
		t.Fatalf("master: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("master status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "#EXTM3U") || !strings.Contains(body, "segment-00000") {
		t.Fatalf("master body unexpected: %s", body)
	}

	seg := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/stream/v1/hls/segment-00000.ts", nil)
	if err := s.Segment(seg, req2, domain.Video{ID: "v1"}, "segment-00000.ts"); err != nil {
		t.Fatalf("segment: %v", err)
	}
	if seg.Code != 200 || seg.Body.Len() == 0 {
		t.Fatalf("segment status/body = %d %d", seg.Code, seg.Body.Len())
	}
}

// TestMasterM3U8ServesGrowingPlaylist verifies the playlist is served while
// the transcode is still running: it must contain at least one segment and
// must not contain the ENDLIST marker until the transcode finishes.
func TestMasterM3U8ServesGrowingPlaylist(t *testing.T) {
	base := t.TempDir()
	src := makeSampleDur(t, base, 90)
	s := &Service{
		ffmpegPath: ffmpegPath(),
		hlsPreset:  "ultrafast",
		hlsDir:     filepath.Join(base, "hls"),
		active:     make(map[string]*transcode),
	}
	v := domain.Video{ID: "v1", Path: src}
	dir := filepath.Join(s.hlsDir, "v1")

	// Start the transcode and wait for the first servable playlist.
	firstRec := httptest.NewRecorder()
	firstReq := httptest.NewRequest("GET", "/api/stream/v1/hls/master.m3u8", nil)
	if err := s.MasterM3U8(firstRec, firstReq, v); err != nil {
		t.Fatalf("first master: %v", err)
	}
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first master status = %d", firstRec.Code)
	}
	firstBody := firstRec.Body.String()
	if !strings.Contains(firstBody, "#EXTINF") {
		t.Fatalf("served playlist has no segments: %s", firstBody)
	}

	// Poll the playlist a few times while the transcode may still be running;
	// every served snapshot must stay valid (segments present, atomic write).
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stream/v1/hls/master.m3u8", nil)
		if err := s.MasterM3U8(rec, req, v); err != nil {
			t.Fatalf("master poll %d: %v", i, err)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "#EXTINF") {
			t.Fatalf("poll %d served a playlist without segments: %q", i, body)
		}
		if !strings.HasPrefix(body, "#EXTM3U") {
			t.Fatalf("poll %d served a malformed playlist: %q", i, body)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Wait for completion and verify the final playlist is a complete VOD.
	deadline := time.Now().Add(120 * time.Second)
	for {
		if hlsComplete(dir) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transcode did not complete")
		}
		time.Sleep(200 * time.Millisecond)
	}
	data, err := os.ReadFile(filepath.Join(dir, "master.m3u8"))
	if err != nil {
		t.Fatalf("final playlist: %v", err)
	}
	final := string(data)
	if !strings.Contains(final, "#EXT-X-ENDLIST") {
		t.Fatalf("final playlist missing ENDLIST: %s", final)
	}
	if !strings.Contains(final, "segment-00000.ts") {
		t.Fatalf("final playlist missing first segment: %s", final)
	}
}

// TestSegmentsServableDuringTranscode plays the role of an HLS client: it
// repeatedly fetches the manifest while the transcode is still running and
// then fetches every segment the manifest lists. Any 404/locked-file would
// surface here (the atomic temp_file writes must guarantee segments are
// servable the moment the playlist references them).
func TestSegmentsServableDuringTranscode(t *testing.T) {
	base := t.TempDir()
	src := makeSampleDur(t, base, 90)
	s := &Service{
		ffmpegPath: ffmpegPath(),
		hlsPreset:  "ultrafast",
		hlsDir:     filepath.Join(base, "hls"),
		active:     make(map[string]*transcode),
	}
	v := domain.Video{ID: "v1", Path: src}

	started := make(chan struct{})
	go func() {
		close(started)
		// First call kicks off the single-flight transcode and returns the
		// first servable playlist.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stream/v1/hls/master.m3u8", nil)
		_ = s.MasterM3U8(rec, req, v)
	}()

	deadline := time.Now().Add(120 * time.Second)
	fetched := map[string]bool{}
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stream/v1/hls/master.m3u8", nil)
		if err := s.MasterM3U8(rec, req, v); err != nil {
			t.Fatalf("manifest fetch: %v", err)
		}
		body := rec.Body.String()
		for _, m := range segmentRefRe.FindAllStringSubmatch(body, -1) {
			name := m[1]
			if fetched[name] {
				continue
			}
			fetched[name] = true
			srec := httptest.NewRecorder()
			sreq := httptest.NewRequest("GET", "/api/stream/v1/hls/"+name, nil)
			if err := s.Segment(srec, sreq, v, name); err != nil {
				t.Fatalf("segment %s fetch failed mid-transcode: %v", name, err)
			}
			if srec.Code != http.StatusOK || srec.Body.Len() == 0 {
				t.Fatalf("segment %s status/body = %d/%d", name, srec.Code, srec.Body.Len())
			}
		}
		if hlsComplete(filepath.Join(s.hlsDir, "v1")) && len(fetched) > 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(fetched) < 2 {
		t.Fatalf("fetched only %d distinct segments during transcode: %v", len(fetched), fetched)
	}
}
