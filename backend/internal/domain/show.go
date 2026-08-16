package domain

import "context"

// Show is a TV series grouping episodes (ADR-015).
type Show struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Overview       string  `json:"overview"`
	Rating         float64 `json:"rating,omitempty"`
	PosterPath     string  `json:"poster_path,omitempty"`
	BackdropPath   string  `json:"backdrop_path,omitempty"`
	MetadataSource string  `json:"metadata_source"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`

	// Derived counts for the poster wall.
	SeasonCount    int `json:"season_count"`
	EpisodeCount   int `json:"episode_count"`
	UnwatchedCount int `json:"unwatched_count"`
}

// Season is one series of a show. A series = a season bound to a root path;
// number is always 1 for the folder-as-series model, root_path is its identity.
type Season struct {
	ID           string `json:"id"`
	ShowID       string `json:"show_id"`
	Number       int    `json:"number"`
	Name         string `json:"name"`
	RootPath     string `json:"root_path,omitempty"` // 系列根目录
	Overview     string `json:"overview,omitempty"`
	PosterPath   string `json:"poster_path,omitempty"`
	EpisodeCount int    `json:"episode_count"` // derived
}

// Episode is one episode inside a season, with playback progress joined from
// history so the UI can show per-episode resume state.
type Episode struct {
	VideoID       string  `json:"video_id"`
	ShowID        string  `json:"show_id"`
	SeasonNumber  int     `json:"season_number"`
	EpisodeNumber int     `json:"episode_number"`
	Title         string  `json:"title"`
	EpisodeTitle  string  `json:"episode_title,omitempty"`
	RelativePath  string  `json:"relative_path"`
	Duration      float64 `json:"duration"`
	ThumbPath     string  `json:"thumb_path,omitempty"`
	Progress      float64 `json:"progress"`
}

// ShowRepo persists shows, seasons and their episodes.
type ShowRepo interface {
	List(ctx context.Context) ([]Show, error)
	Get(ctx context.Context, id string) (Show, error)
	GetSeasons(ctx context.Context, showID string) ([]Season, error)
	// GetEpisodes returns the episodes of a show, optionally restricted to one
	// season, ordered by season then episode number.
	GetEpisodes(ctx context.Context, showID string, seasonNumber int) ([]Episode, error)
	// FindByName resolves a show by name (case-insensitive) for grouping.
	FindByName(ctx context.Context, name string) (Show, error)
	Create(ctx context.Context, s Show) error
	// EnsureSeason returns the season, creating it if missing.
	EnsureSeason(ctx context.Context, showID string, number int) (Season, error)
	// UpdateMetadata applies editable show metadata.
	UpdateMetadata(ctx context.Context, s Show) error
	// RemoveEmptyShow deletes a show that no longer has episodes.
	RemoveEmptyShow(ctx context.Context, id string) error
}
