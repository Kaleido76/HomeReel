package media

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Info is the subset of ffprobe output the library needs.
type Info struct {
	Duration  float64
	Container string
	Codec     string
	AudioCodec string
	Width     int
	Height    int
	// Segmented marks MP4-family files whose media data is split across
	// multiple top-level mdat boxes or uses moof fragments (hls.js-downloaded
	// files, fragmented MP4). Chrome's <video src> demuxer downloads such files
	// in full before playing, so they are judged non-direct-playable and the
	// user is prompted to convert them instead.
	Segmented bool
	// FastStart marks MP4-family files whose moov box sits before the first
	// mdat (the media bytes), i.e. a "faststart" layout the browser can seek
	// in immediately. A moov-at-tail or fragmented file is shown in the detail
	// page as non-faststart with a suggestion to convert for smooth seeking.
	FastStart bool
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
	// ffprobe format_name can be a comma-separated demuxer list (e.g.
	// "mov,mp4,m4a,3gp,3g2,mj2"); keep only the primary container so it can be
	// matched against playability/Content-Type maps.
	info.Container = primaryContainer(raw.Format.FormatName)
	info.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if info.Codec == "" {
				info.Codec = s.CodecName
				info.Width = s.Width
				info.Height = s.Height
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec = s.CodecName
			}
		}
	}
	info.Segmented = mp4Family(info.Container) && isSegmented(path)
	info.FastStart = mp4Family(info.Container) && isFastStart(path)
	return info, nil
}

// primaryContainer returns the first token of a comma-separated container
// name, trimming whitespace.
func primaryContainer(name string) string {
	for _, part := range strings.Split(name, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			return part
		}
	}
	return name
}

// mp4Family reports whether a primary container token belongs to the
// MP4/MOV box family, i.e. a file parsed by walking top-level boxes
// (mdat/moof counting applies only here).
func mp4Family(container string) bool {
	switch strings.ToLower(strings.TrimSpace(container)) {
	case "mov", "mp4", "m4v", "qt", "3gp", "3g2":
		return true
	}
	return false
}

// isSegmented walks the top-level boxes of an MP4-family file and reports a
// "segmented" layout: more than one mdat box, or any moof fragment box. These
// arise from hls.js download tools (one mdat per HLS segment) and fragmented
// MP4; desktop Chrome's <video src> demuxer downloads such files in full
// before playback, so they must be converted (non-direct-playable) instead.
//
// The walk reads only each box header (8 or 16 bytes) and seeks past the box,
// so a normal file (ftyp/moov/single mdat) is resolved in a handful of reads
// and a segmented one exits as soon as the second mdat is seen.
func isSegmented(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	var hdr [16]byte
	mdat := 0
	for offset := int64(0); offset < info.Size(); {
		if _, err := f.ReadAt(hdr[:8], offset); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		switch size {
		case 1: // 64-bit largesize follows the header
			if _, err := f.ReadAt(hdr[8:16], offset+8); err != nil {
				return false
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		case 0: // box extends to end of file
			size = info.Size() - offset
		}
		if size < 8 {
			return false
		}
		switch typ {
		case "mdat":
			mdat++
			if mdat > 1 {
				return true
			}
		case "moof":
			return true
		}
		offset += size
	}
	return false
}

// isFastStart walks the top-level boxes of an MP4-family file and reports a
// faststart layout: the moov box appears before the first mdat box. A moov
// that comes after mdat (or is missing entirely, as in fragmented MP4) forces
// browsers to download or buffer before seeking.
func isFastStart(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	var hdr [16]byte
	for offset := int64(0); offset < info.Size(); {
		if _, err := f.ReadAt(hdr[:8], offset); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		switch size {
		case 1: // 64-bit largesize follows the header
			if _, err := f.ReadAt(hdr[8:16], offset+8); err != nil {
				return false
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		case 0: // box extends to end of file
			size = info.Size() - offset
		}
		if size < 8 {
			return false
		}
		switch typ {
		case "moov":
			return true
		case "mdat":
			return false
		}
		offset += size
	}
	return false
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

// SubtitleStream is one detected subtitle track of a media file.
type SubtitleStream struct {
	Index    int
	Codec    string
	Language string
	Title    string
}

// TextSubtitleCodecs are the subtitle encodings remuxable to WebVTT for the
// browser <track> element. Bitmap subtitles (PGS/VobSub/…) cannot be converted
// to text and must be burned into the picture by the format factory instead.
var TextSubtitleCodecs = map[string]bool{
	"subrip": true, "srt": true, "ass": true, "ssa": true,
	"webvtt": true, "mov_text": true, "text": true,
}

// ProbeSubtitles lists the subtitle tracks of src via ffprobe.
func ProbeSubtitles(ctx context.Context, ffprobePath, src string) ([]SubtitleStream, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-select_streams", "s",
		"-show_entries", "stream=index,codec_name:stream_tags=language,title",
		"-of", "json",
		src)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe subtitles %s: %w", src, err)
	}
	var raw struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecName string `json:"codec_name"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe subtitles: %w", err)
	}
	var subs []SubtitleStream
	for _, st := range raw.Streams {
		subs = append(subs, SubtitleStream{
			Index:    st.Index,
			Codec:    st.CodecName,
			Language: st.Tags.Language,
			Title:    st.Tags.Title,
		})
	}
	return subs, nil
}

// ExtractTextSubtitle extracts the subtitle stream with the given stream index
// into a WebVTT file at outVTT (written via a temp file + rename).
func ExtractTextSubtitle(ctx context.Context, ffmpegPath, src string, streamIndex int, outVTT string) error {
	tmp := outVTT + ".tmp"
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-c:s", "webvtt",
		"-f", "webvtt",
		tmp)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg extract subtitle: %w", err)
	}
	return os.Rename(tmp, outVTT)
}

// keyframePacket is one ffprobe packet entry with its keyframe flag. pts_time
// is decoded from either a number or the "N/A" string ffprobe emits for some
// container edge cases.
type keyframePacket struct {
	Pts   json.RawMessage `json:"pts_time"`
	Flags string          `json:"flags"`
}

// keyframePts parses a packet's pts_time (number or "N/A") into a float.
func keyframePts(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil || s == "N/A" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	var f float64
	return f, json.Unmarshal(raw, &f) == nil
}

// AudioStream is one detected audio track of a media file.
type AudioStream struct {
	Index    int
	Codec    string
	Channels int
	Language string
	Title    string
}

// ProbeAudioStreams lists the audio tracks of src via ffprobe, in stream order.
func ProbeAudioStreams(ctx context.Context, ffprobePath, src string) ([]AudioStream, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-select_streams", "a",
		"-show_entries", "stream=index,codec_name,channels:stream_tags=language,title",
		"-of", "json",
		src)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe audio %s: %w", src, err)
	}
	var raw struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecName string `json:"codec_name"`
			Channels  int    `json:"channels"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe audio: %w", err)
	}
	var tracks []AudioStream
	for _, st := range raw.Streams {
		tracks = append(tracks, AudioStream{
			Index:    st.Index,
			Codec:    st.CodecName,
			Channels: st.Channels,
			Language: st.Tags.Language,
			Title:    st.Tags.Title,
		})
	}
	return tracks, nil
}

// ProbeAudioChannels returns the channel count of the streamIndex-th audio
// stream of src (0 when there is no such stream or the probe fails). It feeds
// the HLS transcode audio decision: a >2-channel source is remapped to the
// standard 5.1 layout so the native AAC encoder emits an ADTS chanCfg != 0
// stream (it writes chanCfg=0 + PCE for non-standard layouts like 5.1(side),
// and hls.js turns that into a 0-channel esds that Chromium's MSE rejects).
func ProbeAudioChannels(ctx context.Context, ffprobePath, src string, streamIndex int) int {
	out, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-select_streams", fmt.Sprintf("a:%d", streamIndex),
		"-show_entries", "stream=channels",
		"-of", "csv=p=0",
		src).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// ScanKeyframes lists the video keyframe timestamps of src via a packet-level
// ffprobe scan (flags contain 'K'). It reads only packet headers, so it is fast
// even for large files — no frames are decoded. The result feeds the on-demand
// HLS segmenter (streaming package), which needs each segment to start exactly
// at a keyframe so a stream copy is frame-exact.
func ScanKeyframes(ctx context.Context, ffprobePath, src string) ([]float64, error) {
	out, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "json",
		src).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframes %s: %w", src, err)
	}
	var raw struct {
		Packets []keyframePacket `json:"packets"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe keyframes: %w", err)
	}
	var kfs []float64
	for _, p := range raw.Packets {
		if strings.Contains(p.Flags, "K") {
			if f, ok := keyframePts(p.Pts); ok {
				kfs = append(kfs, f)
			}
		}
	}
	return kfs, nil
}
