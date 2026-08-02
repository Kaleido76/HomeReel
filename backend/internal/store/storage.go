package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"homereel/backend/internal/domain"
)

type storageRepo struct {
	db *sql.DB
}

// NewStorageRepo returns a SQLite-backed domain.StorageRepo.
func NewStorageRepo(database *sql.DB) domain.StorageRepo {
	return &storageRepo{db: database}
}

const storageCols = "id, name, type, root_path, device_id, readonly, enabled, available, created_at"

type scanner interface {
	Scan(dest ...any) error
}

func scanStorage(row scanner) (domain.Storage, error) {
	var (
		s         domain.Storage
		typ       string
		readonly  int
		enabled   int
		available int
		deviceID  sql.NullString
	)
	if err := row.Scan(&s.ID, &s.Name, &typ, &s.RootPath, &deviceID,
		&readonly, &enabled, &available, &s.CreatedAt); err != nil {
		return domain.Storage{}, err
	}
	s.Type = domain.StorageType(typ)
	if deviceID.Valid {
		s.DeviceID = deviceID.String
	}
	s.Readonly = readonly != 0
	s.Enabled = enabled != 0
	s.Available = available != 0
	return s, nil
}

func (r *storageRepo) List(ctx context.Context) ([]domain.Storage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+storageCols+` FROM storages ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list storages: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Storage, 0)
	for rows.Next() {
		s, err := scanStorage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *storageRepo) Get(ctx context.Context, id string) (domain.Storage, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+storageCols+` FROM storages WHERE id = ?`, id)
	s, err := scanStorage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Storage{}, domain.ErrNotFound
	}
	return s, err
}

func (r *storageRepo) Create(ctx context.Context, s domain.Storage) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO storages (id, name, type, root_path, device_id, readonly, enabled, available, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, string(s.Type), s.RootPath, nullString(s.DeviceID),
		boolInt(s.Readonly), boolInt(s.Enabled), boolInt(s.Available), s.CreatedAt)
	return err
}

func (r *storageRepo) Update(ctx context.Context, s domain.Storage) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE storages
		SET name = ?, type = ?, root_path = ?, device_id = ?, readonly = ?, enabled = ?
		WHERE id = ?`,
		s.Name, string(s.Type), s.RootPath, nullString(s.DeviceID),
		boolInt(s.Readonly), boolInt(s.Enabled), s.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *storageRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM storages WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
