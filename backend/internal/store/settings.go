package store

import (
	"context"
	"database/sql"
	"errors"

	"homereel/backend/internal/domain"
)

type settingsRepo struct {
	db *sql.DB
}

// NewSettingsRepo returns a SQLite-backed domain.SettingsRepo (key/value rows
// in the settings table, upserted on Set).
func NewSettingsRepo(database *sql.DB) domain.SettingsRepo {
	return &settingsRepo{db: database}
}

func (r *settingsRepo) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return v, err
}

func (r *settingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, domain.Now())
	return err
}
