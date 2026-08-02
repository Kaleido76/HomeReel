package store

import (
	"context"
	"errors"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/domain"
)

func newVideoTestRepo(t *testing.T) (domain.VideoRepo, domain.StorageRepo) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewVideoRepo(database), NewStorageRepo(database)
}

func TestVideoCRUDAndFingerprint(t *testing.T) {
	repo, storages := newVideoTestRepo(t)
	ctx := context.Background()

	st := domain.Storage{ID: "s1", Name: "x", Type: domain.StorageTypeInternal, RootPath: "C:\\x", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := storages.Create(ctx, st); err != nil {
		t.Fatalf("create storage: %v", err)
	}

	v := domain.Video{
		ID: "v1", StorageID: "s1", FileID: "100", RelativePath: "a.mp4",
		Path: "C:\\x\\a.mp4", Size: 100, MTime: 1000,
		Title: "a", CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t",
	}
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(ctx, "v1")
	if err != nil || got.RelativePath != "a.mp4" || got.Size != 100 {
		t.Fatalf("get = %+v err=%v", got, err)
	}

	if err := repo.UpdateFingerprint(ctx, "v1", "C:\\x\\b.mp4", "b.mp4", 120, 2000, "scan2"); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	got, _ = repo.Get(ctx, "v1")
	if got.Path != "C:\\x\\b.mp4" || got.RelativePath != "b.mp4" || got.LastScannedAt != "scan2" {
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
	repo, storages := newVideoTestRepo(t)
	ctx := context.Background()
	st := domain.Storage{ID: "s1", Name: "x", Type: domain.StorageTypeInternal, RootPath: "C:\\x", CreatedAt: "t"}
	_ = storages.Create(ctx, st)

	base := domain.Video{
		StorageID: "s1", FileID: "1", Path: "C:\\x\\a.mp4",
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

	ids, err := repo.MarkMissing(ctx, "s1", "2026-01-02T00:00:00Z")
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

func TestVideoUniquePath(t *testing.T) {
	repo, storages := newVideoTestRepo(t)
	ctx := context.Background()
	st := domain.Storage{ID: "s1", Name: "x", Type: domain.StorageTypeInternal, RootPath: "C:\\x", CreatedAt: "t"}
	_ = storages.Create(ctx, st)
	base := func(id, rel string) domain.Video {
		return domain.Video{ID: id, StorageID: "s1", FileID: id, RelativePath: rel, Path: "p", Size: 1, MTime: 1, Title: "t", CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t"}
	}
	if err := repo.Create(ctx, base("v1", "a.mp4")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(ctx, base("v2", "a.mp4")); err == nil {
		t.Fatalf("duplicate path should fail")
	}
}

func TestVideoListPaginationAndFilter(t *testing.T) {
	repo, storages := newVideoTestRepo(t)
	ctx := context.Background()
	st := domain.Storage{ID: "s1", Name: "x", Type: domain.StorageTypeInternal, RootPath: "C:\\x", CreatedAt: "t"}
	_ = storages.Create(ctx, st)

	base := func(id, rel, title string, dur float64) domain.Video {
		return domain.Video{
			ID: id, StorageID: "s1", FileID: id, RelativePath: rel, Path: "p",
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
	// Create only stores the fingerprint columns; duration/codec come from
	// UpdateProbe, so persist them the way the scanner would.
	durations := map[string]float64{"v1": 100, "v2": 200, "v3": 300}
	for id, dur := range durations {
		v, _ := repo.Get(ctx, id)
		upd := v
		upd.Duration = dur
		upd.Codec = "h264"
		upd.Container = "mp4"
		if err := repo.UpdateProbe(ctx, upd); err != nil {
			t.Fatalf("probe %s: %v", id, err)
		}
	}

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
