package fservice

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// notifierRec records the ingest/evict paths a file operation reported to the
// library pipeline (ADR-017).
type notifierRec struct {
	mu      sync.Mutex
	ingests []string
	evicts  []string
}

func (r *notifierRec) ingest(ctx context.Context, paths []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ingests = append(r.ingests, paths...)
	return nil
}

func (r *notifierRec) evict(ctx context.Context, paths []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evicts = append(r.evicts, paths...)
	return nil
}

func notifyService(rec *notifierRec) *Service {
	s := &Service{}
	s.SetLibraryNotifier(rec.ingest, rec.evict)
	return s
}

type silentReporter struct{}

func (silentReporter) Progress(float64)        {}
func (silentReporter) Subtask(string)          {}
func (silentReporter) SubtaskProgress(float64) {}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRenameNotifiesIngestThenEvict verifies a rename reports the new path to
// ingest first (file_id identity) and the old path to evict.
func TestRenameNotifiesIngestThenEvict(t *testing.T) {
	rec := &notifierRec{}
	s := notifyService(rec)
	ctx := context.Background()
	old := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, old)

	if err := s.Rename(ctx, old, "b.txt"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(old), "b.txt")
	if !reflect.DeepEqual(rec.ingests, []string{want}) {
		t.Fatalf("ingest = %v, want %v", rec.ingests, []string{want})
	}
	if !reflect.DeepEqual(rec.evicts, []string{old}) {
		t.Fatalf("evict = %v, want %v", rec.evicts, []string{old})
	}
}

// TestDeleteNotifiesEvict verifies a delete reports the removed paths to evict
// (and never to ingest).
func TestDeleteNotifiesEvict(t *testing.T) {
	rec := &notifierRec{}
	s := notifyService(rec)
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, p)

	res := s.Delete(ctx, []string{p})
	if res.Done != 1 {
		t.Fatalf("delete result = %+v", res)
	}
	if !reflect.DeepEqual(rec.evicts, []string{p}) {
		t.Fatalf("evict = %v, want %v", rec.evicts, []string{p})
	}
	if len(rec.ingests) != 0 {
		t.Fatalf("delete must not ingest, got %v", rec.ingests)
	}
}

// TestCopyJobNotifiesIngest verifies a finished copy feeds the created paths to
// ingest.
func TestCopyJobNotifiesIngest(t *testing.T) {
	rec := &notifierRec{}
	s := notifyService(rec)
	ctx := context.Background()
	base := t.TempDir()
	src := filepath.Join(base, "src", "clip.mkv")
	dest := filepath.Join(base, "dst")
	writeFile(t, src)

	if err := s.copyJob(ctx, []string{src}, dest, silentReporter{}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dest, "clip.mkv")
	if !reflect.DeepEqual(rec.ingests, []string{want}) {
		t.Fatalf("ingest = %v, want %v", rec.ingests, []string{want})
	}
	if len(rec.evicts) != 0 {
		t.Fatalf("copy must not evict, got %v", rec.evicts)
	}
}

// TestMoveJobNotifiesIngestThenEvict verifies a finished move ingests the new
// locations before evicting the old ones (order keeps file_id identity).
func TestMoveJobNotifiesIngestThenEvict(t *testing.T) {
	rec := &notifierRec{}
	s := notifyService(rec)
	ctx := context.Background()
	base := t.TempDir()
	src := filepath.Join(base, "src", "clip.mkv")
	dest := filepath.Join(base, "dst")
	writeFile(t, src)

	if err := s.moveJob(ctx, []string{src}, dest, silentReporter{}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dest, "clip.mkv")
	if !reflect.DeepEqual(rec.ingests, []string{want}) {
		t.Fatalf("ingest = %v, want %v", rec.ingests, []string{want})
	}
	if !reflect.DeepEqual(rec.evicts, []string{src}) {
		t.Fatalf("evict = %v, want %v", rec.evicts, []string{src})
	}
}
