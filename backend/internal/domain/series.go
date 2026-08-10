package domain

import "context"

// Series is one organizational unit of the library: a user-created container
// bound to a root directory (RootPath). Its members are exactly the videos
// living as direct children of that folder, ordered by file name 1..N. A series
// has only a name (the folder name) — there is no season numbering, and it is
// purely a database concept (it never changes unless the user syncs it).
type Series struct {
	ID             string  `json:"id"`
	ShowID         string  `json:"show_id"`
	RootPath       string  `json:"root_path,omitempty"` // 系列根目录（成员必须是其直接一级子文件）
	Title          string  `json:"title"`               // 标题（show.name）
	Name           string  `json:"name"`                // 显示名 = 标题（无季/部）
	Kind           string  `json:"kind"`                // 恒为 "tv"（历史列，无电影/tv 结构类型；区分走标签）
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

// SeriesMember is one video inside a series, with playback progress. The probe
// fields (codec/container/audio_codec/segmented) feed the frontend's runtime
// canPlayType check for the play button; DirectPlayable is the backend's
// conservative fallback.
type SeriesMember struct {
	VideoID       string  `json:"video_id"`
	Title         string  `json:"title"`
	EpisodeNumber int     `json:"episode_number"`
	EpisodeTitle  string  `json:"episode_title,omitempty"`
	Duration      float64 `json:"duration"`
	ThumbPath     string  `json:"thumb_path,omitempty"`
	RelativePath  string  `json:"relative_path"`
	Progress      float64 `json:"progress"`
	Codec         string  `json:"codec,omitempty"`
	AudioCodec    string  `json:"audio_codec,omitempty"`
	Container     string  `json:"container,omitempty"`
	Segmented     bool    `json:"segmented,omitempty"`
	DirectPlayable bool   `json:"direct_playable"`
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

// SeriesRepo persists series (a show + a season bound to a root path) and their
// weak links.
type SeriesRepo interface {
	List(ctx context.Context, q SeriesQuery) ([]Series, error)
	// Get returns the series identified by its season id.
	Get(ctx context.Context, id string) (Series, error)
	// FindID resolves the series (season) id for a show/season pair.
	FindID(ctx context.Context, showID string, seasonNumber int) (string, error)
	// FindByRoot resolves the series bound to a root directory (ErrNotFound
	// when none). Series identity is the root path.
	FindByRoot(ctx context.Context, rootPath string) (Series, error)
	// CreateAtRoot creates the show+season bound to rootPath (name = folder
	// name, season number 1) and returns the series. Idempotent per root path.
	CreateAtRoot(ctx context.Context, name, rootPath string) (Series, error)
	// BindMembers assigns the given videos to the series in one transaction:
	// each member is set kind='episode' with its position and file-derived
	// title, and is detached from any other series it currently belongs to.
	BindMembers(ctx context.Context, seriesID string, members []EpisodeAssign) error
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
