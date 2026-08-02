package streaming

import (
	"context"
	"os/exec"
)

// newTranscodeCommand builds the ffmpeg invocation that turns a source file
// into an HLS playlist (ADR-006 second layer). Segments are written into the
// video's cache directory; the playlist path is the final argument.
//
// -hls_list_size 0 keeps every segment in the playlist so the growing playlist
// stays seekable while the transcode runs.
// -hls_flags temp_file writes the playlist/segments via a temp file + atomic
// rename, so a concurrent reader never observes a half-written file (critical
// on Windows where an open handle would otherwise race the rewrite).
// The audio map is optional (? suffix) so videos without an audio track do not
// abort the transcode.
func newTranscodeCommand(ctx context.Context, ffmpegPath, src, segmentPattern, playlist, preset string) *exec.Cmd {
	return exec.CommandContext(ctx, ffmpegPath,
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-y",
		"-i", src,
		"-map", "0:v:0",
		"-c:v", "libx264",
		"-preset", preset,
		"-pix_fmt", "yuv420p",
		"-map", "0:a:0?",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2",
		"-hls_time", "10",
		"-hls_list_size", "0",
		"-hls_flags", "temp_file+independent_segments",
		"-hls_segment_filename", segmentPattern,
		playlist)
}
