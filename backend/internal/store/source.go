package store

import (
	"context"
	"database/sql"
	"errors"

	"homereel/backend/internal/domain"
)

type sourceRepo struct {
	db *sql.DB
}

// NewSourceRepo returns a SQLite-backed domain.SourceRepo for multimedia
// source markers.
func NewSourceRepo(database *sql.DB) domain.SourceRepo {
	return &sourceRepo{db: database}
}

const sourceCols = "id, path, created_at, last_scan_at"

func scanSource(row scanner) (domain.MediaSource, error) {
	var (
		s         domain.MediaSource
		lastScan  sql.NullString
		createdAt string
	)
	if err := row.Scan(&s.ID, &s.Path, &createdAt, &lastScan); err != nil {
		return domain.MediaSource{}, err
	}
	s.CreatedAt = createdAt
	if lastScan.Valid {
		s.LastScanAt = lastScan.String
	}
	return s, nil
}

func (r *sourceRepo) List(ctx context.Context) ([]domain.MediaSource, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+sourceCols+` FROM media_sources ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.MediaSource, 0)
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sourceRepo) Get(ctx context.Context, id string) (domain.MediaSource, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+sourceCols+` FROM media_sources WHERE id = ?`, id)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MediaSource{}, domain.ErrNotFound
	}
	return s, err
}

func (r *sourceRepo) GetByPath(ctx context.Context, path string) (domain.MediaSource, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+sourceCols+` FROM media_sources WHERE path = ?`, path)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MediaSource{}, domain.ErrNotFound
	}
	return s, err
}

func (r *sourceRepo) Create(ctx context.Context, s domain.MediaSource) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO media_sources (id, path, created_at, last_scan_at)
		VALUES (?, ?, ?, ?)`, s.ID, s.Path, s.CreatedAt, nullString(s.LastScanAt))
	return err
}

func (r *sourceRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM media_sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *sourceRepo) TouchLastScan(ctx context.Context, id, lastScanAt string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE media_sources SET last_scan_at = ? WHERE id = ?`, lastScanAt, id)
	return err
}
