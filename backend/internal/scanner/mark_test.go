package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/jobs"
)

// noopReporter is a no-op jobs.Reporter for driving HandleJob in tests.
type noopReporter struct{}

func (noopReporter) Progress(float64)        {}
func (noopReporter) Subtask(string)          {}
func (noopReporter) SubtaskProgress(float64) {}

func markJob(path, kind string) jobs.Job {
	extra, _ := json.Marshal(map[string]string{"path": path, "kind": kind})
	return jobs.Job{ID: "j", Type: jobs.TypeMarkResource, Target: path, Extra: string(extra)}
}

// Marking a folder outside any media source is rejected (discrete resources no
// longer exist — everything must belong to a media source).
func TestMarkSeriesRequiresMediaSource(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	root := t.TempDir()
	mkVideo(t, filepath.Join(root, "a.mp4"))

	err := svc.HandleJob(context.Background(), markJob(root, "series"), noopReporter{})
	if err == nil {
		t.Fatalf("marking a path outside every media source must fail")
	}
}

// Creating a series inside an existing series root (or containing one) is
// rejected: membership is strictly the direct children, so series cannot nest.
func TestMarkSeriesRejectsNesting(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mkVideo(t, filepath.Join(showDir, "e1.mkv"))
	mkVideo(t, filepath.Join(showDir, "e2.mkv"))
	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}

	inner := filepath.Join(showDir, "Inner")
	mkVideo(t, filepath.Join(inner, "x.mkv"))
	if err := svc.HandleJob(ctx, markJob(inner, "series"), noopReporter{}); err == nil {
		t.Fatalf("marking inside an existing series must fail (nested)")
	}
	if err := svc.HandleJob(ctx, markJob(root, "series"), noopReporter{}); err == nil {
		t.Fatalf("marking a folder that contains a series must fail")
	}
}

// Marking the same folder twice is an idempotent refresh — one series, no empty
// duplicates.
func TestMarkSeriesIdempotent(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mkVideo(t, filepath.Join(showDir, "1.mp4"))
	mkVideo(t, filepath.Join(showDir, "2.mp4"))

	for i := 0; i < 2; i++ {
		if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
			t.Fatalf("mark series #%d: %v", i, err)
		}
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].Title != "Show" || series[0].MemberCount != 2 {
		t.Fatalf("series after re-mark = %+v, want one Show with 2 members", series)
	}
}

// Only the direct children of the marked folder become members; videos inside
// subfolders stay standalone.
func TestMarkSeriesDirectChildrenOnly(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	mkVideo(t, filepath.Join(root, "Show", "e1.mkv"))
	mkVideo(t, filepath.Join(root, "Show", "e2.mkv"))
	mkVideo(t, filepath.Join(root, "Show", "Nested", "deep.mkv"))

	if err := svc.HandleJob(ctx, markJob(filepath.Join(root, "Show"), "series"), noopReporter{}); err != nil {
		t.Fatal(err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].MemberCount != 2 {
		t.Fatalf("series = %+v, want 2 direct children (nested skipped)", series)
	}
	all, _ := svc.videos.ListAll(ctx)
	for _, v := range all {
		if v.RelativePath == "Show/Nested/deep.mkv" && v.Kind == "episode" {
			t.Fatalf("nested video must stay standalone: %+v", v)
		}
	}
}

// Renaming a folder and every file inside it is still recognised as the same
// series: file_id is name-independent, so marking the new folder refreshes the
// existing identity in place and never leaves a duplicate.
func TestMarkSeriesRefreshesRenamedFolder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mkVideo(t, filepath.Join(showDir, "Show.S01E01.mkv"))
	mkVideo(t, filepath.Join(showDir, "Show.S01E02.mkv"))

	newDir := filepath.Join(root, "Renamed")
	if err := os.Rename(showDir, newDir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Show.S01E01.mkv", "Show.S01E02.mkv"} {
		if err := os.Rename(filepath.Join(newDir, f), filepath.Join(newDir, "ep"+f[len("Show.S01E0"):len(f)])); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.HandleJob(ctx, markJob(newDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].Title != "Renamed" || series[0].MemberCount != 2 {
		t.Fatalf("series = %+v, want one Renamed series with 2 members", series)
	}
	members, err := svc.series.GetMembers(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, m := range members {
		seen[m.Title] = m.EpisodeNumber
	}
	if seen["ep1"] != 1 || seen["ep2"] != 2 {
		t.Fatalf("members = %+v, want refreshed file names ep1/ep2", members)
	}
}

// Moving a member out of one series into another updates both sides: the
// target gains it, the origin loses it (and empty series are pruned).
func TestMarkSeriesAbsorbsMovedInFile(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	seriesA := filepath.Join(root, "SeriesA")
	seriesB := filepath.Join(root, "SeriesB")
	mkVideo(t, filepath.Join(seriesA, "a1.mp4"))
	mkVideo(t, filepath.Join(seriesA, "a2.mp4"))
	mkVideo(t, filepath.Join(seriesB, "b1.mp4"))
	mkVideo(t, filepath.Join(seriesB, "b2.mp4"))
	if err := svc.HandleJob(ctx, markJob(seriesA, "series"), noopReporter{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleJob(ctx, markJob(seriesB, "series"), noopReporter{}); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(seriesB, "b1.mp4"), filepath.Join(seriesA, "b1.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleJob(ctx, markJob(seriesA, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}

	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 2 {
		t.Fatalf("series = %+v, want 2", series)
	}
	counts := map[string]int{}
	for _, s := range series {
		counts[s.Title] = s.MemberCount
	}
	if counts["SeriesA"] != 3 || counts["SeriesB"] != 1 {
		t.Fatalf("member counts = %+v, want SeriesA=3 SeriesB=1", counts)
	}
}

// Members imported by an earlier scan carry a source-relative path (with the
// series folder prefix), while the disk check compares file names directly
// under the series root. CheckSeries must compare by file name so a correctly
// indexed series is never reported as out-of-sync.
func TestSeriesCheckIgnoresRelativePathBasis(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "古惑仔")
	mkVideo(t, filepath.Join(showDir, "古惑仔-1-人在江湖.mkv"))
	mkVideo(t, filepath.Join(showDir, "古惑仔-2-猛龙过江.mkv"))

	// 先扫描（relative_path = 相对源根，带目录前缀），再手动标记系列。
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 {
		t.Fatalf("series = %d, want 1", len(series))
	}

	check, err := svc.CheckSeries(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.OutOfSync || len(check.Missing) != 0 || len(check.New) != 0 {
		t.Fatalf("series with disk-matching members must be in sync, got %+v", check)
	}
}

// CheckSeries detects drift (a deleted member / a new unindexed file) and
// SyncSeries converges it.
func TestSeriesCheckAndSync(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mkVideo(t, filepath.Join(showDir, "e1.mkv"))
	mkVideo(t, filepath.Join(showDir, "e2.mkv"))
	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatal(err)
	}
	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	id := series[0].ID

	check, err := svc.CheckSeries(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if check.OutOfSync || len(check.Missing) != 0 || len(check.New) != 0 {
		t.Fatalf("fresh series should be in sync, got %+v", check)
	}

	// 删掉 e2，并新增 e3：检查应同时报告 Missing 与 New。
	if err := os.Remove(filepath.Join(showDir, "e2.mkv")); err != nil {
		t.Fatal(err)
	}
	mkVideo(t, filepath.Join(showDir, "e3.mkv"))
	check, err = svc.CheckSeries(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !check.OutOfSync {
		t.Fatalf("check should report out-of-sync: %+v", check)
	}
	if len(check.Missing) != 1 || check.Missing[0] != "e2.mkv" {
		t.Fatalf("missing = %+v, want [e2.mkv]", check.Missing)
	}
	if len(check.New) != 1 || check.New[0] != "e3.mkv" {
		t.Fatalf("new = %+v, want [e3.mkv]", check.New)
	}

	if err := svc.SyncSeries(ctx, id); err != nil {
		t.Fatalf("sync series: %v", err)
	}
	members, err := svc.series.GetMembers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, m := range members {
		seen[m.Title] = m.EpisodeNumber
	}
	if len(seen) != 2 || seen["e1"] != 1 || seen["e3"] != 2 {
		t.Fatalf("members after sync = %+v, want e1,e3", members)
	}
}

// CheckVideo/SyncVideo: a renamed/moved file is found by file_id (moved → 可同
// 步), and after syncing the row points at the new path; a gone file is missing.
func TestVideoCheckAndSync(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	mkVideo(t, filepath.Join(root, "movie.mp4"))
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatal(err)
	}
	all, _ := svc.videos.ListBySource(ctx, "s1")
	if len(all) != 1 {
		t.Fatalf("videos = %d, want 1", len(all))
	}
	id := all[0].ID

	st, err := svc.CheckVideo(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "ok" {
		t.Fatalf("source status = %q, want ok", st.Status)
	}

	if err := os.Rename(filepath.Join(root, "movie.mp4"), filepath.Join(root, "renamed.mp4")); err != nil {
		t.Fatal(err)
	}
	st, err = svc.CheckVideo(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "moved" || filepath.Base(st.Path) != "renamed.mp4" {
		t.Fatalf("source status = %+v, want moved renamed.mp4", st)
	}

	if err := svc.SyncVideo(ctx, id); err != nil {
		t.Fatalf("sync video: %v", err)
	}
	v, err := svc.videos.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.RelativePath != "renamed.mp4" {
		t.Fatalf("video not re-linked: %+v", v)
	}

	if err := os.Remove(v.Path); err != nil {
		t.Fatal(err)
	}
	st, err = svc.CheckVideo(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "missing" {
		t.Fatalf("source status = %q, want missing", st.Status)
	}
	if err := svc.SyncVideo(ctx, id); !errors.Is(err, ErrVideoMissing) {
		t.Fatalf("sync missing video err = %v, want ErrVideoMissing", err)
	}
}

// Manual marking then scanning stays idempotent: one series, correct order.
func TestScanAndManualMarkAreIdempotent(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	_ = ensureSource(t, svc, root)
	showDir := filepath.Join(root, "Show")
	mkVideo(t, filepath.Join(showDir, "Show.S01E01.mkv"))
	mkVideo(t, filepath.Join(showDir, "Show.S01E02.mkv"))
	mkVideo(t, filepath.Join(showDir, "Show.S01E03.mkv"))

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan 1: %v", err)
	}
	if err := svc.HandleJob(ctx, markJob(showDir, "series"), noopReporter{}); err != nil {
		t.Fatalf("mark series: %v", err)
	}
	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan 2: %v", err)
	}

	series, _ := svc.series.List(ctx, domain.SeriesQuery{})
	if len(series) != 1 || series[0].Title != "Show" || series[0].MemberCount != 3 {
		t.Fatalf("series = %+v, want one Show series with 3 members", series)
	}
	members, err := svc.series.GetMembers(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range members {
		if m.EpisodeNumber != i+1 {
			t.Fatalf("member %d episode = %d, want filename order %d", i, m.EpisodeNumber, i+1)
		}
	}
}
