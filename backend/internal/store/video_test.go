package store

import (
	"context"
	"errors"
	"testing"

	"videomesh/backend/internal/db"
	"videomesh/backend/internal/domain"
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
