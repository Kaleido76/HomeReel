package media

import (
	"context"
	"fmt"
)

// RemuxOpts drives the container-only stream copy (the Remux tier, ADR-006):
// Src's browser-decodable streams are copied into a faststart MP4 at Out.
type RemuxOpts struct {
	Src   string
	Out   string
	Audio int // audio stream ordinal for -map 0:a:N (0 = default)
}

// RemuxVideo stream-copies the video's browser-decodable streams into the MP4
// at out (faststart layout so Range seeking works). Audio is always aac/mp3 or
// absent here (RemuxPlayable gates the endpoint), so it is stream-copied too.
func RemuxVideo(ctx context.Context, p Paths, o RemuxOpts) error {
	cmd := ffmpegCmd(ctx, p,
		"-i", o.Src,
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", o.Audio),
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		o.Out,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg remux: %w: %s", err, truncate(string(output), 300))
	}
	return nil
}
