package scanner

import (
	"context"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
)

func mustVideo(t *testing.T, svc *Service, ctx context.Context, root, rel string) string {
	t.Helper()
	mkVideo(t, filepath.Join(root, filepath.FromSlash(rel)))
	return rel
}

// Two similar episode files in one directory become a series; a lone file in
// its own directory stays standalone.
func TestGroupSameDirSeries(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Foo.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Lonely/Lonely.S01E01.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Title != "Foo" || series[0].SeasonNumber != 1 || series[0].MemberCount != 2 {
		t.Fatalf("series = %+v, want one Foo season with 2 members", series)
	}

	all, _ := svc.videos.ListBySource(ctx, "s1")
	for _, v := range all {
		if v.RelativePath == "Lonely/Lonely.S01E01.mkv" && v.Kind != "movie" {
			t.Errorf("lone episode should stay standalone, got kind=%s", v.Kind)
		}
	}
}

// A series strictly corresponds to its physical folder: a single non-matching
// video object (a special, an unrelated file) keeps the whole directory
// standalone, even when a clear pattern is otherwise present.
func TestGroupRejectsMixedFolder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Foo.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Foo 特别篇.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("mixed folder must not auto-group, got %d series", len(series))
	}
	all, _ := svc.videos.ListBySource(ctx, "s1")
	for _, v := range all {
		if v.Kind != "movie" {
			t.Errorf("%s should stay standalone, got kind=%s", v.RelativePath, v.Kind)
		}
	}
}

// A directory mixing several titles is not grouped — no pattern covers every
// video, and the folder cannot correspond to one series.
func TestGroupMixedTitlesStaysStandalone(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Foo.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Bar.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Bar.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Baz.S01E01.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("mixed titles must not auto-group, got %d series", len(series))
	}
	all, _ := svc.videos.ListBySource(ctx, "s1")
	for _, v := range all {
		if v.Kind != "movie" {
			t.Errorf("%s should stay standalone, got kind=%s", v.RelativePath, v.Kind)
		}
	}
}

// A Season folder is an explicit series relationship, but a season with a
// single episode stays standalone (series need multiple videos).
func TestGroupSeasonFolder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Show/Season 1/e01.mkv")
	mustVideo(t, svc, ctx, root, "Show/Season 1/e02.mkv")
	mustVideo(t, svc, ctx, root, "Show/Season 2/e01.mkv")
	mustVideo(t, svc, ctx, root, "Show/Season 2/e02.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2 (two seasons)", len(series))
	}
	if series[0].Title != "Show" || series[1].Title != "Show" {
		t.Errorf("show titles wrong: %+v", series)
	}
	if series[0].MemberCount != 2 || series[1].MemberCount != 2 {
		t.Errorf("member counts wrong: %+v", series)
	}

	// Season 1 and Season 2 are weakly linked automatically.
	links, err := svc.series.GetLinks(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].LinkedID != series[1].ID {
		t.Errorf("auto links = %+v, want link to %s", links, series[1].ID)
	}
}

func TestLoneEpisodeInSeasonFolderStaysStandalone(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Show/Season 1/e01.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("single-episode season must stay standalone, got %d series", len(series))
	}
}

// Numbered parts (Part N / numeric suffix) group as ordinary series — one show
// with one season per part, linked — and are not a distinct movie/tv type. The
// folder must be pure: a non-matching video in the same directory keeps it
// ungrouped.
func TestGroupNumberedParts(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Bar 1.mkv")
	mustVideo(t, svc, ctx, root, "Bar 2.mkv")
	mustVideo(t, svc, ctx, root, "Solo/Solo.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2 (one season per part)", len(series))
	}
	for _, s := range series {
		if s.Title != "Bar" || s.MemberCount != 1 {
			t.Errorf("series wrong: %+v", s)
		}
	}
	if series[0].SeasonNumber != 1 || series[1].SeasonNumber != 2 {
		t.Errorf("part ordering wrong: %+v", series)
	}
	links, err := svc.series.GetLinks(ctx, series[0].ID)
	if err != nil || len(links) != 1 || links[0].LinkedID != series[1].ID {
		t.Errorf("part links = %+v err=%v", links, err)
	}

	// Solo (in its own folder) stays standalone.
	all, _ := svc.videos.ListBySource(ctx, "s1")
	for _, v := range all {
		if v.RelativePath == "Solo/Solo.mkv" && v.Kind != "movie" {
			t.Errorf("Solo should stay standalone, got kind=%s", v.Kind)
		}
	}
}

// A lone video in a folder keeps the folder ungrouped even when a part of a
// matching numbered pair sits in the same directory (strict folder purity).
func TestGroupPartMixedWithOtherVideoStaysStandalone(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Bar 1.mkv")
	mustVideo(t, svc, ctx, root, "Bar 2.mkv")
	mustVideo(t, svc, ctx, root, "Solo.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("parts with a non-matching sibling must not group, got %d series", len(series))
	}
}

// Missing positions are preserved: a series with episodes 1 and 3 keeps the
// gap (no ordering by DB id); the frontend simply shows no placeholder.
func TestGroupKeepsGaps(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Gap.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Gap.S01E03.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil || len(series) != 1 {
		t.Fatalf("series = %+v err=%v", series, err)
	}
	real, err := svc.series.GetMembers(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 2 || real[0].EpisodeNumber != 1 || real[1].EpisodeNumber != 3 {
		t.Errorf("gap not preserved: %+v", real)
	}
}

// Release/quality tags (720p, x264, …) are not part of the title, so files
// differing only in those still belong to one series.
func TestGroupToleratesQualityTags(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo.S01E01.720p.mkv")
	mustVideo(t, svc, ctx, root, "Foo.S01E02.1080p.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil || len(series) != 1 {
		t.Fatalf("series = %+v err=%v", series, err)
	}
	if series[0].Title != "Foo" || series[0].MemberCount != 2 {
		t.Errorf("series = %+v", series[0])
	}
}

// Unnumbered but related files share a title prefix and distinct suffixes
// ("XX冒险记：北京" / "XX冒险记：上海" / "XX冒险记：广州"). They form one series
// ordered by file name.
func TestGroupUnnumberedSeries(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "XX冒险记：北京.mkv")
	mustVideo(t, svc, ctx, root, "XX冒险记：上海.mkv")
	mustVideo(t, svc, ctx, root, "XX冒险记：广州.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 {
		t.Fatalf("series = %d, want 1 unnumbered series", len(series))
	}
	if series[0].Title != "XX冒险记" || series[0].MemberCount != 3 {
		t.Errorf("series = %+v", series[0])
	}
	members, err := svc.series.GetMembers(ctx, series[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, m := range members {
		if m.EpisodeNumber < 1 || m.EpisodeNumber > 3 {
			t.Errorf("member %s episode = %d, want 1..3", m.Title, m.EpisodeNumber)
		}
		seen[m.EpisodeNumber] = true
	}
	for i := 1; i <= 3; i++ {
		if !seen[i] {
			t.Errorf("missing position %d in unnumbered series", i)
		}
	}
}

// Chinese 第x集 files inside a show folder carry no title of their own — the
// show name comes from the parent directory.
func TestGroupChineseEpisodesInShowFolder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "凡人修仙传/第01集.mkv")
	mustVideo(t, svc, ctx, root, "凡人修仙传/第02集.mkv")

	if _, err := svc.ScanSource(ctx, ensureSource(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Title != "凡人修仙传" || series[0].MemberCount != 2 {
		t.Fatalf("series = %+v, want one 凡人修仙传 series with 2 members", series)
	}
}
