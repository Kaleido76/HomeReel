package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/media"
	"homereel/backend/internal/store"
)

// scanCalls counts inline probe/thumbnail invocations so tests can assert the
// serial per-file work without relying on the async job queue.
type scanCalls struct {
	probes int
	thumbs int
}

func newTestScanner(t *testing.T) (*Service, *jobs.Service, *scanCalls) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	jobsSvc := jobs.NewService(store.NewJobRepo(database), jobs.NewLiveStatus())
	svc := New(
		store.NewVideoRepo(database),
		store.NewStorageRepo(database),
		store.NewShowRepo(database),
		store.NewSeriesRepo(database),
		jobsSvc,
		files.NewService(t.TempDir()),
		events.New(),
		"ffprobe",
		"ffmpeg",
		t.TempDir(),
	)
	calls := &scanCalls{}
	svc.probe = func(context.Context, string, string) (media.Info, error) {
		calls.probes++
		return media.Info{Duration: 90, Codec: "h264", Container: "mp4", Width: 1920, Height: 1080}, nil
	}
	svc.thumbnail = func(context.Context, string, string, string, string, float64) error {
		calls.thumbs++
		return nil
	}
	return svc, jobsSvc, calls
}

func mkVideo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func storageFor(root string) domain.Storage {
	return domain.Storage{
		ID: "s1", Name: "root", Type: domain.StorageTypeInternal,
		RootPath: root, Available: true, CreatedAt: "t",
	}
}

func ensureStorage(t *testing.T, svc *Service, root string) domain.Storage {
	t.Helper()
	st := storageFor(root)
	if _, err := svc.storages.Get(context.Background(), st.ID); err != nil {
		if err := svc.storages.Create(context.Background(), st); err != nil {
			t.Fatalf("create storage: %v", err)
		}
	}
	return st
}

func TestScanAddsAndReScans(t *testing.T) {
	svc, _, calls := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "a.mp4"))
	mkVideo(t, filepath.Join(root, "sub", "b.mkv"))

	res, err := svc.Scan(ctx, ensureStorage(t, svc, root))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Added != 2 || res.Unchanged != 0 {
		t.Fatalf("first scan result = %+v", res)
	}
	if calls.probes != 2 || calls.thumbs != 2 {
		t.Fatalf("inline work = probes %d thumbs %d, want 2/2", calls.probes, calls.thumbs)
	}

	// Second scan: unchanged files with metadata present — no re-probe.
	res, err = svc.Scan(ctx, ensureStorage(t, svc, root))
	if err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	if res.Added != 0 || res.Unchanged != 2 || res.Missing != 0 {
		t.Fatalf("re-scan result = %+v", res)
	}
	if calls.probes != 2 || calls.thumbs != 2 {
		t.Fatalf("re-scan re-processed files: probes %d thumbs %d", calls.probes, calls.thumbs)
	}
}

func TestScanDetectsMoveByFileID(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "movie.mp4"))

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	first, _ := svc.videos.ListByStorage(ctx, "s1")
	if len(first) != 1 {
		t.Fatalf("videos = %d, want 1", len(first))
	}

	// Rename (move) the file; file_id stays the same.
	if err := os.Rename(filepath.Join(root, "movie.mp4"), filepath.Join(root, "renamed.mp4")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	res, err := svc.Scan(ctx, ensureStorage(t, svc, root))
	if err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	if res.Added != 0 || res.Updated != 1 || res.Missing != 0 {
		t.Fatalf("move result = %+v", res)
	}
	after, _ := svc.videos.ListByStorage(ctx, "s1")
	if len(after) != 1 || after[0].RelativePath != "renamed.mp4" {
		t.Fatalf("video not re-linked: %+v", after)
	}
}

func TestScanMarksDeleted(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "gone.mp4"))

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "gone.mp4")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	res, err := svc.Scan(ctx, ensureStorage(t, svc, root))
	if err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	if res.Missing != 1 {
		t.Fatalf("missing = %d, want 1", res.Missing)
	}
	after, _ := svc.videos.ListByStorage(ctx, "s1")
	if len(after) != 0 {
		t.Fatalf("videos after delete = %d, want 0", len(after))
	}
}

func TestReProbeWhenMetadataMissing(t *testing.T) {
	svc, _, calls := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "a.mp4"))

	failProbe := true
	svc.probe = func(context.Context, string, string) (media.Info, error) {
		calls.probes++
		if failProbe {
			return media.Info{}, errors.New("probe failed")
		}
		return media.Info{Duration: 90, Codec: "h264", Container: "mp4"}, nil
	}

	// First scan: probe fails, so the video keeps no metadata.
	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if calls.probes != 1 {
		t.Fatalf("probes after first scan = %d, want 1", calls.probes)
	}

	// Unchanged file but metadata still missing → must re-probe (self-heal).
	failProbe = false
	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	if calls.probes != 2 {
		t.Fatalf("probes after re-scan = %d, want 2 (re-probe)", calls.probes)
	}
}

func TestScanReportsSubTasks(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "a.mp4"))

	var texts []string
	var pcts []float64
	_, err := svc.scan(ctx, ensureStorage(t, svc, root), nil, func(text string, pct float64) {
		if text != "" {
			texts = append(texts, text)
			pcts = append(pcts, pct)
		}
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"探测 a.mp4", "生成 a.mp4 的缩略图"}
	if len(texts) != len(want) {
		t.Fatalf("sub-tasks = %v, want %v", texts, want)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Fatalf("sub-task[%d] = %q, want %q", i, texts[i], want[i])
		}
		if pcts[i] < 0 {
			t.Fatalf("sub-task[%d] pct = %v, want >= 0", i, pcts[i])
		}
	}
}

func TestScanSkipsWhenRootUnreachable(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "a.mp4"))
	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if _, err := svc.videos.ListByStorage(ctx, "s1"); err != nil {
		t.Fatal(err)
	}

	// Point the storage at a missing root; the scan must abort without
	// deleting anything (ADR-014).
	_, err := svc.Scan(ctx, storageFor(filepath.Join(root, "does-not-exist")))
	if err == nil {
		t.Fatalf("scan of missing root should error")
	}
	after, _ := svc.videos.ListByStorage(ctx, "s1")
	if len(after) != 1 {
		t.Fatalf("videos lost after unreachable scan = %d, want 1", len(after))
	}
}
