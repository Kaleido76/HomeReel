package fservice

import (
	"context"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/store"
)

// TestGetPinsNeverNil guards the null-crash regression: a settings key that has
// never been written must decode to an empty JSON array ([]), not null, or the
// file tab's `pins.includes(path)` throws on null.
func TestGetPinsNeverNil(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	svc := New(nil, store.NewSettingsRepo(database), nil)
	ctx := context.Background()

	pins, err := svc.GetPins(ctx)
	if err != nil {
		t.Fatalf("GetPins: %v", err)
	}
	if pins == nil {
		t.Error("GetPins with no pins must return an empty non-nil slice (JSON [] not null)")
	}
}

// TestGetPinsMigratesLegacyKey verifies pins saved under the pre-rename key are
// copied to the current key on first read, so the rename does not lose them.
func TestGetPinsMigratesLegacyKey(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	svc := New(nil, store.NewSettingsRepo(database), nil)
	ctx := context.Background()

	want := []string{`C:\a`, `D:\b`}
	if err := svc.pins.Set(ctx, "fs2.pins", `["C:\\a","D:\\b"]`); err != nil {
		t.Fatal(err)
	}

	pins, err := svc.GetPins(ctx)
	if err != nil {
		t.Fatalf("GetPins: %v", err)
	}
	if len(pins) != 2 || pins[0] != want[0] || pins[1] != want[1] {
		t.Errorf("migrated pins = %v, want %v", pins, want)
	}
	raw, err := svc.pins.Get(ctx, "files.pins")
	if err != nil {
		t.Fatalf("files.pins not persisted after migration: %v", err)
	}
	if raw != `["C:\\a","D:\\b"]` {
		t.Errorf("files.pins = %q, want %q", raw, `["C:\\a","D:\\b"]`)
	}
}
