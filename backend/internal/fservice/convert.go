package fservice

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
)

// ConvertParams drives a format-factory conversion: which video codec to use
// (stream copy, h264, h265), the re-encode quality (CRF), how audio is handled,
// and whether the first subtitle is burned into the picture. Empty fields fall
// back to the fast-MP4 defaults (stream copy + smart audio).
type ConvertParams struct {
	Video string `json:"video"` // copy | h264 | h265; empty = copy
	VCRF  int    `json:"vcrf"`  // re-encode quality (0 = preset default)
	Audio string `json:"audio"` // smart | copy | aac; empty = smart
	AKbps int    `json:"akbps"` // AAC bitrate (0 = 192)
	Burn  bool   `json:"burn"`  // burn the first subtitle into the picture
}

// norm fills defaults and clamps out-of-range values so an arbitrary client
// payload can never produce a nonsensical ffmpeg invocation.
func (p ConvertParams) norm() ConvertParams {
	if p.Video != "h264" && p.Video != "h265" {
		p.Video = "copy"
	}
	if p.Audio != "copy" && p.Audio != "aac" {
		p.Audio = "smart"
	}
	if p.AKbps < 64 || p.AKbps > 512 {
		p.AKbps = 192
	}
	if p.VCRF < 0 || p.VCRF > 51 {
		p.VCRF = 0
	}
	if p.Video == "h264" && p.VCRF == 0 {
		p.VCRF = 19
	}
	if p.Video == "h265" && p.VCRF == 0 {
		p.VCRF = 23
	}
	return p
}

// convertMeta is the job payload of TypeConvert: the absolute source path to
// turn into a faststart MP4 copy (a file or a directory of videos) plus the
// conversion parameters chosen in the operations panel.
type convertMeta struct {
	Path   string        `json:"path"`
	Params ConvertParams `json:"params"`
}

// ConvertJob reports one enqueued conversion unit and its intended output.
// The actual output may still bump to a " (N)" suffix at run time when the
// name collides with an existing file.
type ConvertJob struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // file | dir
	JobID  string `json:"job_id"`
	Output string `json:"output"`
}

// EnqueueConvert validates each selected path and enqueues one convert job per
// unit (a video file → an mp4 copy next to it; a directory → a sibling " (MP4)"
// folder holding the converted copies of its direct-level video files).
// Per-item validation failures are returned separately so the caller can show
// exactly which selection could not be queued.
func (s *Service) EnqueueConvert(ctx context.Context, paths []string, params ConvertParams) ([]ConvertJob, []OpError) {
	var out []ConvertJob
	var errs []OpError
	for _, p := range paths {
		clean := filepath.Clean(p)
		info, err := os.Stat(clean)
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		} else if !files.IsConvertible(clean) {
			errs = append(errs, OpError{Path: p, Message: "不是可转换的媒体文件"})
			continue
		}
		extra, err := json.Marshal(convertMeta{Path: clean, Params: params})
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		id, err := s.jobs.Enqueue(ctx, jobs.TypeConvert, clean, "转换 · "+filepath.Base(clean), string(extra))
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		out = append(out, ConvertJob{
			Path:   clean,
			Kind:   kind,
			JobID:  id,
			Output: intendedOutput(clean, info.IsDir()),
		})
	}
	return out, errs
}

// HandleConvert runs one TypeConvert job.
func (s *Service) HandleConvert(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	var meta convertMeta
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.Path == "" {
		return errors.New("convert job missing path")
	}
	params := meta.Params.norm()
	info, err := os.Stat(meta.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return s.convertDir(ctx, meta.Path, params, report)
	}
	return s.convertFile(ctx, meta.Path, params, report)
}

// convertFile creates a faststart MP4 copy of one video next to the source,
// with the same base name and a " (N)" suffix if the target name is taken.
func (s *Service) convertFile(ctx context.Context, src string, params ConvertParams, report jobs.Reporter) error {
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	dst := filepath.Join(filepath.Dir(src), freeName(filepath.Dir(src), stem, ".mp4"))
	report.Subtask("转换 " + filepath.Base(src))
	return s.runConvert(ctx, src, dst, params, func(f float64) { report.Progress(f) })
}

// convertDir creates a sibling " (MP4)" folder of the source directory and
// converts every direct-level video file inside it (non-recursive). Overall
// progress advances by file count; per-file failures are collected into one
// job error so a partially successful batch still shows in the task panel.
func (s *Service) convertDir(ctx context.Context, dir string, params ConvertParams, report jobs.Reporter) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var videos []string
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		full := filepath.Join(dir, de.Name())
		if files.IsConvertible(full) {
			videos = append(videos, full)
		}
	}
	if len(videos) == 0 {
		return fmt.Errorf("目录下没有可转换的文件：%s", filepath.Base(dir))
	}
	outDir := filepath.Join(filepath.Dir(dir), freeName(filepath.Dir(dir), filepath.Base(dir), " (MP4)"))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var errs []string
	for i, v := range videos {
		if err := ctx.Err(); err != nil {
			return err
		}
		report.Subtask(fmt.Sprintf("转换 %s（%d/%d）", filepath.Base(v), i+1, len(videos)))
		base := float64(i) / float64(len(videos))
		dst := filepath.Join(outDir, freeName(outDir, strings.TrimSuffix(filepath.Base(v), filepath.Ext(v)), ".mp4"))
		if err := s.runConvert(ctx, v, dst, params, func(f float64) {
			report.Progress(base + f/float64(len(videos)))
		}); err != nil {
			errs = append(errs, filepath.Base(v)+": "+err.Error())
		}
	}
	report.Progress(1)
	return collectErrors(errs)
}

// sourceProbe is the ffprobe snapshot that decides how a source is converted:
// its duration (for progress/ETA), whether it carries audio (and whether that
// audio can be stream-copied without breaking playback), and whether it carries
// a subtitle stream to burn in.
type sourceProbe struct {
	duration      float64
	hasAudio      bool
	audioCopyable bool
	hasSubtitle   bool
}

// universalMp4Audio are the audio codecs that every target player (Chrome,
// Edge, Safari, Windows Media Player, VLC…) decodes inside an mp4. Anything
// else — including AC3/EAC3, which Chrome plays but Windows apps report as
// "unsupported" — is re-encoded to AAC so the produced file never comes out
// silent or unplayable on some device.
var universalMp4Audio = map[string]bool{
	"aac": true, "mp3": true,
}

// ffprobeProbe is the JSON shape of the ffprobe call used by probeSource.
type ffprobeProbe struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// streamProbe is the raw ffprobe snapshot of a source: its duration, the first
// video codec, and the full audio/subtitle codec lists. It feeds both the
// conversion strategy (probeSource) and the operations-panel probe endpoint.
type streamProbe struct {
	duration       float64
	videoCodec     string
	audioCodecs    []string
	subtitleCodecs []string
}

// probeStreams reads the stream list and duration of src in one ffprobe call.
func (s *Service) probeStreams(ctx context.Context, src string) (streamProbe, error) {
	out, err := exec.CommandContext(ctx, s.ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type,codec_name",
		"-of", "json",
		src).Output()
	if err != nil {
		return streamProbe{}, fmt.Errorf("ffprobe: %w", err)
	}
	var pr ffprobeProbe
	if err := json.Unmarshal(out, &pr); err != nil {
		return streamProbe{}, fmt.Errorf("parse ffprobe: %w", err)
	}
	// Codec lists start non-nil so probeOne can hand them to JSON as arrays —
	// a missing stream type must never serialize to null.
	p := streamProbe{
		audioCodecs:    []string{},
		subtitleCodecs: []string{},
	}
	if d, err := strconv.ParseFloat(pr.Format.Duration, 64); err == nil && d > 0 {
		p.duration = d
	}
	for _, st := range pr.Streams {
		switch st.CodecType {
		case "video":
			if p.videoCodec == "" {
				p.videoCodec = st.CodecName
			}
		case "audio":
			p.audioCodecs = append(p.audioCodecs, st.CodecName)
		case "subtitle":
			p.subtitleCodecs = append(p.subtitleCodecs, st.CodecName)
		}
	}
	return p, nil
}

// probeSource reduces a streamProbe to the conversion strategy: whether audio
// exists (and whether the primary audio stream is universally playable), and
// whether any subtitle stream exists to burn in.
func (s *Service) probeSource(ctx context.Context, src string) (sourceProbe, error) {
	st, err := s.probeStreams(ctx, src)
	if err != nil {
		return sourceProbe{}, err
	}
	p := sourceProbe{duration: st.duration}
	if len(st.audioCodecs) > 0 {
		p.hasAudio = true
		p.audioCopyable = universalMp4Audio[st.audioCodecs[0]]
	}
	p.hasSubtitle = len(st.subtitleCodecs) > 0
	return p, nil
}

// estimatedDuration guesses a total duration from the file size when ffprobe
// reports none (a rough ~8 Mbps average total bitrate), so a conversion whose
// length is unknown still gets a determinate progress bar and a time estimate
// instead of spinning blindly.
func estimatedDuration(src string) float64 {
	if info, err := os.Stat(src); err == nil && info.Size() > 0 {
		return float64(info.Size()) * 8 / 8_000_000
	}
	return 0
}

// runConvert turns src into a browser-friendly faststart MP4 copy at dst,
// reporting the per-file completion fraction [0,1] through step. The actual
// ffmpeg invocation is chosen from the params by convertAttempts; each attempt
// is tried in order until one succeeds, and only the last error is returned.
func (s *Service) runConvert(ctx context.Context, src, dst string, params ConvertParams, step func(float64)) error {
	probe, err := s.probeSource(ctx, src)
	if err != nil {
		// ffprobe missing/failed: fall back to a blind lossless copy; the
		// fallback attempts below still rescue files it cannot handle.
		probe = sourceProbe{audioCopyable: true}
	}
	// ffprobe's duration is the source of truth for progress: out_time/duration
	// is exact. Only when it is missing does an estimate or the size signal step
	// in.
	realDur := probe.duration > 0
	total := probe.duration
	if !realDur {
		total = estimatedDuration(src)
	}
	var inputSize int64
	if info, err := os.Stat(src); err == nil {
		inputSize = info.Size()
	}
	// A stream copy lands at roughly the source size (same streams, container
	// overhead ±a few %), so total_size/inputSize is an accurate fallback for
	// files whose duration is unknown — no arbitrary ratio.
	p := params.norm()
	var lastErr error
	for _, attempt := range s.convertAttempts(probe, src, p) {
		expected := int64(0)
		if attempt.streamCopy {
			expected = inputSize
		}
		if err := s.runConvertArgs(ctx, src, dst, attempt.args, total, expected, realDur, step); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		} else {
			lastErr = err
		}
	}
	slog.Warn("convert failed", "src", src, "dst", dst, "params", p, "err", lastErr)
	return fmt.Errorf("ffmpeg 转换失败：%w", lastErr)
}

// convertAttempt is one concrete ffmpeg invocation: the raw codec/filter args
// and whether it is a stream copy (whose output size tracks the source, which
// makes total_size a usable progress signal).
type convertAttempt struct {
	args       []string
	streamCopy bool
}

// convertAttempts lays out the ordered ffmpeg attempts for the given params,
// most desirable first; runConvert tries each until one succeeds.
//
// Stream copy (快速 MP4): every frame is copied bit-for-bit and text subtitles
// are re-wrapped to mov_text. mp4 cannot hold bitmap subtitles (PGS/VobSub are
// exactly why people keep MKVs), so when the copy fails the video is re-encoded
// to h264 with the first subtitle burned into the picture (libass) and audio
// kept or rebuilt to AAC.
//
// Re-encode (H.264/H.265 MP4): the primary video is re-encoded at the requested
// CRF. With burn it draws the first subtitle into the picture; otherwise text
// subtitles are kept as mov_text, falling back to dropping them entirely when
// the source carries bitmap subtitles mp4 cannot hold.
func (s *Service) convertAttempts(p sourceProbe, src string, params ConvertParams) []convertAttempt {
	if params.Video == "h264" || params.Video == "h265" {
		if params.Burn {
			return []convertAttempt{{args: s.reencodeArgs(p, src, params, true, false)}}
		}
		return []convertAttempt{
			{args: s.reencodeArgs(p, src, params, false, false)},
			{args: s.reencodeArgs(p, src, params, false, true)},
		}
	}
	attempts := []convertAttempt{{args: s.losslessArgs(p, params), streamCopy: true}}
	attempts = append(attempts, convertAttempt{args: s.burnArgs(p, src, params)})
	return attempts
}

// losslessArgs maps every stream with -c copy, keeping text subtitles as
// mov_text; audio follows the params (see audioSelector).
func (s *Service) losslessArgs(p sourceProbe, params ConvertParams) []string {
	args := []string{"-map", "0", "-c", "copy", "-c:s", "mov_text"}
	return append(args, s.audioSelector(p, params)...)
}

// reencodeArgs re-encodes the primary video to h264/h265 at the requested CRF
// with yuv420p output. Audio follows the params. With burn the first subtitle
// track is drawn into the picture and no subtitle stream is mapped; otherwise
// text subtitles are wrapped to mov_text (dropSubs retries without any subtitle
// stream for sources with bitmap subtitles mp4 cannot hold).
func (s *Service) reencodeArgs(p sourceProbe, src string, params ConvertParams, burn, dropSubs bool) []string {
	codec := "libx264"
	if params.Video == "h265" {
		codec = "libx265"
	}
	args := []string{"-map", "0:v:0"}
	if burn && p.hasSubtitle {
		args = append(args, "-vf", "subtitles='"+escapeFilterPath(src)+"':si=0")
	}
	args = append(args, "-c:v", codec, "-crf", strconv.Itoa(params.VCRF), "-pix_fmt", "yuv420p")
	args = append(args, "-map", "0:a?")
	args = append(args, s.audioSelector(p, params)...)
	if burn {
		return args
	}
	if dropSubs {
		args = append(args, "-sn")
	} else {
		args = append(args, "-map", "0:s?")
		args = append(args, "-c:s", "mov_text")
	}
	return args
}

// burnArgs is the stream-copy fallback: the primary video is re-encoded to
// h264 (near-transparent CRF 19) and the first subtitle track is burned into
// the picture when one exists. Audio follows the params.
func (s *Service) burnArgs(p sourceProbe, src string, params ConvertParams) []string {
	args := []string{"-map", "0:v:0"}
	if p.hasSubtitle {
		args = append(args, "-vf", "subtitles='"+escapeFilterPath(src)+"':si=0")
	}
	args = append(args, "-c:v", "libx264", "-crf", "19", "-pix_fmt", "yuv420p")
	args = append(args, "-map", "0:a?")
	return append(args, s.audioSelector(p, params)...)
}

// audioSelector returns the -c:a args for the params' audio choice. "smart"
// copies only universally-playable codecs (aac/mp3) and rebuilds everything
// else to AAC — AC3/EAC3/Opus decode fine in Chrome but Windows apps report
// them as unsupported, so copying them would ship a file that plays silent on
// half the machines.
func (s *Service) audioSelector(p sourceProbe, params ConvertParams) []string {
	kbps := strconv.Itoa(params.AKbps)
	switch params.Audio {
	case "aac":
		return []string{"-c:a", "aac", "-b:a", kbps + "k"}
	case "copy":
		if p.hasAudio {
			return []string{"-c:a", "copy"}
		}
		return nil
	default: // smart
		if p.hasAudio && !p.audioCopyable {
			return []string{"-c:a", "aac", "-b:a", kbps + "k"}
		}
		if p.hasAudio {
			return []string{"-c:a", "copy"}
		}
		return nil
	}
}

// escapeFilterPath renders a Windows path safe for the ffmpeg subtitles filter:
// forward slashes plus an escaped drive colon (C\:/Users/…), wrapped in single
// quotes by the caller.
func escapeFilterPath(p string) string {
	return strings.ReplaceAll(filepath.ToSlash(p), ":", "\\:")
}

// runConvertArgs runs one ffmpeg invocation that writes a faststart MP4 to
// dst (via a temp file + atomic rename). extra carries the stream mapping /
// codec / filter options; the muxer is pinned with -f mp4 because the temp
// output has no useful extension. Progress is read from ffmpeg's -progress
// stream, preferring the exact signal over a heuristic, in this order:
//  1. out_time / duration — exact, when ffprobe reported a real duration;
//  2. total_size / expectedSize — accurate for a stream copy (~1:1 with the
//     source), used when the duration is unknown;
//  3. out_time / estimated duration — rough last resort.
func (s *Service) runConvertArgs(ctx context.Context, src, dst string, extra []string, total float64, expectedSize int64, realDur bool, step func(float64)) error {
	tmp := dst + ".tmp"
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
	}
	args = append(args, extra...)
	args = append(args, "-progress", "pipe:1", "-f", "mp4", "-movflags", "+faststart", tmp)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if step != nil {
		scanOut := bufio.NewScanner(stdout)
		var outTime, totalSize float64 = -1, -1
		for scanOut.Scan() {
			line := scanOut.Text()
			if val, ok := strings.CutPrefix(line, "out_time_us="); ok {
				if us, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil && us >= 0 {
					outTime = float64(us) / 1e6
				}
			} else if val, ok := strings.CutPrefix(line, "total_size="); ok {
				if n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil && n >= 0 {
					totalSize = float64(n)
				}
			} else {
				continue
			}
			var frac float64 = -1
			switch {
			case realDur && outTime >= 0:
				frac = min(outTime/total, 1)
			case expectedSize > 0 && totalSize >= 0:
				frac = min(totalSize/float64(expectedSize), 1)
			case !realDur && outTime >= 0 && total > 0:
				frac = min(outTime/total, 1)
			}
			if frac >= 0 {
				step(frac)
			}
		}
	} else {
		_, _ = io.Copy(io.Discard, stdout)
	}
	if err := cmd.Wait(); err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %w（%s）", err, truncate(stderr.String(), 300))
	}
	// The last -progress report can stop short of the total; pin the fraction
	// to 1 so the bar reaches 100% the moment the conversion finishes.
	if step != nil {
		step(1)
	}
	// Atomic rename so a crash never leaves a half-written mp4 behind.
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// freeName returns base+ext inside dir, bumping to "base (N)+ext" until a free
// name is found so a conversion never overwrites an existing file.
func freeName(dir, base, ext string) string {
	cand := base + ext
	for n := 1; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, cand)); os.IsNotExist(err) {
			return cand
		}
		cand = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
}

// intendedOutput mirrors freeName's first choice (before collision bumps) so
// the enqueue response can show where each copy is heading. When the source is
// already an .mp4 the copy is always bumped to " (1)" — never the source file.
func intendedOutput(path string, isDir bool) string {
	if isDir {
		return filepath.Join(filepath.Dir(path), filepath.Base(path)+" (MP4)")
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := filepath.Join(filepath.Dir(path), stem+".mp4")
	if out == path {
		out = filepath.Join(filepath.Dir(path), stem+" (1).mp4")
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
