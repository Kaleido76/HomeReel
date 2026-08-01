//go:build integration

package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func haveFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
}

func TestProbeAndThumbnail(t *testing.T) {
	haveFFmpeg(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Generate a tiny test clip with ffmpeg.
	src := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=size=64x64:rate=1",
		"-t", "2", "-pix_fmt", "yuv420p", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate sample: %v\n%s", err, out)
	}

	info, err := Probe(ctx, "ffprobe", src)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if info.Codec != "h264" || info.Width != 64 || info.Height != 64 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Duration <= 0 {
		t.Fatalf("missing duration: %+v", info)
	}

	cover := filepath.Join(dir, "cover.jpg")
	thumb := filepath.Join(dir, "thumb.jpg")
	if err := Thumbnail(ctx, "ffmpeg", src, cover, thumb, info.Duration); err != nil {
		t.Fatalf("thumbnail: %v", err)
	}
	for _, p := range []string{cover, thumb} {
		if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
			t.Fatalf("thumbnail %s missing or empty: %v", p, err)
		}
	}
}
