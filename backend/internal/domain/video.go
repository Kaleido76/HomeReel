package domain

import "context"

// Video is an indexed media file (ADR-007: identity is
// storage_id + file_id + relative_path, fingerprint is size + mtime).
type Video struct {
	ID             string  `json:"id"`
	StorageID      string  `json:"storage_id"`
	FileID         string  `json:"file_id"`
	RelativePath   string  `json:"relative_path"`
	Path           string  `json:"path"`
	Size           int64   `json:"size"`
	MTime          int64   `json:"mtime"` // unix milliseconds
	Title          string  `json:"title"`
	Kind           string  `json:"kind"` // movie | episode
	Description    string  `json:"description"`
	Duration       float64 `json:"duration"`
	Codec          string  `json:"codec"`
	AudioCodec     string  `json:"audio_codec,omitempty"`
	Container      string  `json:"container"`
	Segmented      bool    `json:"segmented,omitempty"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            float64 `json:"fps,omitempty"`
	FileSize       int64   `json:"file_size,omitempty"`
	CoverPath      string  `json:"cover_path"`
	ThumbPath      string  `json:"thumb_path"`
	BackdropPath   string  `json:"backdrop_path,omitempty"`
	ShowID         string  `json:"show_id,omitempty"`
	SeasonNumber   int     `json:"season_number,omitempty"`
	EpisodeNumber  int     `json:"episode_number,omitempty"`
	EpisodeTitle   string  `json:"episode_title,omitempty"`
	Year           int     `json:"year,omitempty"`
	Rating         float64 `json:"rating,omitempty"`
	Genre          string  `json:"genre,omitempty"`
	Overview       string  `json:"overview,omitempty"`
	Studio         string  `json:"studio,omitempty"`
	CastText       string  `json:"cast_text,omitempty"`
	MetadataSource string  `json:"metadata_source"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	LastScannedAt  string  `json:"last_scanned_at"`
}

// VideoPatch carries editable metadata fields for PATCH /api/videos/:id.
// Nil pointers leave the corresponding column untouched.
type VideoPatch struct {
	Title          *string
	Description    *string
	Kind           *string
	Year           *int
	Rating         *float64
	Genre          *string
	Overview       *string
	Studio         *string
	CastText       *string
	ShowID         *string // *"" clears the show linkage
	SeasonNumber   *int
	EpisodeNumber  *int
	EpisodeTitle   *string
	BackdropPath   *string
	MetadataSource *string
}

// VideoQuery filters and paginates the video library.
type VideoQuery struct {
	Q         string // matches title or relative path
	Kind      string // movie | episode
	Tag       string
	ShowID    string
	Ungrouped bool   // standalone videos only (not part of a series)
	Sort      string // title | date | duration | name | rating
	Order     string // asc | desc
	Page      int    // 1-based
	PageSize  int
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
	// Delete removes a video record (FK cascades clear tags/history
	// and the FTS/empty-show triggers run).
	Delete(ctx context.Context, id string) error
	// UpdateProbe stores ffprobe-derived metadata.
	UpdateProbe(ctx context.Context, v Video) error
	// UpdateCovers stores the generated cover/thumb paths.
	UpdateCovers(ctx context.Context, id, coverPath, thumbPath string) error
	// UpdateMetadata applies editable metadata fields and rebuilds search_text.
	UpdateMetadata(ctx context.Context, id string, patch VideoPatch) error
	// AssignEpisode groups v under a show/season/episode and rebuilds search_text.
	AssignEpisode(ctx context.Context, id, showID string, seasonNumber, episodeNumber int, episodeTitle string) error
	// AssignMovie marks v as a standalone movie (clears show linkage).
	AssignMovie(ctx context.Context, id string) error
	// SetTags replaces the tag set for a video and rebuilds search_text.
	SetTags(ctx context.Context, id string, tags []string) error
	// Tags returns the tags of a video.
	Tags(ctx context.Context, id string) ([]string, error)
	// AllTags returns every tag with its usage count (for filters).
	AllTags(ctx context.Context) ([]TagCount, error)
	// ListByStorage returns all videos of one storage.
	ListByStorage(ctx context.Context, storageID string) ([]Video, error)
	// ListSegmented returns all videos flagged as segmented (hls.js-assembled
	// MP4), used by the remux management API.
	ListSegmented(ctx context.Context) ([]Video, error)
	// ContinueWatching returns videos with in-progress playback, most recently
	// active first (used by the home rows).
	ContinueWatching(ctx context.Context, limit int) ([]Video, error)
	// MarkMissing deletes videos in storage whose last_scanned_at is older
	// than since, returning their IDs (for deletion events).
	MarkMissing(ctx context.Context, storageID, since string) ([]string, error)
}

// TagCount is a tag and how many videos carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
