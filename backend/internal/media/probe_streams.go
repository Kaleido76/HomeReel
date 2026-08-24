package media

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// StreamProbe is the raw ffprobe snapshot of a source: its duration, the first
// video codec, and the full audio/subtitle codec lists. It feeds both the
// conversion strategy (ConvertToMp4) and the format-factory operations-panel
// probe endpoint.
type StreamProbe struct {
	Duration       float64
	VideoCodec     string
	AudioCodecs    []string
	SubtitleCodecs []string
}

// ffprobeProbe is the JSON shape of the ffprobe call used by ProbeStreams.
type ffprobeProbe struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// ProbeStreams reads the stream list and duration of src in one ffprobe call.
func ProbeStreams(ctx context.Context, p Paths, src string) (StreamProbe, error) {
	out, err := ffprobeCmd(ctx, p,
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type,codec_name",
		"-of", "json",
		src).Output()
	if err != nil {
		return StreamProbe{}, fmt.Errorf("ffprobe: %w", err)
	}
	var pr ffprobeProbe
	if err := json.Unmarshal(out, &pr); err != nil {
		return StreamProbe{}, fmt.Errorf("parse ffprobe: %w", err)
	}
	// Codec lists start non-nil so callers can hand them to JSON as arrays — a
	// missing stream type must never serialize to null.
	sp := StreamProbe{AudioCodecs: []string{}, SubtitleCodecs: []string{}}
	if d, err := strconv.ParseFloat(pr.Format.Duration, 64); err == nil && d > 0 {
		sp.Duration = d
	}
	for _, st := range pr.Streams {
		switch st.CodecType {
		case "video":
			if sp.VideoCodec == "" {
				sp.VideoCodec = st.CodecName
			}
		case "audio":
			sp.AudioCodecs = append(sp.AudioCodecs, st.CodecName)
		case "subtitle":
			sp.SubtitleCodecs = append(sp.SubtitleCodecs, st.CodecName)
		}
	}
	return sp, nil
}
