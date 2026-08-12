package domain

import "context"

// Video is an indexed media file (ADR-007: identity is source_id + file_id +
// relative_path, fingerprint is size + mtime). file_id is matched globally so a
// file moved between sources keeps its identity.
type Video struct {
	ID             string  `json:"id"`
	SourceID       string  `json:"source_id,omitempty"`
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
	FastStart      bool    `json:"faststart"`
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
	TitleSource    string  `json:"title_source"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	LastScannedAt  string  `json:"last_scanned_at"`
}

// VideoPatch carries editable metadata fields for PATCH /api/videos/:id.
// Nil pointers leave the corresponding column untouched. Structure fields
// (show/season/episode linkage) are not editable here — membership and order
// follow the on-disk layout and are maintained by scans and manual series sync.
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
	BackdropPath   *string
	MetadataSource *string
}

// VideoQuery filters and paginates the video library.
type VideoQuery struct {
	Q         string   // matches title or relative path
	Desc      string   // matches description
	Genre     string   // matches genre (substring)
	Year      int      // matches year exactly
	Kind      string   // movie | episode
	Tags      []string // all tags must be present (AND)
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

// TitleSource tells where a video's title came from: "file" (derived from the
// file name, refreshed on scan/probe) or "manual" (user-edited, never
// overwritten by scans). Series members always follow the file name and are
// reset to "file" by BindMembers (ADR-017).
const (
	TitleSourceFile   = "file"
	TitleSourceManual = "manual"
)

// EpisodeAssign is one member of a show/season grouping applied atomically in
// a single transaction by ShowRepo.AssignSeason.
type EpisodeAssign struct {
	VideoID       string
	EpisodeNumber int
	Title         string // episode_title fallback (file base name)
}

// VideoRepo persists video records.
type VideoRepo interface {
	Get(ctx context.Context, id string) (Video, error)
	// List returns one page of library videos matching q, newest first by
	// default (used by GET /api/videos).
	List(ctx context.Context, q VideoQuery) (VideoPage, error)
	Create(ctx context.Context, v Video) error
	// UpdateFingerprint refreshes the on-disk location/size/mtime of a video
	// (used for moves between sources and in-place changes) and marks it as
	// scanned.
	UpdateFingerprint(ctx context.Context, id, sourceID, path, relativePath string, size, mtime int64, lastScannedAt string) error
	// Touch refreshes last_scanned_at for an unchanged video.
	Touch(ctx context.Context, id string, lastScannedAt string) error
	// Delete removes a video record (FK cascades clear tags/history
	// and the FTS/empty-show triggers run).
	Delete(ctx context.Context, id string) error
	// DeleteBySource removes every video owned by a media source and returns the
	// deleted ids (for deletion events). Used when a source marker is removed:
	// its whole library disappears with it.
	DeleteBySource(ctx context.Context, sourceID string) ([]string, error)
	// UpdateProbe stores ffprobe-derived metadata.
	UpdateProbe(ctx context.Context, v Video) error
	// UpdateCovers stores the generated cover/thumb paths.
	UpdateCovers(ctx context.Context, id, coverPath, thumbPath string) error
	// UpdateMetadata applies editable metadata fields and rebuilds search_text.
	UpdateMetadata(ctx context.Context, id string, patch VideoPatch) error
	// AssignStandalone detaches every given video from its series in a single
	// statement (kind=movie, series linkage cleared), used when a video no
	// longer lives inside any series folder.
	AssignStandalone(ctx context.Context, ids []string) error
	// SetTags replaces the tag set for a video and rebuilds search_text.
	SetTags(ctx context.Context, id string, tags []string) error
	// Tags returns the tags of a video.
	Tags(ctx context.Context, id string) ([]string, error)
	// AllTags returns every tag with its usage count (for filters).
	AllTags(ctx context.Context) ([]TagCount, error)
	// ListAll returns every indexed video (used by the source scanner for
	// global file_id move detection).
	ListAll(ctx context.Context) ([]Video, error)
	// ListBySource returns all videos of one media source.
	ListBySource(ctx context.Context, sourceID string) ([]Video, error)
	// ContinueWatching returns videos with in-progress playback, most recently
	// active first (used by the home rows).
	ContinueWatching(ctx context.Context, limit int) ([]Video, error)
	// MarkMissingBySource deletes videos owned by source whose last_scanned_at
	// is older than since, except those whose absolute path falls under any of
	// excludeRoots (child sources claim those). It returns the deleted IDs (for
	// deletion events).
	MarkMissingBySource(ctx context.Context, sourceID, since string, excludeRoots []string) ([]string, error)
}

// TagCount is a tag and how many videos carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
