package fservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/store"
)

func sourceService(t *testing.T) *Service {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	return New(nil, store.NewSettingsRepo(database), store.NewSourceRepo(database), "ffmpeg", "ffprobe")
}

// TestAddSourceRejectsNesting verifies media sources cannot overlap: marking a
// directory inside an existing source (or one that contains it) is rejected,
// siblings and re-marks stay legal (ADR-017).
func TestAddSourceRejectsNesting(t *testing.T) {
	svc := sourceService(t)
	ctx := context.Background()
	base := t.TempDir()
	parent := filepath.Join(base, "videos")
	child := filepath.Join(parent, "movies")
	for _, d := range []string{parent, child} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := svc.AddSource(ctx, parent); err != nil {
		t.Fatalf("add parent: %v", err)
	}
	if _, err := svc.AddSource(ctx, child); !errors.Is(err, ErrNestedSource) {
		t.Fatalf("nested-inside add = %v, want ErrNestedSource", err)
	}
	if _, err := svc.AddSource(ctx, base); !errors.Is(err, ErrNestedSource) {
		t.Fatalf("nested-contains add = %v, want ErrNestedSource", err)
	}

	sibling := filepath.Join(base, "music")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddSource(ctx, sibling); err != nil {
		t.Fatalf("sibling add: %v", err)
	}
	if _, err := svc.AddSource(ctx, parent); err != nil {
		t.Fatalf("re-marking the same path: %v", err)
	}
}
