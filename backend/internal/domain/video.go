package domain

import "context"

// Video is an indexed media file (ADR-007: identity is
// storage_id + file_id + relative_path, fingerprint is size + mtime).
type Video struct {
	ID            string  `json:"id"`
	StorageID     string  `json:"storage_id"`
	FileID        string  `json:"file_id"`
	RelativePath  string  `json:"relative_path"`
	Path          string  `json:"path"`
	Size          int64   `json:"size"`
	MTime         int64   `json:"mtime"` // unix milliseconds
	Title         string  `json:"title"`
	Duration      float64 `json:"duration"`
	Codec         string  `json:"codec"`
	Container     string  `json:"container"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	CoverPath     string  `json:"cover_path"`
	ThumbPath     string  `json:"thumb_path"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	LastScannedAt string  `json:"last_scanned_at"`
}

// VideoQuery filters and paginates the video library.
type VideoQuery struct {
	Q        string // matches title or relative path
	Sort     string // title | date | duration | name
	Order    string // asc | desc
	Page     int    // 1-based
	PageSize int
}

// VideoPage is one page of library results with the total match count.
type VideoPage struct {
	Videos []Video
	Total  int
}

// VideoRepo persists video records.
type VideoRepo interface {
	Get(ctx context.Context, id string) (Video, error)
	// List returns one page of library videos matching q, newest first by
	// default (used by GET /api/videos).
	List(ctx context.Context, q VideoQuery) (VideoPage, error)
	Create(ctx context.Context, v Video) error
	// UpdateFingerprint refreshes the on-disk location/size/mtime of a video
	// (used for moves and in-place changes) and marks it as scanned.
	UpdateFingerprint(ctx context.Context, id, path, relativePath string, size, mtime int64, lastScannedAt string) error
	// Touch refreshes last_scanned_at for an unchanged video.
	Touch(ctx context.Context, id string, lastScannedAt string) error
	// UpdateProbe stores ffprobe-derived metadata.
	UpdateProbe(ctx context.Context, v Video) error
	// UpdateCovers stores the generated cover/thumb paths.
	UpdateCovers(ctx context.Context, id, coverPath, thumbPath string) error
	// ListByStorage returns all videos of one storage.
	ListByStorage(ctx context.Context, storageID string) ([]Video, error)
	// MarkMissing deletes videos in storage whose last_scanned_at is older
	// than since, returning their IDs (for deletion events).
	MarkMissing(ctx context.Context, storageID, since string) ([]string, error)
}
