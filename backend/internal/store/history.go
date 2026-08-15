package store

import (
	"context"
	"database/sql"
	"errors"

	"homereel/backend/internal/domain"
)

type historyRepo struct {
	db *sql.DB
}

// NewHistoryRepo returns a SQLite-backed domain.HistoryRepo.
func NewHistoryRepo(database *sql.DB) domain.HistoryRepo {
	return &historyRepo{db: database}
}

func (r *historyRepo) Get(ctx context.Context, videoID, user string) (domain.History, error) {
	var h domain.History
	err := r.db.QueryRowContext(ctx,
		`SELECT video_id, user, progress, updated_at FROM history WHERE video_id = ? AND user = ?`,
		videoID, user).Scan(&h.VideoID, &h.User, &h.Progress, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.History{}, domain.ErrNotFound
	}
	return h, err
}

func (r *historyRepo) Upsert(ctx context.Context, h domain.History) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO history (video_id, user, progress, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(video_id, user) DO UPDATE SET
			progress = excluded.progress,
			updated_at = excluded.updated_at`,
		h.VideoID, h.User, h.Progress, h.UpdatedAt)
	return err
}

// Delete removes a video's resume position (清空播放历史).
func (r *historyRepo) Delete(ctx context.Context, videoID, user string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM history WHERE video_id = ? AND user = ?`,
		videoID, user)
	return err
}

// DeleteBySeries clears the resume position of every member of a series. A
// series' members are the episodes bound to its show+season (root folder), so
// this deletes exactly the rows the series progress card aggregated.
func (r *historyRepo) DeleteBySeries(ctx context.Context, seriesID, user string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM history
		WHERE user = ? AND video_id IN (
			SELECT v.id FROM videos v
			JOIN seasons se ON se.id = ?
			WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode'
		)`, user, seriesID)
	return err
}
