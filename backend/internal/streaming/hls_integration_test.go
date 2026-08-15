//go:build integration

package streaming

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
)

// TestHLSTranscodeSegmentsPlayable verifies the on-demand transcode segmenter
// (ADR-006 修订): a segment is re-encoded to h264/AAC, its content starts
// exactly at the segment's keyframe on the source timeline (so adjacent
// segments tile without gaps or overlaps), and it decodes cleanly.
func TestHLSTranscodeSegmentsPlayable(t *testing.T) {
	base := t.TempDir()
	src := makeHLSMkv(t, base, "hevc.mkv", "hevc")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v2", Path: src, Duration: 30, Codec: "hevc", AudioCodec: "aac"}

	hs := s.hls.session(v, "tok", 0)
	if err := s.ensureKeyframes(context.Background(), hs, v); err != nil {
		t.Fatalf("ensureKeyframes: %v", err)
	}
	if len(hs.keyframes) < 3 {
		t.Fatalf("expected multiple keyframes, got %v", hs.keyframes)
	}

	out0 := filepath.Join(hs.dir, "seg-0.ts")
	out1 := filepath.Join(hs.dir, "seg-1.ts")
	if err := s.generateSegment(context.Background(), v, hs, 0, out0); err != nil {
		t.Fatalf("gen seg 0: %v", err)
	}
	if err := s.generateSegment(context.Background(), v, hs, 1, out1); err != nil {
		t.Fatalf("gen seg 1: %v", err)
	}
	for _, c := range streamCodecsHLSTest(t, out0) {
		if strings.HasPrefix(c, "video:h264") {
			goto haveVideo
		}
	}
	t.Fatalf("transcode segment should output h264 video, got %v", streamCodecsHLSTest(t, out0))
haveVideo:

	// PTS on the source timeline: seg0 starts at keyframe[0], seg1 at keyframe[1]
	// (transcode seeks accurately, so the content starts exactly at the keyframe).
	pts0 := hlsVideoPTS(t, out0)
	pts1 := hlsVideoPTS(t, out1)
	if pts0[0] < hs.keyframes[0]-0.1 || pts0[0] > hs.keyframes[0]+0.2 {
		t.Errorf("seg0 first PTS = %v, want ~keyframe[0]=%v", pts0[0], hs.keyframes[0])
	}
	if pts1[0] < hs.keyframes[1]-0.1 || pts1[0] > hs.keyframes[1]+0.2 {
		t.Errorf("seg1 first PTS = %v, want ~keyframe[1]=%v", pts1[0], hs.keyframes[1])
	}
	// Adjacent segments must not drift: seg0's last PTS precedes seg1's first.
	// A one-frame B-frame reorder tail at the boundary (the frame whose decode
	// time falls inside seg0 but displays just past it) is expected in MPEG-TS
	// and harmless — hls.js aligns segments by the accumulated EXTINF timeline.
	if pts0[len(pts0)-1] > pts1[0]+0.1 {
		t.Errorf("PTS drift: seg0 last=%v should be ~seg1 first=%v", pts0[len(pts0)-1], pts1[0])
	}
}

// TestRemuxMkvToPlayableMp4 verifies the remux tier (ADR-006 修订): an MKV with
// browser-decodable streams is stream-copied once into a faststart MP4 that
// keeps the h264/aac streams and is served over HTTP Range.
func TestRemuxMkvToPlayableMp4(t *testing.T) {
	base := t.TempDir()
	src := makeHLSMkv(t, base, "h264aac.mkv", "h264")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v1", Path: src, Duration: 30, Codec: "h264", AudioCodec: "aac"}

	if !s.RemuxPlayable(v) {
		t.Fatalf("h264+aac should be remuxable")
	}
	out, err := s.remuxPath(context.Background(), v, 0)
	if err != nil {
		t.Fatalf("remuxPath: %v", err)
	}
	if _, err := filepath.Glob(out); err != nil || !fileExists(t, out) {
		t.Fatalf("remuxed MP4 missing: %s", out)
	}
	var haveVideo, haveAAC bool
	for _, c := range streamCodecsHLSTest(t, out) {
		if strings.HasPrefix(c, "video:h264") {
			haveVideo = true
		}
		if c == "audio:aac" {
			haveAAC = true
		}
	}
	if !haveVideo || !haveAAC {
		t.Fatalf("remuxed MP4 stream copy lost streams, got %v", streamCodecsHLSTest(t, out))
	}
	// Second request reuses the cache without regenerating.
	again, err := s.remuxPath(context.Background(), v, 0)
	if err != nil {
		t.Fatalf("cached remuxPath: %v", err)
	}
	if again != out {
		t.Fatalf("cached path = %q, want %q", again, out)
	}
}

// TestHLSReencodesNonCopyableAudio verifies that a video whose audio cannot be
// stream-copied into a playable MP4 (AC3/PCM/DTS — no Dolby decoder in browsers)
// is not remuxable and instead goes through the HLS transcode tier, whose
// per-segment audio re-encode turns it into AAC. Re-encoding a whole
// feature-length audio track is too slow for the whole-file remux tier, so the
// transcode tier's per-segment approach is what makes these files start fast.
func TestHLSReencodesNonCopyableAudio(t *testing.T) {
	base := t.TempDir()
	src := makeMkvWithAudio(t, base, "h264ac3.mkv", "h264", "ac3")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v1", Path: src, Duration: 30, Codec: "h264", AudioCodec: "ac3"}

	if s.RemuxPlayable(v) {
		t.Fatalf("h264+ac3 should not be remuxable (audio needs re-encode)")
	}
	if !s.TranscodePlayable(v) {
		t.Fatalf("h264+ac3 should be transcodeable")
	}
	hs := s.hls.session(v, "tok", 0)
	if err := s.ensureKeyframes(context.Background(), hs, v); err != nil {
		t.Fatalf("ensureKeyframes: %v", err)
	}
	out := filepath.Join(hs.dir, "seg-0.ts")
	if err := s.generateSegment(context.Background(), v, hs, 0, out); err != nil {
		t.Fatalf("gen seg: %v", err)
	}
	var haveAAC, haveAC3 bool
	for _, c := range streamCodecsHLSTest(t, out) {
		if c == "audio:aac" {
			haveAAC = true
		}
		if strings.HasPrefix(c, "audio:ac3") {
			haveAC3 = true
		}
	}
	if !haveAAC || haveAC3 {
		t.Fatalf("hls segment should re-encode ac3 to aac, got %v", streamCodecsHLSTest(t, out))
	}
}

// makeDualAudioMkv builds an MKV with one video stream and two audio tracks of
// the given codecs/titles, so track selection can be exercised (each track
// comes from a different sine source).
func makeDualAudioMkv(t *testing.T, dir, name, vcodec, acodec0, acodec1, title0, title1 string) string {
	t.Helper()
	src := filepath.Join(dir, name)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-f", "lavfi", "-i", "sine=frequency=880",
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0",
		"-t", "30",
		"-c:v", vcodec, "-pix_fmt", "yuv420p",
		"-g", "50", "-keyint_min", "50", "-sc_threshold", "0",
		"-c:a:0", acodec0, "-b:a:0", "64k",
		"-c:a:1", acodec1, "-b:a:1", "64k",
		"-metadata:s:a:0", "title=" + title0,
		"-metadata:s:a:1", "title=" + title1,
		src,
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make dual-audio sample: %v\n%s", err, out)
	}
	return src
}

// TestAudioTrackListLabels verifies the track enumeration the player menu is
// built from: multi-track containers list every audio track with a readable
// label (title tags win).
func TestAudioTrackListLabels(t *testing.T) {
	base := t.TempDir()
	src := makeDualAudioMkv(t, base, "dual.mkv", "h264", "aac", "mp3", "国语", "粤语")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v1", Path: src}

	tracks := s.ListAudioTracks(context.Background(), v)
	if len(tracks) != 2 {
		t.Fatalf("audio tracks = %d, want 2", len(tracks))
	}
	if tracks[0].Label != "国语" || tracks[1].Label != "粤语" {
		t.Fatalf("track labels = %q / %q, want 国语/粤语", tracks[0].Label, tracks[1].Label)
	}
	if tracks[0].Index != 0 || tracks[1].Index != 1 {
		t.Fatalf("track indices = %d / %d, want audio ordinals 0/1", tracks[0].Index, tracks[1].Index)
	}
}

// TestRemuxSelectsAudioTrack verifies the remux tier maps the requested track:
// aac for track 0, mp3 for track 1, cached in separate per-track files.
func TestRemuxSelectsAudioTrack(t *testing.T) {
	base := t.TempDir()
	src := makeDualAudioMkv(t, base, "dual.mkv", "h264", "aac", "mp3", "A", "B")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v1", Path: src, Duration: 30, Codec: "h264", AudioCodec: "aac"}

	if !s.RemuxPlayable(v) {
		t.Fatal("h264+aac should be remuxable")
	}
	out0, err := s.remuxPath(context.Background(), v, 0)
	if err != nil {
		t.Fatalf("remux track 0: %v", err)
	}
	out1, err := s.remuxPath(context.Background(), v, 1)
	if err != nil {
		t.Fatalf("remux track 1: %v", err)
	}
	if out0 == out1 {
		t.Fatalf("tracks must remux to separate cache files: %s", out0)
	}
	remuxDir := filepath.Join(s.dataDir, "remux")
	if !fileExists(t, filepath.Join(remuxDir, "v1.mp4")) || !fileExists(t, filepath.Join(remuxDir, "v1-a1.mp4")) {
		t.Fatal("per-track remux caches missing (v1.mp4 / v1-a1.mp4)")
	}
	var haveAAC, haveMP3 bool
	for _, c := range streamCodecsHLSTest(t, out0) {
		if c == "audio:aac" {
			haveAAC = true
		}
	}
	for _, c := range streamCodecsHLSTest(t, out1) {
		if c == "audio:mp3" {
			haveMP3 = true
		}
	}
	if !haveAAC {
		t.Fatalf("track 0 remux should keep aac, got %v", streamCodecsHLSTest(t, out0))
	}
	if !haveMP3 {
		t.Fatalf("track 1 remux should keep mp3, got %v", streamCodecsHLSTest(t, out1))
	}
}

// TestHLSSelectsAudioTrack verifies the HLS transcode tier transcodes the
// requested track: both track 0 and track 1 sessions produce AAC segments.
func TestHLSSelectsAudioTrack(t *testing.T) {
	base := t.TempDir()
	src := makeDualAudioMkv(t, base, "dual.mkv", "h264", "aac", "mp3", "A", "B")
	s := New(nil, t.TempDir(), "ffmpeg", "ffprobe")
	v := domain.Video{ID: "v1", Path: src, Duration: 30, Codec: "h264", AudioCodec: "aac"}

	for _, want := range []int{0, 1} {
		hs := s.hls.session(v, fmt.Sprintf("tok%d", want), want)
		if err := s.ensureKeyframes(context.Background(), hs, v); err != nil {
			t.Fatalf("ensureKeyframes track %d: %v", want, err)
		}
		out := filepath.Join(hs.dir, "seg-0.ts")
		if err := s.generateSegment(context.Background(), v, hs, 0, out); err != nil {
			t.Fatalf("gen seg track %d: %v", want, err)
		}
		var haveAAC bool
		for _, c := range streamCodecsHLSTest(t, out) {
			if c == "audio:aac" {
				haveAAC = true
			}
		}
		if !haveAAC {
			t.Fatalf("track %d segment should be aac, got %v", want, streamCodecsHLSTest(t, out))
		}
	}
}

func makeHLSMkv(t *testing.T, dir, name, vcodec string) string {
	t.Helper()
	return makeMkvWithAudio(t, dir, name, vcodec, "aac")
}

func makeMkvWithAudio(t *testing.T, dir, name, vcodec, acodec string) string {
	t.Helper()
	src := filepath.Join(dir, name)
	audio := []string{"-c:a", acodec}
	if strings.HasPrefix(acodec, "pcm_") || acodec == "ac3" {
		audio = append(audio, "-ar", "48000")
	} else {
		audio = append(audio, "-b:a", "64k")
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "30",
		"-c:v", vcodec, "-pix_fmt", "yuv420p",
		"-g", "50", "-keyint_min", "50", "-sc_threshold", "0",
	}
	args = append(args, audio...)
	args = append(args, src)
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make sample: %v\n%s", err, out)
	}
	return src
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func streamCodecsHLSTest(t *testing.T, path string) []string {
	t.Helper()
	cmd := exec.Command("ffprobe", "-v", "error",
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
			switch t2 := strings.TrimSpace(tok); t2 {
			case "video", "audio", "subtitle", "attachment", "data":
				typ = t2
			default:
				name = t2
			}
		}
		if typ != "" {
			res = append(res, typ+":"+name)
		}
	}
	return res
}

func hlsVideoPTS(t *testing.T, path string) []float64 {
	t.Helper()
	cmd := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe pts %s: %v", path, err)
	}
	var res []float64
	for _, tok := range strings.Fields(strings.TrimSpace(string(out))) {
		var f float64
		if _, err := fmt.Sscanf(tok, "%f", &f); err == nil {
			res = append(res, f)
		}
	}
	return res
}
