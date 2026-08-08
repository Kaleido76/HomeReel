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

// IsVideo reports whether a file name has a video extension.
func IsVideo(name string) bool {
	return videoExts[strings.ToLower(filepath.Ext(name))]
}
