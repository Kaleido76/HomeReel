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
	Rating         float64 `json:"rating,omitempty"`
	PosterPath     string  `json:"poster_path,omitempty"`
	BackdropPath   string  `json:"backdrop_path,omitempty"`
	MetadataSource string  `json:"metadata_source"`
	// SortManual=1 表示成员顺序由用户手动维护（拖拽重排，ADR-015 修订）：扫描/
	// 同步只追加新成员到末尾，绝不按文件名序重排既有成员。
	SortManual    bool    `json:"sort_manual"`
	MemberCount   int     `json:"member_count"`
	LinkCount     int     `json:"link_count"`
	TotalDuration float64 `json:"total_duration"` // 成员时长合计（秒），供封面时长徽标
}

// SeriesMember is one video inside a series, with playback progress. The probe
// fields (codec/container/audio_codec/segmented) feed the frontend's runtime
// canPlayType check for the play button; DirectPlayable is the backend's
// conservative fallback, RemuxPlayable gates the container-only MP4 remux and
// TranscodePlayable gates the on-demand HLS transcode (ADR-006 修订).
type SeriesMember struct {
	VideoID          string  `json:"video_id"`
	Title            string  `json:"title"`
	TitleSource      string  `json:"title_source,omitempty"`
	EpisodeNumber    int     `json:"episode_number"`
	EpisodeTitle     string  `json:"episode_title,omitempty"`
	Duration         float64 `json:"duration"`
	ThumbPath        string  `json:"thumb_path,omitempty"`
	RelativePath     string  `json:"relative_path"`
	Progress         float64 `json:"progress"`
	Codec            string  `json:"codec,omitempty"`
	AudioCodec       string  `json:"audio_codec,omitempty"`
	Container        string  `json:"container,omitempty"`
	Segmented        bool    `json:"segmented,omitempty"`
	DirectPlayable   bool    `json:"direct_playable"`
	RemuxPlayable    bool    `json:"remux_playable"`
	TranscodePlayable bool   `json:"transcode_playable"`
}

// SeriesLink is a weak, unnamed relation to another series. In the group model
// (2026-09, 方案 B) a series' links are exactly the other members of its link
// group: all series in one group see each other mutually (A linking B,C makes
// B see A and C too).
type SeriesLink struct {
	SeriesID    string `json:"series_id"`
	LinkedID    string `json:"linked_id"`
	LinkedTitle string `json:"linked_title"`
	LinkedName  string `json:"linked_name"`
	SortIndex   int    `json:"sort_index"`
}

// SeriesQuery filters the series list. Q matches the series display name only.
type SeriesQuery struct {
	Q     string   // matches the series display name
	Tags  []string // every tag present on at least one member video
}

// SeriesRepo persists series (a show + a season bound to a root path) and their
// link groups.
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
	// SetMemberOrder persists a manual member order (1..N in the given video
	// id order) and flags seasons.sort_manual so scans keep it.
	SetMemberOrder(ctx context.Context, seriesID string, videoIDs []string) error
	// SetSortManual flags/unflags seasons.sort_manual. Clearing it lets scans
	// restore automatic file-name ordering (「按文件名字典序重新刷新排序」).
	SetSortManual(ctx context.Context, seriesID string, manual bool) error
	GetMembers(ctx context.Context, id string) ([]SeriesMember, error)
	GetLinks(ctx context.Context, id string) ([]SeriesLink, error)
	// SetLinks replaces the series' link group membership: the series and every
	// linked series end up in ONE group (mutual visibility, 方案 B). Any series
	// already in another group merges that group in; unchecked series leave the
	// group. The whole membership is replaced — the caller sends the full desired set.
	SetLinks(ctx context.Context, seriesID string, linkedIDs []string) error
	// RemoveLink removes one series from the series' link group (a link chip's
	// × button); if the group drops to one member it is removed entirely.
	RemoveLink(ctx context.Context, seriesID, linkedID string) error
	// SyncShowLinks merges all seasons of a show into one link group (idempotent),
	// so the parts of a series display together.
	SyncShowLinks(ctx context.Context, showID string) error
}
