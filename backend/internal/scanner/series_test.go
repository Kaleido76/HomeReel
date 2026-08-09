package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
)

func mustVideo(t *testing.T, svc *Service, ctx context.Context, root, rel string) string {
	t.Helper()
	mkVideo(t, filepath.Join(root, filepath.FromSlash(rel)))
	return rel
}

// A scan never creates series — every imported video stays standalone (游离单集)
// until the user explicitly marks a folder as a series.
func TestScanDoesNotCreateSeries(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo/Foo.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Foo/Foo.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Lonely/Lonely.S01E01.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("scan must not auto-create series, got %d", len(series))
	}
	all, _ := svc.videos.ListBySource(ctx, "s1")
	for _, v := range all {
		if v.Kind != "movie" || v.ShowID != "" {
			t.Errorf("%s should stay standalone, got kind=%s show=%s", v.RelativePath, v.Kind, v.ShowID)
		}
	}
}

// A scan maintains the membership of an existing (manually created) series:
// new files that appear directly under the series root join it in file-name
// order, and members moved out of the folder detach to standalone.
func TestScanMaintainsSeriesMembership(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mustVideo(t, svc, ctx, root, "Show/e1.mkv")
	mustVideo(t, svc, ctx, root, "Show/e2.mkv")

	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].Title != "Show" || series[0].MemberCount != 2 {
		t.Fatalf("series = %+v, want one Show with 2 members", series)
	}

	// 新文件直接加入系列（文件名序），e2 移出后脱离系列。
	mustVideo(t, svc, ctx, root, "Show/e3.mkv")
	if err := os.Rename(filepath.Join(showDir, "e2.mkv"), filepath.Join(root, "e2.mkv")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].MemberCount != 2 {
		t.Fatalf("series after scan = %+v, want 2 members (e1,e3)", got)
	}
	members, err := svc.series.GetMembers(ctx, got[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, m := range members {
		seen[m.Title] = m.EpisodeNumber
	}
	if seen["e1"] != 1 || seen["e3"] != 2 {
		t.Fatalf("members = %+v, want e1=1 e3=2", members)
	}
	all, _ := svc.videos.ListAll(ctx)
	for _, v := range all {
		if v.RelativePath == "e2.mkv" && (v.Kind == "episode" || v.ShowID != "") {
			t.Fatalf("moved-out e2 should be standalone, got %+v", v)
		}
	}
}

// Members are ordered by file name 1..N regardless of numbering gaps — the
// folder is the series, ordering is filename-based.
func TestMarkSeriesOrdersMembersByFileName(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Gap")
	mustVideo(t, svc, ctx, root, "Gap/Gap.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Gap/Gap.S01E03.mkv")

	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatal(err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].Title != "Gap" {
		t.Fatalf("series = %+v, want one Gap series", series)
	}
	real, err := svc.series.GetMembers(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 2 || real[0].EpisodeNumber != 1 || real[1].EpisodeNumber != 2 {
		t.Errorf("members not ordered by file name: %+v", real)
	}
}
