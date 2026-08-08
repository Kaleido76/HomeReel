package store

import (
	"context"
	"errors"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/domain"
)

func newVideoTestRepo(t *testing.T) (domain.VideoRepo, domain.SourceRepo) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewVideoRepo(database), NewSourceRepo(database)
}

func seedSource(t *testing.T, sources domain.SourceRepo, id, path string) {
	t.Helper()
	if err := sources.Create(context.Background(), domain.MediaSource{
		ID: id, Path: path, CreatedAt: "2026-01-01T00:00:00.000000000Z",
	}); err != nil {
		t.Fatalf("create source: %v", err)
	}
}

func TestVideoCRUDAndFingerprint(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()

	seedSource(t, sources, "s1", `C:\x`)

	v := domain.Video{
		ID: "v1", SourceID: "s1", FileID: "100", RelativePath: "a.mp4",
		Path: `C:\x\a.mp4`, Size: 100, MTime: 1000,
		Title: "a", CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t",
	}
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(ctx, "v1")
	if err != nil || got.RelativePath != "a.mp4" || got.Size != 100 {
		t.Fatalf("get = %+v err=%v", got, err)
	}

	if err := repo.UpdateFingerprint(ctx, "v1", "s1", `C:\x\b.mp4`, "b.mp4", 120, 2000, "scan2"); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	got, _ = repo.Get(ctx, "v1")
	if got.Path != `C:\x\b.mp4` || got.RelativePath != "b.mp4" || got.LastScannedAt != "scan2" {
		t.Fatalf("fingerprint not applied: %+v", got)
	}

	if err := repo.Touch(ctx, "v1", "scan3"); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ = repo.Get(ctx, "v1")
	if got.LastScannedAt != "scan3" {
		t.Fatalf("touch not applied: %+v", got)
	}

	upd := got
	upd.Title = "b"
	upd.Duration = 90
	upd.Codec = "h264"
	upd.Width = 1920
	upd.Height = 1080
	if err := repo.UpdateProbe(ctx, upd); err != nil {
		t.Fatalf("probe: %v", err)
	}
	got, _ = repo.Get(ctx, "v1")
	if got.Duration != 90 || got.Codec != "h264" || got.Width != 1920 {
		t.Fatalf("probe not applied: %+v", got)
	}

	if err := repo.UpdateCovers(ctx, "v1", "covers/v1.jpg", "thumbs/v1.thumb.jpg"); err != nil {
		t.Fatalf("covers: %v", err)
	}
	got, _ = repo.Get(ctx, "v1")
	if got.CoverPath != "covers/v1.jpg" {
		t.Fatalf("covers not applied: %+v", got)
	}
}

func TestVideoMarkMissing(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\x`)

	base := domain.Video{
		SourceID: "s1", FileID: "1", Path: `C:\x\a.mp4`,
		Size: 1, MTime: 1, Title: "a",
	}
	v1 := base
	v1.ID = "v1"
	v1.RelativePath = "a.mp4"
	v1.CreatedAt, v1.UpdatedAt, v1.LastScannedAt = "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"
	v2 := base
	v2.ID = "v2"
	v2.RelativePath = "b.mp4"
	v2.CreatedAt, v2.UpdatedAt, v2.LastScannedAt = "2026-01-02T00:00:00Z", "2026-01-02T00:00:00Z", "2026-01-02T00:00:00Z"
	_ = repo.Create(ctx, v1)
	_ = repo.Create(ctx, v2)

	ids, err := repo.MarkMissingBySource(ctx, "s1", "2026-01-02T00:00:00Z", nil)
	if err != nil {
		t.Fatalf("mark missing: %v", err)
	}
	if len(ids) != 1 || ids[0] != "v1" {
		t.Fatalf("mark missing ids = %v, want [v1]", ids)
	}
	if _, err := repo.Get(ctx, "v1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("v1 should be gone, err=%v", err)
	}
	if _, err := repo.Get(ctx, "v2"); err != nil {
		t.Fatalf("v2 should remain: %v", err)
	}
}

func TestVideoMarkMissingSkipsExcludedRoots(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\Videos`)

	base := domain.Video{
		SourceID: "s1", FileID: "1", Path: `C:\Videos\child\a.mp4`,
		Size: 1, MTime: 1, Title: "a",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", LastScannedAt: "2026-01-01T00:00:00Z",
	}
	base.ID = "v1"
	base.RelativePath = "child/a.mp4"
	if err := repo.Create(ctx, base); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A descendant source claims the subtree, so MarkMissing must leave it.
	ids, err := repo.MarkMissingBySource(ctx, "s1", "2026-01-02T00:00:00Z", []string{`C:\Videos\child`})
	if err != nil {
		t.Fatalf("mark missing: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want none", ids)
	}
	if _, err := repo.Get(ctx, "v1"); err != nil {
		t.Fatalf("v1 should remain: %v", err)
	}
}

func TestVideoUniquePath(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\x`)
	base := func(id, rel string) domain.Video {
		return domain.Video{ID: id, SourceID: "s1", FileID: id, RelativePath: rel, Path: "p", Size: 1, MTime: 1, Title: "t", CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t"}
	}
	if err := repo.Create(ctx, base("v1", "a.mp4")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(ctx, base("v2", "a.mp4")); err == nil {
		t.Fatalf("duplicate path should fail")
	}
}

func TestVideoListPaginationAndFilter(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\x`)

	base := func(id, rel, title string, dur float64) domain.Video {
		return domain.Video{
			ID: id, SourceID: "s1", FileID: id, RelativePath: rel, Path: "p",
			Size: 1, MTime: 1, Title: title, Duration: dur,
			Codec: "h264", Container: "mp4",
			CreatedAt:     "2026-01-0" + id[1:] + "T00:00:00.000000000Z",
			UpdatedAt:     "2026-01-0" + id[1:] + "T00:00:00.000000000Z",
			LastScannedAt: "2026-01-0" + id[1:] + "T00:00:00.000000000Z",
		}
	}
	_ = repo.Create(ctx, base("v1", "a/alpha.mp4", "Alpha", 100))
	_ = repo.Create(ctx, base("v2", "b/beta.mp4", "Beta", 200))
	_ = repo.Create(ctx, base("v3", "c/gamma.mp4", "Gamma", 300))

	t.Run("default newest first", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Page: 1, PageSize: 2})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 3 || len(page.Videos) != 2 {
			t.Fatalf("page = %d/%d", page.Total, len(page.Videos))
		}
		if page.Videos[0].ID != "v3" || page.Videos[1].ID != "v2" {
			t.Fatalf("order = %s,%s", page.Videos[0].ID, page.Videos[1].ID)
		}
		page2, _ := repo.List(ctx, domain.VideoQuery{Page: 2, PageSize: 2})
		if len(page2.Videos) != 1 || page2.Videos[0].ID != "v1" {
			t.Fatalf("second page = %+v", page2.Videos)
		}
	})

	t.Run("title search", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Q: "be", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 1 || page.Videos[0].ID != "v2" {
			t.Fatalf("search = %d %+v", page.Total, page.Videos)
		}
	})

	t.Run("sort by title ascending", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Sort: "title", Order: "asc", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Videos[0].ID != "v1" || page.Videos[2].ID != "v3" {
			t.Fatalf("title sort = %+v", page.Videos)
		}
	})

	t.Run("sort by duration", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Sort: "duration", Order: "asc", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Videos[0].Duration != 100 || page.Videos[2].Duration != 300 {
			t.Fatalf("duration sort = %+v", page.Videos)
		}
	})
}

func TestVideoListAdvancedFilter(t *testing.T) {
	repo, sources := newVideoTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\x`)

	base := func(id, title string) domain.Video {
		return domain.Video{
			ID: id, SourceID: "s1", FileID: id, RelativePath: id + ".mp4", Path: "p",
			Size: 1, MTime: 1, Title: title,
			CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t",
		}
	}
	_ = repo.Create(ctx, base("v1", "Alpha"))
	_ = repo.Create(ctx, base("v2", "Beta"))
	_ = repo.Create(ctx, base("v3", "Gamma"))

	set := func(id, desc, genre string, year int, tags []string) {
		if err := repo.UpdateMetadata(ctx, id, domain.VideoPatch{
			Description: &desc, Genre: &genre, Year: &year,
		}); err != nil {
			t.Fatalf("metadata %s: %v", id, err)
		}
		if err := repo.SetTags(ctx, id, tags); err != nil {
			t.Fatalf("tags %s: %v", id, err)
		}
	}
	set("v1", "太空冒险", "科幻", 2001, []string{"科幻", "太空"})
	set("v2", "都市爱情", "科幻", 2010, []string{"爱情"})
	set("v3", "家庭喜剧", "喜剧", 2010, []string{"喜剧"})

	t.Run("desc", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Desc: "冒险", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 1 || page.Videos[0].ID != "v1" {
			t.Fatalf("desc = %d %+v", page.Total, page.Videos)
		}
	})

	t.Run("genre", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Genre: "科幻", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 2 {
			t.Fatalf("genre = %d", page.Total)
		}
	})

	t.Run("year", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Year: 2010, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 2 {
			t.Fatalf("year = %d", page.Total)
		}
	})

	t.Run("multi tag AND", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Tags: []string{"科幻", "太空"}, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 1 || page.Videos[0].ID != "v1" {
			t.Fatalf("multi tag = %d %+v", page.Total, page.Videos)
		}
		page, err = repo.List(ctx, domain.VideoQuery{Tags: []string{"科幻", "爱情"}, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 0 {
			t.Fatalf("conflicting tags should match none, got %d", page.Total)
		}
	})

	t.Run("combined", func(t *testing.T) {
		page, err := repo.List(ctx, domain.VideoQuery{Genre: "科幻", Year: 2010, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 1 || page.Videos[0].ID != "v2" {
			t.Fatalf("combined = %d %+v", page.Total, page.Videos)
		}
	})
}
