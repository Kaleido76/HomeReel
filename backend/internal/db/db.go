package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (creating if needed) the SQLite database at <dataDir>/data.db
// with WAL journaling, foreign keys and a busy timeout (ADR-005).
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, "data.db")
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(1) // single writer avoids SQLITE_BUSY under WAL
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return database, nil
}

var migrations = []string{
	`CREATE TABLE sessions (
		token      TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	)`,
	`CREATE TABLE settings (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE storages (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		type        TEXT NOT NULL DEFAULT 'internal',
		root_path   TEXT NOT NULL,
		device_id   TEXT,
		readonly    INTEGER NOT NULL DEFAULT 0,
		enabled     INTEGER NOT NULL DEFAULT 1,
		available   INTEGER NOT NULL DEFAULT 1,
		created_at  TEXT NOT NULL
	)`,
	`CREATE UNIQUE INDEX idx_storages_device_id
		ON storages(device_id) WHERE device_id IS NOT NULL`,
	`CREATE TABLE videos (
		id               TEXT PRIMARY KEY,
		storage_id       TEXT NOT NULL REFERENCES storages(id) ON DELETE CASCADE,
		file_id          TEXT NOT NULL,
		relative_path    TEXT NOT NULL,
		path             TEXT NOT NULL,
		size             INTEGER NOT NULL DEFAULT 0,
		mtime            INTEGER NOT NULL DEFAULT 0,
		title            TEXT NOT NULL DEFAULT '',
		duration         REAL,
		codec            TEXT,
		container        TEXT,
		width            INTEGER,
		height           INTEGER,
		cover_path       TEXT,
		thumb_path       TEXT,
		created_at       TEXT NOT NULL,
		updated_at       TEXT NOT NULL,
		last_scanned_at  TEXT NOT NULL,
		UNIQUE (storage_id, relative_path)
	)`,
	`CREATE INDEX idx_videos_storage ON videos(storage_id, relative_path)`,
	`CREATE INDEX idx_videos_file ON videos(storage_id, file_id)`,
	`CREATE TABLE jobs (
		id         TEXT PRIMARY KEY,
		type       TEXT NOT NULL,
		target     TEXT NOT NULL,
		extra      TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL,
		progress   REAL NOT NULL DEFAULT 0,
		error      TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX idx_jobs_status ON jobs(status)`,
}

// Migrate applies pending migrations in order, tracking applied versions in
// schema_migrations.
func Migrate(database *sql.DB) error {
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for i, ddl := range migrations {
		version := i + 1
		var applied int
		err := database.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if err == sql.ErrNoRows {
			if err := applyMigration(database, version, ddl); err != nil {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
	}
	return nil
}

func applyMigration(database *sql.DB, version int, ddl string) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(ddl); err != nil {
		return fmt.Errorf("apply migration %d: %w", version, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	return tx.Commit()
}
