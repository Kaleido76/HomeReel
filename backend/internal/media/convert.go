package media

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ConvertOpts drives a format-factory conversion: which video codec to use
// (stream copy, h264, h265), the re-encode quality (CRF), how audio is handled,
// and whether the first subtitle is burned into the picture. Callers normalize
// the values (ConvertParams.norm in the fservice layer) before ConvertToMp4.
type ConvertOpts struct {
	Src   string
	Dst   string
	Video string // copy | h264 | h265
	VCRF  int    // re-encode quality (0 = preset default)
	Audio string // smart | copy | aac
	AKbps int    // AAC bitrate
	Burn  bool   // burn the first subtitle into the picture
}

// ConvertToMp4 turns src into a browser-friendly faststart MP4 copy at dst,
// reporting the per-file completion fraction [0,1] through step (may be nil).
// The actual ffmpeg invocation is chosen from the stream probe by an ordered
// attempt list; each attempt is tried in order until one succeeds, and only
// the last error is returned.
func ConvertToMp4(ctx context.Context, p Paths, o ConvertOpts, step func(float64)) error {
	st, err := ProbeStreams(ctx, p, o.Src)
	if err != nil {
		// ffprobe missing/failed: fall back to a blind lossless copy; the
		// fallback attempts below still rescue files it cannot handle.
		st = StreamProbe{}
	}
	facts := convertFacts{
		duration:      st.Duration,
		hasAudio:      len(st.AudioCodecs) > 0,
		audioCopyable: len(st.AudioCodecs) > 0 && UniversalAudioCodecs[st.AudioCodecs[0]],
		hasSubtitle:   len(st.SubtitleCodecs) > 0,
	}
	// ffprobe's duration is the source of truth for progress: out_time/duration
	// is exact. Only when it is missing does an estimate or the size signal step
	// in.
	realDur := facts.duration > 0
	total := facts.duration
	if !realDur {
		total = estimatedDuration(o.Src)
	}
	var inputSize int64
	if info, err := os.Stat(o.Src); err == nil {
		inputSize = info.Size()
	}
	var lastErr error
	for _, attempt := range convertAttempts(facts, o.Src, o) {
		expected := int64(0)
		if attempt.streamCopy {
			expected = inputSize
		}
		if err := runConvertArgs(ctx, p, o.Src, o.Dst, attempt.args, total, expected, realDur, step); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("ffmpeg 转换失败：%w", lastErr)
}

// convertFacts is the reduced stream snapshot that decides how a source is
// converted: its duration (for progress/ETA), whether it carries audio (and
// whether that audio can be stream-copied without breaking playback), and
// whether it carries a subtitle stream to burn in.
type convertFacts struct {
	duration      float64
	hasAudio      bool
	audioCopyable bool
	hasSubtitle   bool
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

// convertAttempt is one concrete ffmpeg invocation: the raw codec/filter args
// and whether it is a stream copy (whose output size tracks the source, which
// makes total_size a usable progress signal).
type convertAttempt struct {
	args       []string
	streamCopy bool
}

// convertAttempts lays out the ordered ffmpeg attempts for the given params,
// most desirable first; ConvertToMp4 tries each until one succeeds.
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
func convertAttempts(f convertFacts, src string, params ConvertOpts) []convertAttempt {
	if params.Video == "h264" || params.Video == "h265" {
		if params.Burn {
			return []convertAttempt{{args: reencodeArgs(f, src, params, true, false)}}
		}
		return []convertAttempt{
			{args: reencodeArgs(f, src, params, false, false)},
			{args: reencodeArgs(f, src, params, false, true)},
		}
	}
	attempts := []convertAttempt{{args: losslessArgs(f, params), streamCopy: true}}
	attempts = append(attempts, convertAttempt{args: burnArgs(f, src, params)})
	return attempts
}

// losslessArgs maps every stream with -c copy, keeping text subtitles as
// mov_text; audio follows the params (see audioSelector).
func losslessArgs(f convertFacts, params ConvertOpts) []string {
	args := []string{"-map", "0", "-c", "copy", "-c:s", "mov_text"}
	return append(args, audioSelector(f, params)...)
}

// reencodeArgs re-encodes the primary video to h264/h265 at the requested CRF
// with yuv420p output. Audio follows the params. With burn the first subtitle
// track is drawn into the picture and no subtitle stream is mapped; otherwise
// text subtitles are wrapped to mov_text (dropSubs retries without any subtitle
// stream for sources with bitmap subtitles mp4 cannot hold).
func reencodeArgs(f convertFacts, src string, params ConvertOpts, burn, dropSubs bool) []string {
	codec := "libx264"
	if params.Video == "h265" {
		codec = "libx265"
	}
	args := []string{"-map", "0:v:0"}
	if burn && f.hasSubtitle {
		args = append(args, "-vf", "subtitles='"+escapeFilterPath(src)+"':si=0")
	}
	args = append(args, "-c:v", codec, "-crf", strconv.Itoa(params.VCRF), "-pix_fmt", "yuv420p")
	args = append(args, "-map", "0:a?")
	args = append(args, audioSelector(f, params)...)
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
func burnArgs(f convertFacts, src string, params ConvertOpts) []string {
	args := []string{"-map", "0:v:0"}
	if f.hasSubtitle {
		args = append(args, "-vf", "subtitles='"+escapeFilterPath(src)+"':si=0")
	}
	args = append(args, "-c:v", "libx264", "-crf", "19", "-pix_fmt", "yuv420p")
	args = append(args, "-map", "0:a?")
	return append(args, audioSelector(f, params)...)
}

// audioSelector returns the -c:a args for the params' audio choice. "smart"
// copies only universally-playable codecs (aac/mp3) and rebuilds everything
// else to AAC — AC3/EAC3 lack a Dolby decoder in most browsers (Chromium/
// Firefox) and Windows apps report them as unsupported, so copying them would
// ship a file that plays silent on half the machines.
func audioSelector(f convertFacts, params ConvertOpts) []string {
	kbps := strconv.Itoa(params.AKbps)
	switch params.Audio {
	case "aac":
		return []string{"-c:a", "aac", "-b:a", kbps + "k"}
	case "copy":
		if f.hasAudio {
			return []string{"-c:a", "copy"}
		}
		return nil
	default: // smart
		if f.hasAudio && !f.audioCopyable {
			return []string{"-c:a", "aac", "-b:a", kbps + "k"}
		}
		if f.hasAudio {
			return []string{"-c:a", "copy"}
		}
		return nil
	}
}

// escapeFilterPath renders a Windows path safe for the ffmpeg subtitles filter:
// forward slashes plus an escaped drive colon (C\:/Users/...), wrapped in single
// quotes by the caller.
func escapeFilterPath(p string) string {
	return strings.ReplaceAll(filepath.ToSlash(p), ":", "\\:")
}

// runConvertArgs runs one ffmpeg invocation that writes a faststart MP4 to dst
// (via a temp file + atomic rename). extra carries the stream mapping / codec /
// filter options; the muxer is pinned with -f mp4 because the temp output has
// no useful extension. Progress is read from ffmpeg's -progress stream,
// preferring the exact signal over a heuristic, in this order:
//  1. out_time / duration — exact, when ffprobe reported a real duration;
//  2. total_size / expectedSize — accurate for a stream copy (~1:1 with the
//     source), used when the duration is unknown;
//  3. out_time / estimated duration — rough last resort.
func runConvertArgs(ctx context.Context, p Paths, src, dst string, extra []string, total float64, expectedSize int64, realDur bool, step func(float64)) error {
	tmp := dst + ".tmp"
	args := []string{
		"-i", src,
	}
	args = append(args, extra...)
	args = append(args, "-progress", "pipe:1", "-f", "mp4", "-movflags", "+faststart", tmp)
	cmd := ffmpegCmd(ctx, p, args...)
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
