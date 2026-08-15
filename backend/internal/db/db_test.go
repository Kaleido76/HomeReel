package db

import (
	"database/sql"
	"strings"
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

	tables := []string{"shows", "seasons", "video_tags", "videos_fts", "media_sources"}
	for _, table := range tables {
		if !tableExists(t, database, table) {
			t.Errorf("table %s missing after migration", table)
		}
	}
	for _, gone := range []string{"collections", "collection_videos", "storages", "manual_resources"} {
		if tableExists(t, database, gone) {
			t.Errorf("table %s should have been dropped (collections / storages / manual resources removed)", gone)
		}
	}

	cols := []string{"kind", "show_id", "season_number", "episode_number",
		"episode_title", "year", "rating", "genre", "studio", "cast_text",
		"metadata_source", "search_text", "backdrop_path", "audio_codec", "fps", "file_size",
		"source_id", "faststart", "title_source"}
	for _, col := range cols {
		if !columnExists(t, database, "videos", col) {
			t.Errorf("videos.%s missing after migration", col)
		}
	}
	if columnExists(t, database, "videos", "resource_id") {
		t.Errorf("videos.resource_id should have been dropped (management model change)")
	}
	if columnExists(t, database, "videos", "description") {
		t.Errorf("videos.description should have been dropped (单集简介移除)")
	}
	if columnExists(t, database, "videos", "overview") {
		t.Errorf("videos.overview should have been dropped (单集简介移除)")
	}
	if columnExists(t, database, "videos", "storage_id") {
		t.Errorf("videos.storage_id should have been replaced by source_id")
	}
	if !columnExists(t, database, "seasons", "root_path") {
		t.Errorf("seasons.root_path missing after migration")
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

	// The multimedia-source migration clears the library (用户决策：清空重建)
	// and replaces the storage-volume model, so the legacy video is gone.
	if !tableExists(t, database, "media_sources") {
		t.Errorf("media_sources missing after upgrade")
	}
	if tableExists(t, database, "storages") {
		t.Errorf("storages should have been dropped after upgrade")
	}
	var v1 string
	if err := database.QueryRow(`SELECT id FROM videos WHERE id = 'v1'`).Scan(&v1); err != sql.ErrNoRows {
		t.Errorf("legacy video should have been cleared after upgrade, err=%v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT count(*) FROM videos_fts WHERE videos_fts MATCH 'Movie'`).
		Scan(&count); err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if count != 0 {
		t.Errorf("fts should be empty after cleared library, got %d", count)
	}
}

// TestMigrateClearsLibrary simulates a database built by every migration except
// the final management-model change (which carries the dev-time clear), seeds
// library rows, then applies the last migration and asserts it wipes every
// library table and drops the discrete-resource machinery.
func TestMigrateClearsLibrary(t *testing.T) {
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
	// 应用「管理面换代清库迁移」之前的所有迁移（按内容定位，追加的新迁移不会
	// 破坏本测试），模拟管理面换代之前、但已含手动资源机制的历史库。
	cut := len(migrations)
	for i, ddl := range migrations {
		if strings.Contains(ddl, "ALTER TABLE seasons ADD COLUMN root_path") {
			cut = i
			break
		}
	}
	for i := 0; i < cut; i++ {
		for _, stmt := range splitStatements(migrations[i]) {
			if _, err := tx.Exec(stmt); err != nil {
				t.Fatalf("apply legacy migration %d (%q): %v", i+1, stmt, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			i+1, "2026-01-01T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO media_sources (id, path, created_at) VALUES ('ms1', 'C:\\Videos', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO manual_resources (id, path, kind, created_at) VALUES ('r1', 'C:\\Videos\\Clip', 'discrete', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO shows (id, name, metadata_source, created_at, updated_at)
		VALUES ('show1', 'Leftover Show', 'manual', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO seasons (id, show_id, number, kind) VALUES ('sea1', 'show1', 1, 'tv')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO videos (id, source_id, resource_id, file_id, relative_path, path, size, mtime,
		title, kind, created_at, updated_at, last_scanned_at)
		VALUES ('v1', 'ms1', 'r1', 'f1', 'Show.S01E01.mkv', 'C:\\Videos\\Show.S01E01.mkv', 1000, 100,
		'Show', 'episode', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z',
		'2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(database); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	// 库内容表全部清空；media_sources 是扫描单元注册表，不属于库内容，保留。
	for _, table := range []string{"videos", "shows", "seasons", "series_links", "video_tags", "history"} {
		var n int
		if err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s should be empty after the clearing migration, got %d rows", table, n)
		}
	}
	if tableExists(t, database, "manual_resources") {
		t.Errorf("manual_resources should have been dropped")
	}
	if columnExists(t, database, "videos", "resource_id") {
		t.Errorf("videos.resource_id should have been dropped")
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
