package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oklog/ulid/v2"

	"videomesh/backend/internal/domain"
)

type collectionRepo struct {
	db *sql.DB
}

// NewCollectionRepo returns a SQLite-backed domain.CollectionRepo.
func NewCollectionRepo(database *sql.DB) domain.CollectionRepo {
	return &collectionRepo{db: database}
}

func (r *collectionRepo) List(ctx context.Context) ([]domain.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, created_at FROM collections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Collection{}
	for rows.Next() {
		var c domain.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *collectionRepo) Get(ctx context.Context, id string) (domain.Collection, error) {
	var c domain.Collection
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM collections WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Collection{}, domain.ErrNotFound
	}
	return c, err
}

func (r *collectionRepo) Create(ctx context.Context, name string) (domain.Collection, error) {
	c := domain.Collection{ID: ulid.Make().String(), Name: name, CreatedAt: nowRFC3339()}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, created_at) VALUES (?, ?, ?)`,
		c.ID, c.Name, c.CreatedAt); err != nil {
		return domain.Collection{}, err
	}
	return c, nil
}

func (r *collectionRepo) Rename(ctx context.Context, id, name string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE collections SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	return requireRows(res)
}

func (r *collectionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRows(res)
}

func (r *collectionRepo) Videos(ctx context.Context, collectionID string) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+videoCols+` FROM videos v
		WHERE EXISTS (SELECT 1 FROM collection_videos cv
			WHERE cv.collection_id = ? AND cv.video_id = v.id)
		ORDER BY v.created_at DESC`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *collectionRepo) AddVideo(ctx context.Context, collectionID, videoID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO collection_videos (collection_id, video_id) VALUES (?, ?)`,
		collectionID, videoID)
	return err
}

func (r *collectionRepo) RemoveVideo(ctx context.Context, collectionID, videoID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM collection_videos WHERE collection_id = ? AND video_id = ?`,
		collectionID, videoID)
	if err != nil {
		return err
	}
	return requireRows(res)
}

func requireRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
