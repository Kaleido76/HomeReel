package store

import (
	"context"
	"database/sql"
	"errors"

	"homereel/backend/internal/jobs"
)

type jobRepo struct {
	db *sql.DB
}

// NewJobRepo returns a SQLite-backed jobs.Repo.
func NewJobRepo(database *sql.DB) jobs.Repo {
	return &jobRepo{db: database}
}

func (r *jobRepo) Enqueue(ctx context.Context, j jobs.Job) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs (id, type, target, extra, status, progress, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Type, j.Target, j.Extra, j.Status, j.Progress, j.Error, j.CreatedAt, j.UpdatedAt)
	return err
}

func (r *jobRepo) ClaimNext(ctx context.Context) (jobs.Job, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return jobs.Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx,
		`SELECT id, type, target, extra, status, progress, error, created_at, updated_at
		 FROM jobs WHERE status = ? ORDER BY created_at, id LIMIT 1`, jobs.StatusQueued)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, false, nil
	}
	if err != nil {
		return jobs.Job{}, false, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`,
		jobs.StatusRunning, nowRFC3339(), j.ID); err != nil {
		return jobs.Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return jobs.Job{}, false, err
	}
	j.Status = jobs.StatusRunning
	return j, true, nil
}

func (r *jobRepo) MarkDone(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, progress = 1, updated_at = ? WHERE id = ?`,
		jobs.StatusDone, nowRFC3339(), id)
	return err
}

func (r *jobRepo) MarkFailed(ctx context.Context, id, errMsg string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		jobs.StatusFailed, errMsg, nowRFC3339(), id)
	return err
}

func (r *jobRepo) List(ctx context.Context, limit int) ([]jobs.Job, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, type, target, extra, status, progress, error, created_at, updated_at
		FROM jobs ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]jobs.Job, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *jobRepo) ResetRunning(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = ? WHERE status = ?`,
		jobs.StatusQueued, nowRFC3339(), jobs.StatusRunning)
	return err
}

func scanJob(row scanner) (jobs.Job, error) {
	var j jobs.Job
	err := row.Scan(&j.ID, &j.Type, &j.Target, &j.Extra, &j.Status, &j.Progress,
		&j.Error, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}
