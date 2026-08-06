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

// A lone episode-named file stays standalone; two similar files in one
// directory become a series (one season, two ordered members).
func TestGroupSameDirSeries(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Foo.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Foo.S01E02.mkv")
	mustVideo(t, svc, ctx, root, "Lonely.S01E01.mkv")

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	all, _ := svc.videos.ListByStorage(ctx, "s1")
	if len(all) != 3 {
		t.Fatalf("videos = %d, want 3", len(all))
	}
	for _, v := range all {
		if v.RelativePath == "Foo.S01E01.mkv" || v.RelativePath == "Foo.S01E02.mkv" {
			if v.Kind != "episode" || v.ShowID == "" || v.SeasonNumber != 1 {
				t.Errorf("%s not grouped: kind=%s show=%s season=%d",
					v.RelativePath, v.Kind, v.ShowID, v.SeasonNumber)
			}
		}
		if v.RelativePath == "Lonely.S01E01.mkv" && v.Kind != "movie" {
			t.Errorf("lonely episode should stay standalone, got kind=%s", v.Kind)
		}
	}

	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil || len(series) != 1 {
		t.Fatalf("series = %+v, err=%v", series, err)
	}
	if series[0].Title != "Foo" || series[0].SeasonNumber != 1 || series[0].MemberCount != 2 {
		t.Errorf("series = %+v", series[0])
	}
}

// A Season folder is itself an explicit series relationship, even with a
// single episode.
func TestGroupSeasonFolder(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Show/Season 1/e01.mkv")
	mustVideo(t, svc, ctx, root, "Show/Season 1/e02.mkv")
	mustVideo(t, svc, ctx, root, "Show/Season 2/e01.mkv")

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2 (two seasons)", len(series))
	}
	if series[0].MemberCount != 2 || series[1].MemberCount != 1 {
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

// Movie franchise parts (Part N / numeric suffix) group into linked series.
func TestGroupMovieParts(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Bar 1.mkv")
	mustVideo(t, svc, ctx, root, "Bar 2.mkv")
	mustVideo(t, svc, ctx, root, "Solo.mkv")

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	series, err := svc.series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2 movie parts", len(series))
	}
	for _, s := range series {
		if s.Kind != "movie" || s.MemberCount != 1 {
			t.Errorf("movie part wrong: %+v", s)
		}
	}
	if series[0].SeasonNumber != 1 || series[1].SeasonNumber != 2 {
		t.Errorf("part ordering wrong: %+v", series)
	}
	links, err := svc.series.GetLinks(ctx, series[0].ID)
	if err != nil || len(links) != 1 || links[0].LinkedID != series[1].ID {
		t.Errorf("part links = %+v err=%v", links, err)
	}

	// Solo stays standalone.
	all, _ := svc.videos.ListByStorage(ctx, "s1")
	for _, v := range all {
		if v.RelativePath == "Solo.mkv" && v.Kind != "movie" {
			t.Errorf("Solo should stay standalone, got kind=%s", v.Kind)
		}
	}
}

// Missing positions are preserved: a series with episodes 1 and 3 keeps the
// gap (no ordering by DB id).
func TestGroupKeepsGaps(t *testing.T) {
	svc, _, _ := newTestScanner(t)
	ctx := context.Background()
	root := t.TempDir()
	mustVideo(t, svc, ctx, root, "Gap.S01E01.mkv")
	mustVideo(t, svc, ctx, root, "Gap.S01E03.mkv")

	if _, err := svc.Scan(ctx, ensureStorage(t, svc, root)); err != nil {
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
