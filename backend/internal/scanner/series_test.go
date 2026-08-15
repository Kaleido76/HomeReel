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

// 手动编辑过的单集标题（title_source='manual'）在扫描/同步后保留，不被文件名覆盖
// （批量修改显示名称依赖此保证，ADR-015/017 同语义）。
func TestScanPreservesManualMemberTitle(t *testing.T) {
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
	id := series[0].ID
	members, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	// 模拟前端批量改名：手动编辑第一个成员的标题（title_source → manual）。
	if err := svc.videos.UpdateMetadata(ctx, members[0].VideoID, domain.VideoPatch{
		Title: ptr("第一集"),
	}); err != nil {
		t.Fatalf("manual title: %v", err)
	}

	// 扫描与同步都不能把手动标题还原为文件名。
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := svc.SyncSeries(ctx, id); err != nil {
		t.Fatalf("sync: %v", err)
	}
	after, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].VideoID == members[0].VideoID && after[0].Title != "第一集" {
		t.Fatalf("manual title lost after scan/sync: %+v", after[0])
	}
}

// 「按文件名字典序重新刷新排序」清除手动排序（sort_manual=0）并按文件名序重绑；
// 手动编辑过的标题保留，但顺序恢复为文件名序。
func TestResetSeriesSortRestoresFileNameOrder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	// 文件名序 b < a，但手动排序要求 a 在前。
	mustVideo(t, svc, ctx, root, "Show/b.mkv")
	mustVideo(t, svc, ctx, root, "Show/a.mkv")
	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	id := series[0].ID
	members, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	// 手动重排为 b,a 并手动改 b 的标题。
	if err := svc.series.SetMemberOrder(ctx, id, []string{members[1].VideoID, members[0].VideoID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.videos.UpdateMetadata(ctx, members[1].VideoID, domain.VideoPatch{
		Title: ptr("手动名"),
	}); err != nil {
		t.Fatal(err)
	}

	// 恢复自动模式：顺序变回文件名序 a,b，手动标题仍保留。
	if err := svc.ResetSeriesSort(ctx, id); err != nil {
		t.Fatalf("reset sort: %v", err)
	}
	after, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].Title != "a" || after[1].Title != "手动名" {
		t.Fatalf("after reset = %+v, want a then manually-named b", after)
	}
	// 再次扫描也不再按手动顺序维护。
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	afterScan, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterScan) != 2 || afterScan[0].Title != "a" || afterScan[1].Title != "手动名" {
		t.Fatalf("after scan = %+v, want a then manually-named b (auto mode)", afterScan)
	}
}

func ptr[T any](v T) *T { return &v }
