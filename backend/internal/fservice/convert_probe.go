package fservice

import (
	"context"
	"os"
	"path/filepath"

	"homereel/backend/internal/files"
)

// ConvertProbe describes one probed video for the operations panel: its codecs
// and duration, plus whether it carries a bitmap subtitle that forces a burn-in
// re-encode. A directory selection expands to its direct-level video files.
type ConvertProbe struct {
	Path              string   `json:"path"`
	Kind              string   `json:"kind"` // file | dir
	VideoCodec        string   `json:"video_codec"`
	AudioCodecs       []string `json:"audio_codecs"`
	SubtitleCodecs    []string `json:"subtitle_codecs"`
	Duration          float64  `json:"duration"`
	HasBitmapSubtitle bool     `json:"has_bitmap_subtitle"`
}

// bitmapSubtitleCodecs are the subtitle encodings stored as pictures (PGS/
// VobSub/…) that mp4 cannot carry — exactly the files that force the burn-in
// re-encode fallback.
var bitmapSubtitleCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true, "dvd_subtitle": true, "dvdsub": true,
	"dvb_subtitle": true, "dvb_teletext": true, "xsub": true,
}

// ProbeConvert probes every video selected in the format factory and returns
// per-file stream facts so the operations panel can guide the user and disable
// irrelevant options. Unreadable selections are skipped, not errored — probing
// is best-effort guidance.
func (s *Service) ProbeConvert(ctx context.Context, paths []string) []ConvertProbe {
	var out []ConvertProbe
	for _, p := range paths {
		clean := filepath.Clean(p)
		info, err := os.Stat(clean)
		if err != nil {
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(clean)
			if err != nil {
				continue
			}
			for _, de := range entries {
				if de.IsDir() {
					continue
				}
				full := filepath.Join(clean, de.Name())
				if files.IsConvertible(full) {
					out = append(out, s.probeOne(ctx, full, "dir"))
				}
			}
		} else if files.IsConvertible(clean) {
			out = append(out, s.probeOne(ctx, clean, "file"))
		}
	}
	return out
}

// probeOne runs ffprobe on one file; on any failure an entry with empty codec
// lists is returned so the caller still sees the file existed.
func (s *Service) probeOne(ctx context.Context, path, kind string) ConvertProbe {
	p := ConvertProbe{
		Path:           path,
		Kind:           kind,
		AudioCodecs:    []string{},
		SubtitleCodecs: []string{},
	}
	st, err := s.probeStreams(ctx, path)
	if err != nil {
		return p
	}
	p.VideoCodec = st.videoCodec
	p.AudioCodecs = st.audioCodecs
	p.SubtitleCodecs = st.subtitleCodecs
	p.Duration = st.duration
	for _, c := range st.subtitleCodecs {
		if bitmapSubtitleCodecs[c] {
			p.HasBitmapSubtitle = true
			break
		}
	}
	return p
}
