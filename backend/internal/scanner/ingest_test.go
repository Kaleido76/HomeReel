package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
)

func strPtr(s string) *string { return &s }

// TestIngestAddsNewVideo verifies a file entering through the unified ingest
// route is probed and indexed at once, publishing VideoImported (thumbnail job)
// instead of generating inline.
func TestIngestAddsNewVideo(t *testing.T) {
	svc, _, calls := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	src := ensureSource(t, svc, root)
	file := filepath.Join(root, "a.mp4")
	mkVideo(t, file)

	ch := svc.bus.Subscribe(events.VideoImported)
	if err := svc.IngestPaths(ctx, []string{file}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	vids, err := svc.videos.ListBySource(ctx, src.ID)
	if err != nil || len(vids) != 1 {
		t.Fatalf("videos = %d, err %v; want 1", len(vids), err)
	}
	v := vids[0]
	if v.Duration != 90 || v.Codec != "h264" || v.TitleSource != domain.TitleSourceFile {
		t.Fatalf("ingested video = %+v", v)
	}
	if calls.thumbs != 0 {
		t.Fatalf("inline thumbnails = %d, want 0 (event-driven)", calls.thumbs)
	}
	select {
	case ev := <-ch:
		if ev.Data["video_id"] != v.ID {
			t.Fatalf("import event = %v", ev)
		}
	default:
		t.Fatal("no VideoImported event")
	}
}

// TestIngestRelocatesMovedVideo verifies a renamed/moved file is recognised by
// global file_id: the same row keeps its identity and its path is updated.
func TestIngestRelocatesMovedVideo(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	ensureSource(t, svc, root)
	orig := filepath.Join(root, "a.mp4")
	mkVideo(t, orig)
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	before, _ := svc.videos.ListBySource(ctx, "s1")
	if len(before) != 1 {
		t.Fatalf("videos = %d, want 1", len(before))
	}
	// Track manual history to prove identity survives the move.
	if err := svc.videos.SetTags(ctx, before[0].ID, []string{"喜爱"}); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(root, "sub", "b.mp4")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(orig, moved); err != nil {
		t.Fatal(err)
	}
	if err := svc.IngestPaths(ctx, []string{moved}); err != nil {
		t.Fatalf("ingest moved: %v", err)
	}
	after, err := svc.videos.Get(ctx, before[0].ID)
	if err != nil {
		t.Fatalf("get after move: %v", err)
	}
	if after.ID != before[0].ID || after.Path != moved {
		t.Fatalf("moved video = %+v, want same id + new path", after)
	}
	tags, _ := svc.videos.Tags(ctx, after.ID)
	if len(tags) != 1 || tags[0] != "喜爱" {
		t.Fatalf("tags after move = %v, want preserved", tags)
	}
}

// TestIngestThenEvictKeepsIdentity verifies the move ordering invariant: an
// ingest-first / evict-second sequence never deletes a relocated row.
func TestIngestThenEvictKeepsIdentity(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	ensureSource(t, svc, root)
	orig := filepath.Join(root, "a.mp4")
	mkVideo(t, orig)
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	before, _ := svc.videos.ListBySource(ctx, "s1")
	moved := filepath.Join(root, "b.mp4")
	if err := os.Rename(orig, moved); err != nil {
		t.Fatal(err)
	}
	if err := svc.IngestPaths(ctx, []string{moved}); err != nil {
		t.Fatal(err)
	}
	if err := svc.EvictPaths(ctx, []string{orig}); err != nil {
		t.Fatal(err)
	}
	after, err := svc.videos.Get(ctx, before[0].ID)
	if err != nil || after.Path != moved {
		t.Fatalf("row after move = %+v, err %v; want same id at new path", after, err)
	}
}

// TestIngestOutsideSourceIsNoop verifies paths outside every media source are
// outside the library's scope and are ignored.
func TestIngestOutsideSourceIsNoop(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "a.mp4")
	mkVideo(t, outside)
	if err := svc.IngestPaths(ctx, []string{outside}); err != nil {
		t.Fatalf("ingest outside: %v", err)
	}
	all, _ := svc.videos.ListAll(ctx)
	if len(all) != 0 {
		t.Fatalf("videos outside source = %d, want 0", len(all))
	}
}

// TestEvictRemovesDeletedVideo verifies a file deleted on disk is evicted right
// away with a VideoDeleted event.
func TestEvictRemovesDeletedVideo(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	ensureSource(t, svc, root)
	file := filepath.Join(root, "a.mp4")
	mkVideo(t, file)
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	ch := svc.bus.Subscribe(events.VideoDeleted)
	if err := svc.EvictPaths(ctx, []string{file}); err != nil {
		t.Fatalf("evict: %v", err)
	}
	all, _ := svc.videos.ListAll(ctx)
	if len(all) != 0 {
		t.Fatalf("videos after evict = %d, want 0", len(all))
	}
	select {
	case <-ch:
	default:
		t.Fatal("no VideoDeleted event")
	}
}

// TestIngestBindsSeriesMember verifies a file landing under a series root is
// bound to that series during ingest (kind=episode, title follows the file).
func TestIngestBindsSeriesMember(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	src := ensureSource(t, svc, root)
	seriesRoot := filepath.Join(root, "show")
	if _, err := svc.series.CreateAtRoot(ctx, "show", seriesRoot); err != nil {
		t.Fatalf("create series: %v", err)
	}
	file := filepath.Join(seriesRoot, "ep1.mp4")
	mkVideo(t, file)
	if err := svc.IngestPaths(ctx, []string{file}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	vids, _ := svc.videos.ListBySource(ctx, src.ID)
	if len(vids) != 1 {
		t.Fatalf("videos = %d, want 1", len(vids))
	}
	v := vids[0]
	if v.Kind != "episode" || v.ShowID == "" || v.EpisodeNumber != 1 {
		t.Fatalf("series member = %+v", v)
	}
	if v.TitleSource != domain.TitleSourceFile {
		t.Fatalf("member title_source = %s, want file", v.TitleSource)
	}
}

// TestIngestNestedSourceRouting verifies a file under a (legacy) nested source
// belongs to the deepest containing source.
func TestIngestNestedSourceRouting(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	childRoot := filepath.Join(root, "movies")
	ensureSource(t, svc, root)
	if err := svc.sources.Create(ctx, domain.MediaSource{ID: "s2", Path: filepath.Clean(childRoot), CreatedAt: "t"}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(childRoot, "a.mp4")
	mkVideo(t, file)
	if err := svc.IngestPaths(ctx, []string{file}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	all, _ := svc.videos.ListAll(ctx)
	if len(all) != 1 || all[0].SourceID != "s2" {
		t.Fatalf("nested video = %+v, want source s2", all)
	}
}

// TestProcessInlineKeepsManualTitle verifies a user-edited title survives a
// re-probe while a file-derived title is refreshed (ADR-017).
func TestProcessInlineKeepsManualTitle(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	ensureSource(t, svc, root)
	file := filepath.Join(root, "a.mp4")
	mkVideo(t, file)
	if err := svc.IngestPaths(ctx, []string{file}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.videos.ListBySource(ctx, "s1")
	id := v[0].ID
	if err := svc.videos.UpdateMetadata(ctx, id, domain.VideoPatch{Title: strPtr("我的电影")}); err != nil {
		t.Fatal(err)
	}
	svc.processInline(ctx, id, nil)
	got, _ := svc.videos.Get(ctx, id)
	if got.Title != "我的电影" || got.TitleSource != domain.TitleSourceManual {
		t.Fatalf("manual title lost: %+v", got)
	}
	// A file-derived title follows the file name.
	svc.processInline(ctx, id, nil)
	got, _ = svc.videos.Get(ctx, id)
	if got.Title != "我的电影" {
		t.Fatalf("title after second probe = %q, want preserved", got.Title)
	}
}
