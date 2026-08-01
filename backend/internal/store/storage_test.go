package store

import (
	"context"
	"errors"
	"testing"

	"videomesh/backend/internal/db"
	"videomesh/backend/internal/domain"
)

func newTestRepo(t *testing.T) domain.StorageRepo {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStorageRepo(database)
}

func TestStorageRepoCRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}

	in := domain.Storage{
		ID:        "1",
		Name:      "电影",
		Type:      domain.StorageTypeExternal,
		RootPath:  "E:\\Movies",
		DeviceID:  "2CE4-9F30",
		Readonly:  false,
		Enabled:   true,
		Available: true,
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := repo.Create(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "电影" || got.Type != domain.StorageTypeExternal ||
		got.DeviceID != "2CE4-9F30" || !got.Available || got.Readonly {
		t.Fatalf("unexpected storage: %+v", got)
	}

	in.Name = "外接电影库"
	in.Readonly = true
	if err := repo.Update(ctx, in); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.Get(ctx, "1")
	if got.Name != "外接电影库" || !got.Readonly {
		t.Fatalf("update not applied: %+v", got)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "1" {
		t.Fatalf("unexpected list: %+v", list)
	}

	if err := repo.Delete(ctx, "1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}
}
