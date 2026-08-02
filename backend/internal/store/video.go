package store

import (
	"context"
	"database/sql"
	"errors"

	"videomesh/backend/internal/domain"
)

type videoRepo struct {
	db *sql.DB
}

// NewVideoRepo returns a SQLite-backed domain.VideoRepo.
func NewVideoRepo(database *sql.DB) domain.VideoRepo {
	return &videoRepo{db: database}
}

const videoCols = `id, storage_id, file_id, relative_path, path, size, mtime, title,
	duration, codec, container, width, height, cover_path, thumb_path,
	created_at, updated_at, last_scanned_at`

func scanVideo(row scanner) (domain.Video, error) {
	var (
		v         domain.Video
		duration  sql.NullFloat64
		codec     sql.NullString
		container sql.NullString
		width     sql.NullInt64
		height    sql.NullInt64
		cover     sql.NullString
		thumb     sql.NullString
	)
	if err := row.Scan(&v.ID, &v.StorageID, &v.FileID, &v.RelativePath, &v.Path,
		&v.Size, &v.MTime, &v.Title, &duration, &codec, &container, &width, &height,
		&cover, &thumb, &v.CreatedAt, &v.UpdatedAt, &v.LastScannedAt); err != nil {
		return domain.Video{}, err
	}
	if duration.Valid {
		v.Duration = duration.Float64
	}
	if codec.Valid {
		v.Codec = codec.String
	}
	if container.Valid {
		v.Container = container.String
	}
	if width.Valid {
		v.Width = int(width.Int64)
	}
	if height.Valid {
		v.Height = int(height.Int64)
	}
	if cover.Valid {
		v.CoverPath = cover.String
	}
	if thumb.Valid {
		v.ThumbPath = thumb.String
	}
	return v, nil
}

func (r *videoRepo) Get(ctx context.Context, id string) (domain.Video, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+videoCols+` FROM videos WHERE id = ?`, id)
	v, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, domain.ErrNotFound
	}
	return v, err
}

// listSort maps a user-facing sort key to a safe SQL ORDER BY expression.
var listSort = map[string]string{
	"title":    "title",
	"name":     "relative_path",
	"date":     "created_at",
	"duration": "duration",
}

func (r *videoRepo) List(ctx context.Context, q domain.VideoQuery) (domain.VideoPage, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 24
	}
	if pageSize > 200 {
		pageSize = 200
	}

	where := ""
	args := []any{}
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = ` WHERE (title LIKE ? OR relative_path LIKE ?)`
		args = append(args, like, like)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM videos`+where, args...).Scan(&total); err != nil {
		return domain.VideoPage{}, err
	}

	order := "DESC"
	if q.Order == "asc" {
		order = "ASC"
	}
	sort := listSort[q.Sort]
	if sort == "" {
		sort = "created_at"
	}
	query := `SELECT ` + videoCols + ` FROM videos` + where +
		` ORDER BY ` + sort + ` ` + order +
		` LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.VideoPage{}, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0, pageSize)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return domain.VideoPage{}, err
		}
		out = append(out, v)
	}
	return domain.VideoPage{Videos: out, Total: total}, rows.Err()
}

func (r *videoRepo) Create(ctx context.Context, v domain.Video) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO videos (id, storage_id, file_id, relative_path, path, size, mtime,
			title, created_at, updated_at, last_scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.StorageID, v.FileID, v.RelativePath, v.Path, v.Size, v.MTime,
		v.Title, v.CreatedAt, v.UpdatedAt, v.LastScannedAt)
	return err
}

func (r *videoRepo) UpdateFingerprint(ctx context.Context, id, path, relativePath string, size, mtime int64, lastScannedAt string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos SET path = ?, relative_path = ?, size = ?, mtime = ?,
			updated_at = ?, last_scanned_at = ?
		WHERE id = ?`,
		path, relativePath, size, mtime, nowRFC3339(), lastScannedAt, id)
	return err
}

func (r *videoRepo) Touch(ctx context.Context, id, lastScannedAt string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET last_scanned_at = ? WHERE id = ?`, lastScannedAt, id)
	return err
}

func (r *videoRepo) UpdateProbe(ctx context.Context, v domain.Video) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos SET title = ?, duration = ?, codec = ?, container = ?,
			width = ?, height = ?, updated_at = ?
		WHERE id = ?`,
		v.Title, v.Duration, nullString(v.Codec), nullString(v.Container),
		v.Width, v.Height, nowRFC3339(), v.ID)
	return err
}

func (r *videoRepo) UpdateCovers(ctx context.Context, id, coverPath, thumbPath string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos SET cover_path = ?, thumb_path = ?, updated_at = ? WHERE id = ?`,
		nullString(coverPath), nullString(thumbPath), nowRFC3339(), id)
	return err
}

func (r *videoRepo) ListByStorage(ctx context.Context, storageID string) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+videoCols+` FROM videos WHERE storage_id = ? ORDER BY relative_path`, storageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *videoRepo) MarkMissing(ctx context.Context, storageID, since string) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM videos WHERE storage_id = ? AND last_scanned_at < ?`, storageID, since)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM videos WHERE storage_id = ? AND last_scanned_at < ?`, storageID, since); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}
