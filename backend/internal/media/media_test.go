package media

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// box appends a size-prefixed MP4 top-level box to b.
func box(b []byte, typ string, payload []byte) []byte {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(8+len(payload)))
	copy(hdr[4:8], typ)
	b = append(b, hdr[:]...)
	return append(b, payload...)
}

func TestIsSegmented(t *testing.T) {
	normal := box(nil, "ftyp", []byte("isom"))
	normal = box(normal, "moov", make([]byte, 64))
	normal = box(normal, "mdat", make([]byte, 256))

	multiMdat := box(nil, "ftyp", []byte("isom"))
	multiMdat = box(multiMdat, "moov", make([]byte, 64))
	multiMdat = box(multiMdat, "mdat", make([]byte, 256))
	multiMdat = box(multiMdat, "mdat", make([]byte, 128))

	fragmented := box(nil, "ftyp", []byte("isom"))
	fragmented = box(fragmented, "moov", make([]byte, 64))
	fragmented = box(fragmented, "moof", make([]byte, 32))
	fragmented = box(fragmented, "mdat", make([]byte, 128))

	dir := t.TempDir()
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"single mdat", normal, false},
		{"multiple mdat", multiMdat, true},
		{"moof fragment", fragmented, true},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name+".mp4")
		if err := os.WriteFile(path, c.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := isSegmented(path); got != c.want {
			t.Errorf("%s: isSegmented = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsFastStart(t *testing.T) {
	faststart := box(nil, "ftyp", []byte("isom"))
	faststart = box(faststart, "moov", make([]byte, 64))
	faststart = box(faststart, "mdat", make([]byte, 256))

	tailMoov := box(nil, "ftyp", []byte("isom"))
	tailMoov = box(tailMoov, "mdat", make([]byte, 256))
	tailMoov = box(tailMoov, "moov", make([]byte, 64))

	fragmented := box(nil, "ftyp", []byte("isom"))
	fragmented = box(fragmented, "moof", make([]byte, 32))
	fragmented = box(fragmented, "mdat", make([]byte, 128))

	dir := t.TempDir()
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"moov before mdat", faststart, true},
		{"moov after mdat", tailMoov, false},
		{"fragmented no head moov", fragmented, false},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name+".mp4")
		if err := os.WriteFile(path, c.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := isFastStart(path); got != c.want {
			t.Errorf("%s: isFastStart = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMp4Family(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"mov", true}, {"mp4", true}, {"m4v", true}, {"MOV", true},
		{"matroska", false}, {"webm", false}, {"", false},
	} {
		if got := mp4Family(c.in); got != c.want {
			t.Errorf("mp4Family(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
