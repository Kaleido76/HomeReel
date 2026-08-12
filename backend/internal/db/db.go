package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	`CREATE TABLE history (
		video_id   TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
		user       TEXT NOT NULL DEFAULT 'local',
		progress   REAL NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (video_id, user)
	)`,
	// Phase 3 — media library (ADR-015 / ADR-016): shows/seasons, tags.
	// All timestamps use the fixed-width nanosecond layout.
	`CREATE TABLE shows (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		overview        TEXT,
		year            INTEGER,
		rating          REAL,
		genre           TEXT,
		poster_path     TEXT,
		backdrop_path   TEXT,
		metadata_source TEXT NOT NULL DEFAULT 'manual',
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL
	);
	CREATE INDEX idx_shows_name ON shows(name);
	CREATE TABLE seasons (
		id          TEXT PRIMARY KEY,
		show_id     TEXT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
		number      INTEGER NOT NULL,
		name        TEXT,
		overview    TEXT,
		poster_path TEXT,
		UNIQUE (show_id, number)
	);
	CREATE TABLE video_tags (
		video_id  TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
		tag       TEXT NOT NULL,
		PRIMARY KEY (video_id, tag)
	);
	CREATE INDEX idx_video_tags ON video_tags(tag);`,
	// Phase 3 — videos metadata & grouping columns (kind/show/season/episode,
	// scraped fields, denormalised search_text for FTS5).
	`ALTER TABLE videos ADD COLUMN kind TEXT NOT NULL DEFAULT 'movie';
	ALTER TABLE videos ADD COLUMN description TEXT NOT NULL DEFAULT '';
	ALTER TABLE videos ADD COLUMN audio_codec TEXT;
	ALTER TABLE videos ADD COLUMN fps REAL;
	ALTER TABLE videos ADD COLUMN file_size INTEGER;
	ALTER TABLE videos ADD COLUMN backdrop_path TEXT;
	ALTER TABLE videos ADD COLUMN show_id TEXT REFERENCES shows(id);
	ALTER TABLE videos ADD COLUMN season_number INTEGER;
	ALTER TABLE videos ADD COLUMN episode_number INTEGER;
	ALTER TABLE videos ADD COLUMN episode_title TEXT;
	ALTER TABLE videos ADD COLUMN year INTEGER;
	ALTER TABLE videos ADD COLUMN rating REAL;
	ALTER TABLE videos ADD COLUMN genre TEXT;
	ALTER TABLE videos ADD COLUMN overview TEXT;
	ALTER TABLE videos ADD COLUMN studio TEXT;
	ALTER TABLE videos ADD COLUMN cast_text TEXT;
	ALTER TABLE videos ADD COLUMN metadata_source TEXT NOT NULL DEFAULT 'manual';
	ALTER TABLE videos ADD COLUMN search_text TEXT;
	CREATE INDEX idx_videos_show ON videos(show_id, season_number, episode_number);
	CREATE INDEX idx_videos_kind ON videos(kind)`,
	// Phase 3 — FTS5 external-content index over videos (ADR-009), kept in
	// sync by triggers and backfilled for pre-existing rows.
	`CREATE VIRTUAL TABLE videos_fts USING fts5(
		content='videos', content_rowid='rowid',
		title, description, search_text
	);
	CREATE TRIGGER videos_ai AFTER INSERT ON videos BEGIN
		INSERT INTO videos_fts(rowid, title, description, search_text)
		VALUES (new.rowid, new.title, new.description, coalesce(new.search_text, ''));
	END;
	CREATE TRIGGER videos_ad AFTER DELETE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, search_text)
		VALUES ('delete', old.rowid, old.title, old.description, old.search_text);
	END;
	CREATE TRIGGER videos_au AFTER UPDATE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, search_text)
		VALUES ('delete', old.rowid, old.title, old.description, old.search_text);
		INSERT INTO videos_fts(rowid, title, description, search_text)
		VALUES (new.rowid, new.title, new.description, coalesce(new.search_text, ''));
	END;
	INSERT INTO videos_fts(rowid, title, description, search_text)
		SELECT rowid, title, description, search_text FROM videos;`,
	// Phase 3 — remove a show automatically when its last episode is deleted.
	`CREATE TRIGGER videos_bd AFTER DELETE ON videos BEGIN
		DELETE FROM shows WHERE id = OLD.show_id
			AND NOT EXISTS (SELECT 1 FROM videos WHERE show_id = OLD.show_id);
	END`,
	// 视频库组织：系列 = 一季/一部（seasons 行），seasons 增加 kind
	// （tv=剧集季 | movie=电影部）；series_links 存系列间弱关联（无名称、
	// 有排序，ADR 见 plan 6.10 扩展）。
	`ALTER TABLE seasons ADD COLUMN kind TEXT NOT NULL DEFAULT 'tv';
	CREATE TABLE series_links (
		series_id        TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
		linked_series_id TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
		sort_index       INTEGER NOT NULL DEFAULT 0,
		created_at       TEXT NOT NULL,
		PRIMARY KEY (series_id, linked_series_id)
	)`,
	// 移除集合子系统（collections）：视频库以单集/系列组织，集合不再需要。
	// 旧库已建表，这里物理删除；新库从未创建。
	`DROP TABLE IF EXISTS collection_videos;
	DROP TABLE IF EXISTS collections`,
	// 分段 MP4（多个 mdat 盒子 / moof 分片，hls.js 拼接文件）：Chrome 直连
	// 会整文件下载，探测时标记 segmented，前端据此判不可直连并引导转换。
	`ALTER TABLE videos ADD COLUMN segmented INTEGER NOT NULL DEFAULT 0`,
	// 长时任务（jobs）：name 用于任务面板展示，internal 标记探测/缩略图等
	// 内部短任务（对用户隐藏、不发通知）。
	`ALTER TABLE jobs ADD COLUMN name TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN internal INTEGER NOT NULL DEFAULT 0`,
	// 2026-08 多媒体源重构：视频库入库从「存储卷（storages）」改为「多媒体源
	// （media_sources）」。库清空重建（用户决策）；videos.storage_id 迁移为
	// source_id（可空，预留离散资源）；storages 表整体删除。FTS 外部内容表
	// 与触发器随 videos 一并重建。
	// 清空顺序很关键：必须先 DELETE FROM videos（顺带经 CASCADE 清空
	// video_tags/history），再删 seasons/shows —— 若先 DROP videos，shows 的
	// FK 检查会因找不到 videos 表而报错；若先删 shows，又有 videos 行引用。
	`DELETE FROM videos;
	DELETE FROM series_links;
	DELETE FROM seasons;
	DELETE FROM shows;
	DELETE FROM video_tags;
	DELETE FROM history;
	DROP TRIGGER IF EXISTS videos_ai;
	DROP TRIGGER IF EXISTS videos_ad;
	DROP TRIGGER IF EXISTS videos_au;
	DROP TRIGGER IF EXISTS videos_bd;
	DROP TABLE IF EXISTS videos_fts;
	DROP TABLE IF EXISTS videos;
	DROP TABLE IF EXISTS storages;
	CREATE TABLE media_sources (
		id           TEXT PRIMARY KEY,
		path         TEXT NOT NULL UNIQUE,
		created_at   TEXT NOT NULL,
		last_scan_at TEXT
	);
	CREATE TABLE videos (
		id              TEXT PRIMARY KEY,
		source_id       TEXT REFERENCES media_sources(id) ON DELETE SET NULL,
		file_id         TEXT NOT NULL,
		relative_path   TEXT NOT NULL,
		path            TEXT NOT NULL,
		size            INTEGER NOT NULL DEFAULT 0,
		mtime           INTEGER NOT NULL DEFAULT 0,
		title           TEXT NOT NULL DEFAULT '',
		kind            TEXT NOT NULL DEFAULT 'movie',
		description     TEXT NOT NULL DEFAULT '',
		duration        REAL,
		codec           TEXT,
		audio_codec     TEXT,
		container       TEXT,
		segmented       INTEGER NOT NULL DEFAULT 0,
		width           INTEGER,
		height          INTEGER,
		fps             REAL,
		file_size       INTEGER,
		cover_path      TEXT,
		thumb_path      TEXT,
		backdrop_path   TEXT,
		show_id         TEXT REFERENCES shows(id),
		season_number   INTEGER,
		episode_number  INTEGER,
		episode_title   TEXT,
		year            INTEGER,
		rating          REAL,
		genre           TEXT,
		overview        TEXT,
		studio          TEXT,
		cast_text       TEXT,
		metadata_source TEXT NOT NULL DEFAULT 'manual',
		search_text     TEXT,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		last_scanned_at TEXT NOT NULL,
		UNIQUE (source_id, relative_path)
	);
	CREATE INDEX idx_videos_source ON videos(source_id);
	CREATE INDEX idx_videos_file ON videos(file_id);
	CREATE INDEX idx_videos_show ON videos(show_id, season_number, episode_number);
	CREATE INDEX idx_videos_kind ON videos(kind);
	CREATE VIRTUAL TABLE videos_fts USING fts5(
		content='videos', content_rowid='rowid',
		title, description, search_text
	);
	CREATE TRIGGER videos_ai AFTER INSERT ON videos BEGIN
		INSERT INTO videos_fts(rowid, title, description, search_text)
		VALUES (new.rowid, new.title, new.description, coalesce(new.search_text, ''));
	END;
	CREATE TRIGGER videos_ad AFTER DELETE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, search_text)
		VALUES ('delete', old.rowid, old.title, old.description, old.search_text);
	END;
	CREATE TRIGGER videos_au AFTER UPDATE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, search_text)
		VALUES ('delete', old.rowid, old.title, old.description, old.search_text);
		INSERT INTO videos_fts(rowid, title, description, search_text)
		VALUES (new.rowid, new.title, new.description, coalesce(new.search_text, ''));
	END;
	CREATE TRIGGER videos_bd AFTER DELETE ON videos BEGIN
		DELETE FROM shows WHERE id = OLD.show_id
			AND NOT EXISTS (SELECT 1 FROM videos WHERE show_id = OLD.show_id);
	END;`,
	// 手动资源（离散/手动系列）：用户在文件页签标记的路径，独立于媒体源维护。
	// videos.resource_id 指向标记；释放标记经 ON DELETE CASCADE 释放全部元数据。
	`CREATE TABLE manual_resources (
		id            TEXT PRIMARY KEY,
		path          TEXT NOT NULL UNIQUE,
		kind          TEXT NOT NULL,   -- discrete（离散单集）| series（手动系列）
		created_at    TEXT NOT NULL,
		last_check_at TEXT
	);
	ALTER TABLE videos ADD COLUMN resource_id TEXT REFERENCES manual_resources(id) ON DELETE CASCADE;
	CREATE INDEX idx_videos_resource ON videos(resource_id);`,
	// 离散资源不维护存在性（惰性提示由详情页承担）：库内删除某个离散视频（释放）
	// 后，若其所属手动资源不再有成员，自动清理空资源标记（与 videos_bd 删空
	// show 一致）。
	`CREATE TRIGGER manual_resources_bd AFTER DELETE ON videos BEGIN
		DELETE FROM manual_resources WHERE id = OLD.resource_id
			AND NOT EXISTS (SELECT 1 FROM videos WHERE resource_id = OLD.resource_id);
	END`,
	// 2026-08 管理面换代：单集为唯一基本实体（必须归属媒体源），系列是用户
	// 显式创建的管理容器（绑定根目录，成员 = 根目录直接一级子文件），系列只
	// 能手动创建、扫描只维护成员关系。彻底清除离散单集/离散系列概念。
	// 开发期清库重建（用户决策）；manual_resources/resource_id 整体移除；
	// seasons.root_path 承载系列↔根目录绑定。
	`DELETE FROM videos;
	DELETE FROM series_links;
	DELETE FROM seasons;
	DELETE FROM shows;
	DELETE FROM video_tags;
	DELETE FROM history;
	DROP TRIGGER IF EXISTS manual_resources_bd;
	DROP TABLE IF EXISTS manual_resources;
	DROP INDEX IF EXISTS idx_videos_resource;
	ALTER TABLE videos DROP COLUMN resource_id;
	ALTER TABLE seasons ADD COLUMN root_path TEXT;
	CREATE UNIQUE INDEX idx_seasons_root ON seasons(root_path) WHERE root_path IS NOT NULL;`,
	// 播放能力元数据（2026-08）：audio_codec 供「前后端协同能力判定」核对音频
	// 是否可解码；faststart 标记 mp4 家族文件是否 moov 前置（浏览器可立即 seek）。
	`ALTER TABLE videos ADD COLUMN faststart INTEGER NOT NULL DEFAULT 0`,
	// 统一资源进口管线（ADR-017）：title_source 区分「文件派生标题」（file，
	// 扫描/探测时随文件名刷新）与「用户手动编辑标题」（manual，永不被覆盖）。
	// 系列成员标题始终随文件名刷新（BindMembers 置回 file）。
	`ALTER TABLE videos ADD COLUMN title_source TEXT NOT NULL DEFAULT 'file'`,
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

// splitStatements splits a migration body into individual SQL statements so a
// single migration version can perform several DDL changes atomically. It
// understands string literals and BEGIN...END blocks (triggers), whose inner
// semicolons must not split.
func splitStatements(ddl string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	i, n := 0, len(ddl)
	for i < n {
		c := ddl[i]
		switch {
		case c == '\'' || c == '"':
			cur.WriteByte(c)
			i++
			for i < n {
				if ddl[i] == c {
					if i+1 < n && ddl[i+1] == c { // escaped literal ('' or "")
						cur.WriteByte(c)
						cur.WriteByte(c)
						i += 2
						continue
					}
					cur.WriteByte(c)
					i++
					break
				}
				cur.WriteByte(ddl[i])
				i++
			}
		case c == ';' && depth == 0:
			if stmt := strings.TrimSpace(cur.String()); stmt != "" {
				out = append(out, stmt)
			}
			cur.Reset()
			i++
		default:
			if isWordByte(c) {
				start := i
				for i < n && isWordByte(ddl[i]) {
					i++
				}
				switch strings.ToUpper(ddl[start:i]) {
				case "BEGIN":
					depth++
				case "END":
					if depth > 0 {
						depth--
					}
				}
				cur.WriteString(ddl[start:i])
			} else {
				cur.WriteByte(c)
				i++
			}
		}
	}
	if stmt := strings.TrimSpace(cur.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func applyMigration(database *sql.DB, version int, ddl string) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range splitStatements(ddl) {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	return tx.Commit()
}
