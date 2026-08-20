package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
)

type devLogRepo struct {
	db *sql.DB
}

// NewDevLogRepo returns a SQLite-backed domain.DevLogRepo. Archives are stored
// as one row per submission; the entries JSON is written verbatim so a fetched
// archive reproduces exactly what the device captured.
func NewDevLogRepo(database *sql.DB) domain.DevLogRepo {
	return &devLogRepo{db: database}
}

func (r *devLogRepo) Create(ctx context.Context, log *domain.DevLog) error {
	if log.ID == "" {
		log.ID = ulid.Make().String()
	}
	if log.CreatedAt == "" {
		log.CreatedAt = domain.Now()
	}
	entries, err := json.Marshal(log.Entries)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO devlogs (id, source, note, entries, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		log.ID, log.Source, log.Note, string(entries), log.CreatedAt)
	return err
}

func (r *devLogRepo) List(ctx context.Context) ([]domain.DevLogSummary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, source, note, entries, created_at FROM devlogs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.DevLogSummary{}
	for rows.Next() {
		var id, source, note, entriesRaw, created string
		if err := rows.Scan(&id, &source, &note, &entriesRaw, &created); err != nil {
			return nil, err
		}
		var entries []domain.DevLogEntry
		_ = json.Unmarshal([]byte(entriesRaw), &entries)
		out = append(out, domain.DevLogSummary{
			ID: id, Source: source, Note: note, Count: len(entries), CreatedAt: created,
		})
	}
	return out, rows.Err()
}

func (r *devLogRepo) Get(ctx context.Context, id string) (domain.DevLog, error) {
	var log domain.DevLog
	var entriesRaw string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, source, note, entries, created_at FROM devlogs WHERE id = ?`, id).
		Scan(&log.ID, &log.Source, &log.Note, &entriesRaw, &log.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DevLog{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DevLog{}, err
	}
	if err := json.Unmarshal([]byte(entriesRaw), &log.Entries); err != nil {
		return domain.DevLog{}, err
	}
	return log, nil
}

func (r *devLogRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM devlogs WHERE id = ?`, id)
	return err
}
