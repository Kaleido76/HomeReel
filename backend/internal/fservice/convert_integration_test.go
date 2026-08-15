//go:build integration

package fservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func ffmpegBin() string {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	return "ffmpeg"
}

func ffprobeBin() string {
	if p := os.Getenv("FFPROBE_PATH"); p != "" {
		return p
	}
	return "ffprobe"
}

type noopReporter struct{}

func (noopReporter) Progress(float64)      {}
func (noopReporter) Subtask(string)        {}
func (noopReporter) SubtaskProgress(float64) {}

func makeSampleMkv(t *testing.T, dir, name string) string {
	t.Helper()
	src := filepath.Join(dir, name)
	cmd := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make sample: %v\n%s", err, out)
	}
	return src
}

func isH264Mp4(t *testing.T, path string) bool {
	t.Helper()
	cmd := exec.Command(ffprobeBin(), "-v", "error",
		"-select_streams", "v:0", "-show_entries", "stream=codec_name",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "h264"
}

// streamCodecs returns "codec_type:codec_name" lines for every stream,
// classifying tokens by value because ffprobe csv column order is not fixed.
func streamCodecs(t *testing.T, path string) []string {
	t.Helper()
	cmd := exec.Command(ffprobeBin(), "-v", "error",
		"-show_entries", "stream=codec_type,codec_name",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var res []string
	for _, line := range strings.Fields(strings.TrimSpace(string(out))) {
		typ, name := "", ""
		for _, tok := range strings.Split(line, ",") {
			switch t := strings.TrimSpace(tok); t {
			case "video", "audio", "subtitle", "attachment", "data":
				typ = t
			default:
				name = t
			}
		}
		if typ != "" {
			res = append(res, typ+":"+name)
		}
	}
	return res
}

func testService() *Service {
	return &Service{ffmpegPath: ffmpegBin(), ffprobePath: ffprobeBin()}
}

// TestConvertFileProducesFaststartMp4 verifies a single file is re-wrapped
// into a stream-copied h264 mp4 copy next to the source (never overwriting it).
func TestConvertFileProducesFaststartMp4(t *testing.T) {
	base := t.TempDir()
	src := makeSampleMkv(t, base, "sample.mkv")
	s := testService()
	if _, err := s.convertFile(context.Background(), src, ConvertParams{}, noopReporter{}); err != nil {
		t.Fatalf("convertFile: %v", err)
	}
	out := filepath.Join(base, "sample.mp4")
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if !isH264Mp4(t, out) {
		t.Fatal("output is not a playable h264 mp4")
	}
	for _, c := range streamCodecs(t, out) {
		if strings.HasPrefix(c, "audio:aac") {
			return
		}
	}
	t.Fatalf("output audio should stay AAC (browser-safe copy), got %v", streamCodecs(t, out))
}

// TestConvertDirCreatesSiblingCopy verifies a directory's direct-level videos
// land in a sibling " (MP4)" folder, one mp4 copy per source.
func TestConvertDirCreatesSiblingCopy(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	makeSampleMkv(t, dir, "ep1.mkv")
	makeSampleMkv(t, dir, "ep2.mkv")
	// Non-video files must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := testService()
	if _, err := s.convertDir(context.Background(), dir, ConvertParams{}, noopReporter{}); err != nil {
		t.Fatalf("convertDir: %v", err)
	}
	outDir := filepath.Join(base, "show (MP4)")
	if st, err := os.Stat(outDir); err != nil || !st.IsDir() {
		t.Fatalf("sibling folder missing: %v", err)
	}
	for _, f := range []string{"ep1.mp4", "ep2.mp4"} {
		p := filepath.Join(outDir, f)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("sibling output %s missing: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "note.mp4")); !os.IsNotExist(err) {
		t.Fatal("non-video file must not be converted")
	}
}

// TestConvertRebuildsAc3Audio guards the universal-audio rule: AC3 lacks a
// Dolby decoder in most browsers and Windows apps reject it ("unsupported AC3"),
// so a stream-copied AC3 track must never reach the output — it is rebuilt to
// AAC while video stays lossless.
func TestConvertRebuildsAc3Audio(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "ac3.mkv")
	cmd := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "192k", "-f", "matroska", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make ac3 sample: %v\n%s", err, out)
	}
	s := testService()
	dst := filepath.Join(base, "ac3.mp4")
	if err := s.runConvert(context.Background(), src, dst, ConvertParams{}, func(float64) {}); err != nil {
		t.Fatalf("runConvert: %v", err)
	}
	for _, c := range streamCodecs(t, dst) {
		if strings.HasPrefix(c, "audio:aac") {
			return
		}
	}
	t.Fatalf("AC3 audio should be rebuilt to AAC, got %v", streamCodecs(t, dst))
}

// TestConvertReencodePresets verifies the H.264/H.265 operations-panel presets
// re-encode the video to the requested codec (vs. the stream copy of 快速 MP4)
// and rebuild audio to AAC.
func TestConvertReencodePresets(t *testing.T) {
	base := t.TempDir()
	src := makeSampleMkv(t, base, "sample.mkv")
	for _, tc := range []struct {
		name     string
		params   ConvertParams
		expected string // ffprobe codec_name
	}{
		{"h264", ConvertParams{Video: "h264", VCRF: 20, Audio: "aac", AKbps: 128}, "h264"},
		{"h265", ConvertParams{Video: "h265", VCRF: 28, Audio: "aac", AKbps: 128}, "hevc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService()
			dst := filepath.Join(base, "out-"+tc.name+".mp4")
			if err := s.runConvert(context.Background(), src, dst, tc.params, func(float64) {}); err != nil {
				t.Fatalf("runConvert %s: %v", tc.name, err)
			}
			streams := streamCodecs(t, dst)
			var hasVideo, hasAac bool
			for _, c := range streams {
				switch {
				case c == "video:"+tc.expected:
					hasVideo = true
				case strings.HasPrefix(c, "audio:aac"):
					hasAac = true
				}
			}
			if !hasVideo {
				t.Fatalf("output video should be %s, got %v", tc.expected, streams)
			}
			if !hasAac {
				t.Fatalf("output audio should be aac, got %v", streams)
			}
		})
	}
}

// TestConvertReencodeBurnToggle verifies the subtitle handling of the re-encode
// presets: with burn=false a text subtitle survives as a mov_text track; with
// burn=true it is drawn into the picture and no subtitle track remains.
func TestConvertReencodeBurnToggle(t *testing.T) {
	base := t.TempDir()
	src := makeSampleMkv(t, base, "sample.mkv")
	sub := filepath.Join(base, "sub.srt")
	if err := os.WriteFile(sub, []byte("1\n00:00:00,000 --> 00:00:02,000\n字幕\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withSub := src + "sub.mkv"
	if out, err := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", src, "-i", sub, "-map", "0", "-map", "1",
		"-c", "copy", "-c:s", "srt", "-f", "matroska", withSub).CombinedOutput(); err != nil {
		t.Fatalf("attach subtitle: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		name   string
		burn   bool
		hasSub bool // whether a subtitle track should survive
	}{
		{"kept", false, true},
		{"burned", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService()
			dst := filepath.Join(base, "out-"+tc.name+".mp4")
			if err := s.runConvert(context.Background(), withSub, dst,
				ConvertParams{Video: "h264", VCRF: 19, Audio: "aac", Burn: tc.burn},
				func(float64) {}); err != nil {
				t.Fatalf("runConvert: %v", err)
			}
			streams := streamCodecs(t, dst)
			var hasSub bool
			for _, c := range streams {
				if strings.HasPrefix(c, "subtitle:") {
					hasSub = true
				}
			}
			if hasSub != tc.hasSub {
				t.Fatalf("subtitle track present = %v, want %v (%v)", hasSub, tc.hasSub, streams)
			}
		})
	}
}

// TestProbeConvertFacts verifies the operations-panel probe reads real stream
// facts: the video codec, non-universal audio codecs, and text subtitle tracks
// (without ever flagging a text subtitle as bitmap).
func TestProbeConvertFacts(t *testing.T) {
	base := t.TempDir()
	// AC3 audio sample — a non-universal codec the panel should warn about.
	src := filepath.Join(base, "ac3.mkv")
	cmd := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "192k", "-f", "matroska", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make ac3 sample: %v\n%s", err, out)
	}
	// Text-subtitle file built from the same sample.
	withSub := filepath.Join(base, "sub.mkv")
	sub := filepath.Join(base, "sub.srt")
	if err := os.WriteFile(sub, []byte("1\n00:00:00,000 --> 00:00:02,000\n字幕\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", src, "-i", sub, "-map", "0", "-map", "1",
		"-c", "copy", "-c:s", "srt", "-f", "matroska", withSub).CombinedOutput(); err != nil {
		t.Fatalf("attach subtitle: %v\n%s", err, out)
	}

	s := testService()
	results := s.ProbeConvert(context.Background(), []string{withSub, src})
	if len(results) != 2 {
		t.Fatalf("probe results = %d, want 2", len(results))
	}
	byPath := map[string]ConvertProbe{}
	for _, r := range results {
		byPath[r.Path] = r
	}
	subbed := byPath[withSub]
	if subbed.VideoCodec != "h264" {
		t.Fatalf("video codec = %q, want h264", subbed.VideoCodec)
	}
	if !containsStr(subbed.SubtitleCodecs, "subrip") {
		t.Fatalf("subtitle codecs = %v, want subrip", subbed.SubtitleCodecs)
	}
	if subbed.HasBitmapSubtitle {
		t.Fatal("srt subtitle flagged as bitmap")
	}
	ac3 := byPath[src]
	if !containsStr(ac3.AudioCodecs, "ac3") {
		t.Fatalf("audio codecs = %v, want ac3", ac3.AudioCodecs)
	}
	if ac3.HasBitmapSubtitle {
		t.Fatal("ac3 sample flagged as bitmap subtitle")
	}
	// The ac3 sample has no subtitle stream: its list must be an empty array,
	// never nil — JSON null would crash the frontend's .length reads.
	if ac3.SubtitleCodecs == nil {
		t.Fatalf("subtitle codecs must be non-nil empty array, got nil")
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestConvertBurnsSubtitleAndRebuildsAudio verifies the lossy fallback: a file
// that cannot be stream-copied into mp4 (theora video) is re-encoded to h264,
// its non-browser audio (vorbis) is rebuilt to AAC, and its subtitle track is
// burned into the picture (no stray subtitle track survives in the mp4).
func TestConvertBurnsSubtitleAndRebuildsAudio(t *testing.T) {
	base := t.TempDir()
	// A theora+vorbis mkv: mp4 cannot mux theora (so lossless copy fails) and
	// vorbis is not browser-playable in mp4 (so it must be re-encoded).
	src := filepath.Join(base, "old.mkv")
	cmd := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "3", "-c:v", "libtheora", "-c:a", "libvorbis",
		"-f", "matroska", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make theora sample: %v\n%s", err, out)
	}
	sub := filepath.Join(base, "sub.srt")
	if err := os.WriteFile(sub, []byte("1\n00:00:00,000 --> 00:00:02,000\n字幕\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", src, "-i", sub, "-map", "0", "-map", "1",
		"-c", "copy", "-c:s", "srt", "-f", "matroska", src+"sub.mkv").CombinedOutput(); err != nil {
		t.Fatalf("attach subtitle: %v\n%s", err, out)
	}

	s := testService()
	dst := filepath.Join(base, "old.mp4")
	var lastPct float64
	if err := s.runConvert(context.Background(), src+"sub.mkv", dst, ConvertParams{}, func(f float64) { lastPct = f }); err != nil {
		t.Fatalf("runConvert fallback: %v", err)
	}
	if lastPct != 1 {
		t.Fatalf("progress should reach 1, got %v", lastPct)
	}
	streams := streamCodecs(t, dst)
	var hasH264, hasAac, hasSub bool
	for _, c := range streams {
		switch {
		case c == "video:h264":
			hasH264 = true
		case strings.HasPrefix(c, "audio:aac"):
			hasAac = true
		case strings.HasPrefix(c, "subtitle:"):
			hasSub = true
		}
	}
	if !hasH264 || !hasAac {
		t.Fatalf("fallback output should be h264 video + aac audio, got %v", streams)
	}
	if hasSub {
		t.Fatalf("subtitle should be burned into the picture, not a leftover track: %v", streams)
	}
	// Prove the subtitle is burned, not dropped: a frame from the burned output
	// must differ from a plain re-encode of the same source (the subtitle text
	// changes the pixels). runConvert succeeding already implies the subtitles
	// filter rendered (a missing stream would fail the command), this locks it.
	plain := filepath.Join(base, "plain.mp4")
	if out, err := exec.Command(ffmpegBin(),
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src+"sub.mkv",
		"-map", "0:v:0", "-c:v", "libx264", "-crf", "19", "-pix_fmt", "yuv420p",
		"-f", "mp4", plain).CombinedOutput(); err != nil {
		t.Fatalf("plain re-encode: %v\n%s", err, out)
	}
	if frameMD5(t, dst, "0.5") == frameMD5(t, plain, "0.5") {
		t.Fatal("burned output frame identical to no-subtitle re-encode — subtitle not burned")
	}
}

// frameMD5 hashes the decoded frame at ts, used to tell burned subtitles apart.
func frameMD5(t *testing.T, path, ts string) string {
	t.Helper()
	out, err := exec.Command(ffmpegBin(),
		"-hide_banner", "-loglevel", "error", "-y",
		"-ss", ts, "-i", path, "-frames:v", "1", "-f", "md5", "-").Output()
	if err != nil {
		t.Fatalf("frame md5 %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}
