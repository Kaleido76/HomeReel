package domain

import "context"

// History records one viewer's resume position per video. The single user is
// fixed to "local" (fields kept for future multi-user). Cross-terminal
// shared: the last writer wins (ADR-002).
type History struct {
	VideoID   string  `json:"video_id"`
	User      string  `json:"user"`
	Progress  float64 `json:"progress"`
	UpdatedAt string  `json:"updated_at"`
}

// HistoryRepo persists resume positions.
type HistoryRepo interface {
	Get(ctx context.Context, videoID, user string) (History, error)
	Upsert(ctx context.Context, h History) error
}
