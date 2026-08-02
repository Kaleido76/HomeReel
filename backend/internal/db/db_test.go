package db

import (
	"database/sql"
	"testing"
)

// TestMigrateFresh applies all migrations to an empty database and checks the
// Phase 3 tables and columns exist.
func TestMigrateFresh(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := Migrate(database); err != nil {
		t.Fatalf("migrate fresh: %v", err)
	}

	tables := []string{"shows", "seasons", "video_tags", "collections", "collection_videos", "videos_fts"}
	for _, table := range tables {
		if !tableExists(t, database, table) {
			t.Errorf("table %s missing after migration", table)
		}
	}

	cols := []string{"kind", "description", "show_id", "season_number", "episode_number",
		"episode_title", "year", "rating", "genre", "overview", "studio", "cast_text",
		"metadata_source", "search_text", "backdrop_path", "audio_codec", "fps", "file_size"}
	for _, col := range cols {
		if !columnExists(t, database, "videos", col) {
			t.Errorf("videos.%s missing after migration", col)
		}
	}
}

// TestMigrateUpgrade simulates a database created in Phase 0-2 (only the first
// six migrations applied) with pre-existing rows, then upgrades to the full
// schema and verifies data survives and FTS is backfilled.
func TestMigrateUpgrade(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Exec(`CREATE TABLE schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		for _, stmt := range splitStatements(migrations[i]) {
			if _, err := tx.Exec(stmt); err != nil {
				t.Fatalf("seed legacy migration %d (%q): %v", i+1, stmt, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			i+1, "2026-01-01T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO storages (id, name, type, root_path, readonly, enabled, available, created_at)
		VALUES ('s1', 'test', 'internal', 'C:\\Videos', 0, 1, 1, '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO videos (id, storage_id, file_id, relative_path, path, size, mtime,
		title, created_at, updated_at, last_scanned_at)
		VALUES ('v1', 's1', 'f1', 'Movie.mkv', 'C:\\Videos\\Movie.mkv', 1000, 100,
		'Movie', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z',
		'2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(database); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	var kind, source string
	if err := database.QueryRow(`SELECT kind, metadata_source FROM videos WHERE id = 'v1'`).
		Scan(&kind, &source); err != nil {
		t.Fatalf("read upgraded video: %v", err)
	}
	if kind != "movie" || source != "manual" {
		t.Errorf("defaults wrong after upgrade: kind=%q source=%q", kind, source)
	}

	var count int
	if err := database.QueryRow(`SELECT count(*) FROM videos_fts WHERE videos_fts MATCH 'Movie'`).
		Scan(&count); err != nil {
		t.Fatalf("fts backfill query: %v", err)
	}
	if count != 1 {
		t.Errorf("fts backfill failed: expected 1 match, got %d", count)
	}
}

func tableExists(t *testing.T, database *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := database.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return err == nil && n > 0
}

func columnExists(t *testing.T, database *sql.DB, table, col string) bool {
	t.Helper()
	rows, err := database.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma table_info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == col {
			return true
		}
	}
	return false
}
