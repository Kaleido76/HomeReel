package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// Info is the subset of ffprobe output the library needs.
type Info struct {
	Duration  float64
	Container string
	Codec     string
	Width     int
	Height    int
}

// Probe runs ffprobe on path and returns media metadata.
func Probe(ctx context.Context, ffprobePath, path string) (Info, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path)
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration   string `json:"duration"`
			FormatName string `json:"format_name"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Info{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	var info Info
	info.Container = raw.Format.FormatName
	info.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, s := range raw.Streams {
		if s.CodecType == "video" {
			info.Codec = s.CodecName
			info.Width = s.Width
			info.Height = s.Height
			break
		}
	}
	return info, nil
}

// Thumbnail extracts a cover frame (320px wide) and a small thumb (160px)
// into the given output paths. position is the seek point in seconds.
func Thumbnail(ctx context.Context, ffmpegPath, src, coverPath, thumbPath string, duration float64) error {
	pos := 1.0
	if duration > 0 && duration < 2 {
		pos = duration / 2
	}
	if err := extractFrame(ctx, ffmpegPath, src, coverPath, pos, "scale=320:-2"); err != nil {
		return err
	}
	return extractFrame(ctx, ffmpegPath, src, thumbPath, pos, "scale=160:-2")
}

func extractFrame(ctx context.Context, ffmpegPath, src, outPath string, pos float64, vf string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-ss", strconv.FormatFloat(pos, 'f', 3, 64),
		"-i", src,
		"-frames:v", "1",
		"-vf", vf,
		"-q:v", "2",
		outPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg thumbnail %s: %w", src, err)
	}
	return nil
}
