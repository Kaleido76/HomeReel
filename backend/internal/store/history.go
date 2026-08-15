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
