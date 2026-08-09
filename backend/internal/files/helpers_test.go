package files

import "testing"

func TestIsVideo(t *testing.T) {
	cases := map[string]bool{
		"a.mp4":   true,
		"b.MKV":   true, // extension is case-insensitive
		"c.avi":   true,
		"d.ts":    true,
		"e.txt":   false,
		"noext":   false,
		"archive": false,
	}
	for name, want := range cases {
		if got := IsVideo(name); got != want {
			t.Errorf("IsVideo(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsConvertible(t *testing.T) {
	cases := map[string]bool{
		// The library video set is always convertible.
		"a.mp4": true,
		"b.MKV": true,
		"c.ts":  true,
		// Extra ffmpeg-readable containers the format factory accepts.
		"old.rmvb":   true,
		"old.RM":     true,
		"dvd.vob":    true,
		"cam.asf":    true,
		"rec.mts":    true,
		"bcast.trp":  true,
		"bcast.tp":   true,
		"movie.dat":  true,
		"x.divx":     true,
		"web.f4v":    true,
		"flash.swf":  true,
		"legacy.ogm": true,
		// Non-media is never convertible.
		"e.txt":   false,
		"noext":   false,
		"archive": false,
	}
	for name, want := range cases {
		if got := IsConvertible(name); got != want {
			t.Errorf("IsConvertible(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidName(t *testing.T) {
	cases := map[string]bool{
		"file.mkv": true,
		"子目录":      true,
		"":         false,
		".":        false,
		"..":       false,
		"a/b":      false,
		`a\b`:      false,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestUnderRoot(t *testing.T) {
	if !UnderRoot(`C:\Videos\a.mkv`, `C:\Videos`) {
		t.Error("file inside root should be under root")
	}
	if !UnderRoot(`C:\Videos`, `C:\Videos`) {
		t.Error("root itself should be under root")
	}
	if UnderRoot(`C:\Videos2\a.mkv`, `C:\Videos`) {
		t.Error("sibling prefix must not count as under root")
	}
	if UnderRoot(`D:\Videos\a.mkv`, `C:\Videos`) {
		t.Error("different drive must not be under root")
	}
}

func TestUnderAnyRoot(t *testing.T) {
	roots := []string{`C:\Movies`, `D:\Shows`}
	if !UnderAnyRoot(`D:\Shows\a.mkv`, roots) {
		t.Error("second root should match")
	}
	if UnderAnyRoot(`C:\Music\a.flac`, roots) {
		t.Error("unrelated path must not match")
	}
}
