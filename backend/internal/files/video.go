package files

import (
	"path/filepath"
	"strings"
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".ts": true, ".m2ts": true, ".3gp": true, ".ogv": true,
}

// IsVideo reports whether a file name has a video extension. This is the
// library's gate (media sources scan these in) and is deliberately narrower
// than IsConvertible.
func IsVideo(name string) bool {
	return videoExts[strings.ToLower(filepath.Ext(name))]
}

// convertibleExts are additional ffmpeg-readable containers the format factory
// accepts beyond the library's video set — formats like RealMedia (rmvb), DVD
// VOB, ASF, MPEG-TS variants and legacy Flash that ffmpeg can demux into a
// faststart MP4 copy even though the library does not import them.
var convertibleExts = map[string]bool{
	".rmvb": true, ".rm": true, ".vob": true, ".asf": true,
	".mts": true, ".trp": true, ".tp": true, ".dat": true,
	".divx": true, ".f4v": true, ".swf": true, ".ogm": true,
}

// IsConvertible reports whether a file name has an extension the format factory
// can hand to ffmpeg: the library's video set plus the extra ffmpeg-readable
// containers above.
func IsConvertible(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return videoExts[ext] || convertibleExts[ext]
}
