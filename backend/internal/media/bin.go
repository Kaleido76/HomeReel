package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// Paths carries the resolved ffmpeg/ffprobe binaries that every media function
// uses. An empty binary means "not configured": the callers disable the
// corresponding features (remux/transcode/subtitle extraction, conversions).
type Paths struct {
	FFmpeg  string
	FFprobe string
}

// ResolvePaths validates the configured binary paths: an absolute path is used
// as-is, a bare name (the default "ffmpeg"/"ffprobe") is resolved through PATH,
// and a missing binary fails fast at startup so a broken install never surfaces
// mid-request. An explicitly empty value is kept as-is (feature disabled).
func ResolvePaths(ffmpegCfg, ffprobeCfg string) (Paths, error) {
	ffmpeg, err := resolveBin("ffmpeg", ffmpegCfg)
	if err != nil {
		return Paths{}, err
	}
	ffprobe, err := resolveBin("ffprobe", ffprobeCfg)
	if err != nil {
		return Paths{}, err
	}
	return Paths{FFmpeg: ffmpeg, FFprobe: ffprobe}, nil
}

func resolveBin(name, cfg string) (string, error) {
	if cfg == "" || filepath.IsAbs(cfg) {
		return cfg, nil
	}
	full, err := exec.LookPath(cfg)
	if err != nil {
		return "", fmt.Errorf("resolve %s (%q): %w", name, cfg, err)
	}
	return full, nil
}

// ffmpegQuietBase are the flags every ffmpeg invocation shares: no stdin
// interaction, no banner, errors only, and overwrite without asking.
var ffmpegQuietBase = []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}

// ffmpegCmd builds an ffmpeg command that prepends the shared quiet base flags,
// so every invocation uses the same binary path and error/overwrite behavior.
func ffmpegCmd(ctx context.Context, p Paths, args ...string) *exec.Cmd {
	full := make([]string, 0, len(ffmpegQuietBase)+len(args))
	full = append(full, ffmpegQuietBase...)
	full = append(full, args...)
	return exec.CommandContext(ctx, p.FFmpeg, full...)
}

// ffprobeCmd builds an ffprobe command against the resolved binary.
func ffprobeCmd(ctx context.Context, p Paths, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, p.FFprobe, args...)
}

// truncate shortens a long string (ffmpeg stderr) for error/log readability.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// UniversalAudioCodecs are the audio codecs every target player (browsers,
// Windows Media Player, VLC...) decodes inside an MP4, so they can be
// stream-copied without breaking playback. Anything else — including AC3/EAC3,
// which lack a Dolby decoder in Chromium/Firefox — must be re-encoded to AAC.
// This is the single white list shared by the remux tier and the format factory.
var UniversalAudioCodecs = map[string]bool{"aac": true, "mp3": true}

// RemuxVideoCodecs can be stream-copied out of a foreign container (MKV/AVI/...)
// into an MP4 the browser plays natively. Anything else (HEVC without a browser
// extension, RV40, MPEG2, ...) must be re-encoded by the HLS transcode path.
var RemuxVideoCodecs = map[string]bool{"h264": true, "avc1": true, "avc3": true}

// BitmapSubtitleCodecs are the subtitle encodings stored as pictures (PGS/
// VobSub/...) that an MP4 cannot carry — they force the format factory's
// burn-in re-encode fallback.
var BitmapSubtitleCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true, "dvd_subtitle": true, "dvdsub": true,
	"dvb_subtitle": true, "dvb_teletext": true, "xsub": true,
}
