package domain

import "context"

// Series is one organizational unit of the library: a season of a show or one
// part of a movie franchise. Members (videos) are ordered by position, which
// may have gaps (missing episodes are normal; ordering never relies on DB IDs).
type Series struct {
	ID             string  `json:"id"`
	ShowID         string  `json:"show_id"`
	Title          string  `json:"title"` // 标题（show.name）
	Name           string  `json:"name"`  // 显示名：标题 + 季/部
	Kind           string  `json:"kind"`  // tv=剧集季 | movie=电影部
	SeasonNumber   int     `json:"season_number"`
	Overview       string  `json:"overview,omitempty"`
	Year           int     `json:"year,omitempty"`
	Rating         float64 `json:"rating,omitempty"`
	Genre          string  `json:"genre,omitempty"`
	PosterPath     string  `json:"poster_path,omitempty"`
	BackdropPath   string  `json:"backdrop_path,omitempty"`
	MetadataSource string  `json:"metadata_source"`
	MemberCount    int     `json:"member_count"`
	LinkCount      int     `json:"link_count"`
	TotalDuration  float64 `json:"total_duration"` // 成员时长合计（秒），供封面时长徽标
}

// SeriesMember is one video inside a series, with playback progress.
type SeriesMember struct {
	VideoID          string  `json:"video_id"`
	Title            string  `json:"title"`
	EpisodeNumber    int     `json:"episode_number"`
	EpisodeTitle     string  `json:"episode_title,omitempty"`
	Duration         float64 `json:"duration"`
	ThumbPath        string  `json:"thumb_path,omitempty"`
	RelativePath     string  `json:"relative_path"`
	StorageID        string  `json:"storage_id"`
	StorageAvailable bool    `json:"storage_available"`
	Progress         float64 `json:"progress"`
}

// SeriesLink is a weak, unnamed, ordered relation between two series.
type SeriesLink struct {
	SeriesID    string `json:"series_id"`
	LinkedID    string `json:"linked_id"`
	LinkedTitle string `json:"linked_title"`
	LinkedName  string `json:"linked_name"`
	SortIndex   int    `json:"sort_index"`
}

// SeriesQuery filters the series list. Q matches the show name or overview.
type SeriesQuery struct {
	Q     string   // matches show name or overview
	Genre string   // matches genre (substring)
	Year  int      // matches year exactly
	Tags  []string // every tag present on at least one member video
}

// SeriesRepo persists series (seasons) and their weak links.
type SeriesRepo interface {
	List(ctx context.Context, q SeriesQuery) ([]Series, error)
	// Get returns the series identified by its season id.
	Get(ctx context.Context, id string) (Series, error)
	// FindID resolves the series (season) id for a show/season pair.
	FindID(ctx context.Context, showID string, seasonNumber int) (string, error)
	GetMembers(ctx context.Context, id string) ([]SeriesMember, error)
	GetLinks(ctx context.Context, id string) ([]SeriesLink, error)
	// AddLink creates a weak relation (directed; lookups are symmetric).
	AddLink(ctx context.Context, seriesID, linkedID string, sortIndex int) error
	// RemoveLink deletes a weak relation in either direction.
	RemoveLink(ctx context.Context, seriesID, linkedID string) error
	// SyncShowLinks creates ordered weak links between consecutive seasons of
	// a show (idempotent), so the parts of a series display together.
	SyncShowLinks(ctx context.Context, showID string) error
}
