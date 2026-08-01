package storage

import (
	"context"
	"errors"
	"testing"

	"videomesh/backend/internal/domain"
)

type memRepo struct {
	items map[string]domain.Storage
	order []string
}

func newMemRepo() *memRepo {
	return &memRepo{items: map[string]domain.Storage{}}
}

func (m *memRepo) List(context.Context) ([]domain.Storage, error) {
	out := make([]domain.Storage, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.items[id])
	}
	return out, nil
}

func (m *memRepo) Get(_ context.Context, id string) (domain.Storage, error) {
	s, ok := m.items[id]
	if !ok {
		return domain.Storage{}, domain.ErrNotFound
	}
	return s, nil
}

func (m *memRepo) Create(_ context.Context, s domain.Storage) error {
	m.items[s.ID] = s
	m.order = append(m.order, s.ID)
	return nil
}

func (m *memRepo) Update(_ context.Context, s domain.Storage) error {
	if _, ok := m.items[s.ID]; !ok {
		return domain.ErrNotFound
	}
	m.items[s.ID] = s
	return nil
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func TestCreateAssignsIDAndProbes(t *testing.T) {
	svc := New(newMemRepo())
	root := t.TempDir()
	in := domain.Storage{
		Name:     "内置",
		Type:     domain.StorageTypeInternal,
		RootPath: root,
	}
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == "" || got.CreatedAt == "" {
		t.Fatalf("id/created_at not set: %+v", got)
	}
	if !got.Available {
		t.Fatalf("existing dir should probe available: %+v", got)
	}
}

func TestCreateValidation(t *testing.T) {
	svc := New(newMemRepo())
	ctx := context.Background()

	if _, err := svc.Create(ctx, domain.Storage{Name: "", Type: domain.StorageTypeInternal, RootPath: "C:\\x"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty name = %v, want ErrInvalid", err)
	}
	if _, err := svc.Create(ctx, domain.Storage{Name: "x", Type: domain.StorageTypeInternal, RootPath: ""}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty root = %v, want ErrInvalid", err)
	}
	if _, err := svc.Create(ctx, domain.Storage{Name: "x", Type: "blob", RootPath: "C:\\x"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad type = %v, want ErrInvalid", err)
	}
	if _, err := svc.Create(ctx, domain.Storage{Name: "x", Type: domain.StorageTypeExternal, RootPath: "C:\\x", DeviceID: ""}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("external without device_id = %v, want ErrInvalid", err)
	}
}

func TestProbeMissingDirOffline(t *testing.T) {
	svc := New(newMemRepo())
	got, err := svc.Create(context.Background(), domain.Storage{
		Name:     "x",
		Type:     domain.StorageTypeInternal,
		RootPath: "C:\\definitely\\missing\\videomesh-test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Available {
		t.Fatalf("missing dir should probe offline: %+v", got)
	}
}

func TestUpdatePatch(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo)
	ctx := context.Background()
	got, err := svc.Create(ctx, domain.Storage{
		Name: "a", Type: domain.StorageTypeInternal, RootPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	name := "b"
	readonly := true
	updated, err := svc.Update(ctx, got.ID, Patch{Name: &name, Readonly: &readonly})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "b" || !updated.Readonly || updated.Type != domain.StorageTypeInternal {
		t.Fatalf("patch not applied: %+v", updated)
	}

	if _, err := svc.Update(ctx, "missing", Patch{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update missing = %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	svc := New(newMemRepo())
	ctx := context.Background()
	got, err := svc.Create(ctx, domain.Storage{Name: "a", Type: domain.StorageTypeInternal, RootPath: t.TempDir()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, got.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}
}
