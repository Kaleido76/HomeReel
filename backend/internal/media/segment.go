package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// SegmentOpts drives one on-demand HLS transcode segment (the Transcode tier,
// ADR-006): re-encode the keyframe-aligned GOP [Start, End) of Src into an
// mpegts segment at Out on the source PTS timeline.
type SegmentOpts struct {
	Src      string
	Out      string
	Start    float64 // keyframe timestamp where the segment begins
	End      float64 // next keyframe (or the video end)
	Audio    int     // audio stream ordinal for -map 0:a:N (0 = default)
	Channels int     // channel count of the selected track (>2 remaps to 5.1)
}

// TranscodeSegment re-encodes one segment's GOP with libx264 + AAC. The input
// -ss target is the keyframe itself: for re-encodes ffmpeg seeks accurately
// (decodes the source from the previous keyframe and discards frames to the
// target), so the output always starts exactly at Start regardless of container
// seek quirks. The mpegts copyts + output_ts_offset pair keeps the output PTS
// on the source timeline so adjacent segments concatenate without gaps.
func TranscodeSegment(ctx context.Context, p Paths, o SegmentOpts) error {
	if err := os.MkdirAll(filepath.Dir(o.Out), 0o755); err != nil {
		return err
	}
	// Audio: a >2-channel source is remapped to the standard 5.1 layout (with a
	// bit more headroom) so the native AAC encoder writes an ADTS chanCfg=6
	// stream instead of chanCfg=0+PCE. hls.js derives a 0-channel esds from the
	// latter, which Chromium's MSE rejects with a bufferAppendError that stalls
	// the player and loops the same fragment forever.
	audioArgs := []string{"-c:a", "aac", "-b:a", "128k"}
	if o.Channels > 2 {
		audioArgs = []string{"-c:a", "aac", "-b:a", "192k", "-channel_layout", "5.1"}
	}
	args := []string{
		"-ss", strconv.FormatFloat(o.Start, 'f', 3, 64),
		"-i", o.Src,
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", o.Audio),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
	}
	args = append(args, audioArgs...)
	args = append(args,
		"-mpegts_copyts", "1",
		"-output_ts_offset", strconv.FormatFloat(o.Start, 'f', 3, 64),
		"-t", strconv.FormatFloat(o.End-o.Start, 'f', 3, 64),
		"-f", "mpegts",
		o.Out+".tmp",
	)
	cmd := ffmpegCmd(ctx, p, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg hls segment: %w: %s", err, truncate(string(output), 300))
	}
	return os.Rename(o.Out+".tmp", o.Out)
}
