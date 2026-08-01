package files

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMkdirAndValidation(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()

	if err := svc.Mkdir(root, "", "新建文件夹"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "新建文件夹")); err != nil {
		t.Fatalf("mkdir not created: %v", err)
	}

	if err := svc.Mkdir(root, "", "a/b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("mkdir with separator = %v, want ErrInvalidName", err)
	}
	if err := svc.Mkdir(root, "", ".."); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("mkdir .. = %v, want ErrInvalidName", err)
	}
	if err := svc.Mkdir(root, "../escape", "x"); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("mkdir outside root = %v, want ErrOutsideRoot", err)
	}
}

func TestRename(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "old.txt"))

	if err := svc.Rename(root, "old.txt", "new.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "old.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file should be gone: %v", err)
	}

	if err := svc.Rename(root, "new.txt", "../x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("rename invalid name = %v, want ErrInvalidName", err)
	}
}

func TestMoveAndDelete(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src"))
	mustMkdir(t, filepath.Join(root, "dst"))
	mustWrite(t, filepath.Join(root, "src", "a.mp4"))
	mustWrite(t, filepath.Join(root, "src", "b.txt"))

	res := svc.Move(root, []string{"src/a.mp4", "src/b.txt"}, "dst")
	if res.Done != 2 || len(res.Errors) != 0 {
		t.Fatalf("move result = %+v, want 2 done", res)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "a.mp4")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}

	res = svc.Move(root, []string{"dst/a.mp4"}, "missing")
	if len(res.Errors) != 1 {
		t.Fatalf("move to missing dir should error: %+v", res)
	}

	res = svc.Delete(root, []string{"src/b.txt", "dst"})
	if res.Done != 2 || len(res.Errors) != 0 {
		t.Fatalf("delete result = %+v, want 2 done", res)
	}
	if _, err := os.Stat(filepath.Join(root, "dst")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted dir should be gone: %v", err)
	}
}

func TestOpenFile(t *testing.T) {
	svc := newSvc()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.mp4"))

	f, err := svc.OpenFile(root, "a.mp4")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = f.Close()

	if _, err := svc.OpenFile(root, "../secret"); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("open traversal = %v, want ErrOutsideRoot", err)
	}
}

func TestUploadChunks(t *testing.T) {
	svc := NewService(t.TempDir())
	root := t.TempDir()

	data := []byte("0123456789abcde")
	const total = 3
	for i := 0; i < total; i++ {
		start := i * 5
		end := start + 5
		if end > len(data) {
			end = len(data)
		}
		if err := svc.SaveChunk("up1", i, bytes.NewReader(data[start:end])); err != nil {
			t.Fatalf("save chunk %d: %v", i, err)
		}
	}
	if err := svc.CompleteUpload("up1", "movie.mp4", root, "", total); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "movie.mp4"))
	if err != nil {
		t.Fatalf("read assembled: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("assembled = %q, want %q", got, data)
	}

	if err := svc.CompleteUpload("nope", "x.txt", root, "", 2); !errors.Is(err, ErrMissingChunk) {
		t.Fatalf("missing chunks = %v, want ErrMissingChunk", err)
	}
	if err := svc.CompleteUpload("up2", "../evil.txt", root, "", 1); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad filename = %v, want ErrInvalidName", err)
	}
}

func TestCompleteUploadIdempotentAfterLostResponse(t *testing.T) {
	svc := NewService(t.TempDir())
	root := t.TempDir()

	if err := svc.SaveChunk("up-x", 0, bytes.NewReader([]byte("abc"))); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := svc.CompleteUpload("up-x", "a.txt", root, "", 1); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Parts are cleaned up; the final response was lost and the client retries
	// the final chunk. Must be treated as already completed, not an error.
	if err := svc.CompleteUpload("up-x", "a.txt", root, "", 1); err != nil {
		t.Fatalf("re-complete after lost response = %v, want nil", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(got) != "abc" {
		t.Fatalf("file corrupted after retry: %q err=%v", got, err)
	}
}

func TestCleanupStaleUploads(t *testing.T) {
	uploads := t.TempDir()
	svc := NewService(uploads)

	stale := filepath.Join(uploads, "stale-upload")
	fresh := filepath.Join(uploads, "fresh-upload")
	if err := os.MkdirAll(filepath.Join(stale, "0.part"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fresh, "0.part"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	removed, err := svc.CleanupStaleUploads(24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale dir should be gone: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh dir should remain: %v", err)
	}
}
