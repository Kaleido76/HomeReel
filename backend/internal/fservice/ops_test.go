package fservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func noopStep(int64) {}

func TestCopyTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	var counted int64
	if err := copyTree(context.Background(), src, filepath.Join(dst, "copy"), func(n int64) { counted += n }); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if counted != 10 {
		t.Fatalf("copied bytes = %d, want 10", counted)
	}
	got, err := os.ReadFile(filepath.Join(dst, "copy", "sub", "b.txt"))
	if err != nil || string(got) != "world" {
		t.Fatalf("nested copy = %q, %v", got, err)
	}
}

func TestTotalBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.bin"), make([]byte, 5), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "y.bin"), make([]byte, 7), 0o644); err != nil {
		t.Fatal(err)
	}
	total, err := totalBytes([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if total != 12 {
		t.Fatalf("total bytes = %d, want 12", total)
	}
}
