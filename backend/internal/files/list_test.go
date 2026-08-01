package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newSvc() *Service { return NewService("") }

func TestIsVideo(t *testing.T) {
	cases := map[string]bool{
		"a.mp4":     true,
		"b.MKV":     true,
		"c.m2ts":    true,
		"d.txt":     false,
		"photo.jpg": false,
		"noext":     false,
	}
	for name, want := range cases {
		if got := IsVideo(name); got != want {
			t.Fatalf("IsVideo(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestListDirSortAndDetection(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustMkdir(t, filepath.Join(root, "b_dir"))
	mustWrite(t, filepath.Join(root, "b.mp4"))
	mustWrite(t, filepath.Join(root, "a.txt"))
	mustWrite(t, filepath.Join(root, "sub", "c.mkv"))

	entries, err := svc.ListDir(root, "")
	if err != nil {
		t.Fatalf("list root: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(entries))
	}
	if entries[0].Name != "b_dir" || entries[1].Name != "sub" {
		t.Fatalf("directories should sort first: %+v", entries)
	}
	if entries[2].Name != "a.txt" || entries[3].Name != "b.mp4" {
		t.Fatalf("files not case-insensitive sorted: %+v", entries)
	}
	if entries[2].IsVideo || !entries[3].IsVideo {
		t.Fatalf("is_video flags wrong: %+v", entries)
	}
	if entries[3].Path != "b.mp4" || entries[1].Path != "sub" {
		t.Fatalf("path fields wrong: %+v", entries)
	}
}

func TestListDirSubdirAndRelativePath(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Season 1"))
	mustWrite(t, filepath.Join(root, "Season 1", "ep01.mkv"))

	entries, err := svc.ListDir(root, "Season 1")
	if err != nil {
		t.Fatalf("list subdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "ep01.mkv" || entries[0].Path != "Season 1/ep01.mkv" {
		t.Fatalf("unexpected subdir listing: %+v", entries)
	}
}

func TestListDirMissingDir(t *testing.T) {
	svc := newSvc()
	_, err := svc.ListDir(t.TempDir(), "nope")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing dir = %v, want os.ErrNotExist", err)
	}
}

func TestListDirTraversalRejected(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()

	for _, rel := range []string{"..", "../..", "sub/../../..", "C:\\Windows", "\\"} {
		if _, err := svc.ListDir(root, rel); !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("traversal %q = %v, want ErrOutsideRoot", rel, err)
		}
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
