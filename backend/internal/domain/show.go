package domain

import "context"

// Show is a TV series grouping episodes (ADR-015).
type Show struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Overview       string  `json:"overview"`
	Year           int     `json:"year,omitempty"`
	Rating         float64 `json:"rating,omitempty"`
	Genre          string  `json:"genre,omitempty"`
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

// Season is one numbered season of a show.
type Season struct {
	ID           string `json:"id"`
	ShowID       string `json:"show_id"`
	Number       int    `json:"number"`
	Name         string `json:"name"`
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
	// AssignSeason groups a set of already-existing videos under one
	// show/season atomically: it creates the show and season when missing and
	// assigns every member in a single transaction, so a series never appears
	// in the library half-formed. It returns the show id.
	AssignSeason(ctx context.Context, showName string, seasonNumber int, members []EpisodeAssign) (string, error)
	// UpdateMetadata applies editable show metadata.
	UpdateMetadata(ctx context.Context, s Show) error
	// RemoveEmptyShow deletes a show that no longer has episodes.
	RemoveEmptyShow(ctx context.Context, id string) error
}
