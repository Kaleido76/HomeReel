package domain

import "context"

// PlaybackPrefs is a per-video cache of the playback selections a user made:
// the chosen audio track, the chosen subtitle source and the volume. The player
// auto-applies them on every play and only refreshes them when the user manually
// changes a selection. Unlike History (user data), this is a rebuildable cache:
// deleting a row resets the video to the browser defaults and it is cleared from
// the cache manager page.
//
// AudioTrack is the 0-based audio ordinal (matches ?audio=N / -map 0:a:<N>);
// nil means "not chosen" and falls back to the container default. SubtitleID
// identifies a subtitle source: "sidecar" for the sidecar file, "e<N>" for the
// embedded text track with stream index N, and "" for "explicitly turned off".
// Volume is 0..1 and muted is recorded separately.
type PlaybackPrefs struct {
	VideoID    string   `json:"video_id"`
	User       string   `json:"-"`
	AudioTrack *int     `json:"audio_track,omitempty"`
	SubtitleID *string  `json:"subtitle_id,omitempty"`
	Volume     *float64 `json:"volume,omitempty"`
	Muted      *bool    `json:"muted,omitempty"`
	UpdatedAt  string   `json:"updated_at"`
}

// PlaybackPrefsPatch is a partial update: only the provided fields are written,
// the rest of an existing row (or a fresh row's defaults) stays untouched.
type PlaybackPrefsPatch struct {
	AudioTrack *int
	SubtitleID *string
	Volume     *float64
	Muted      *bool
}

// SeriesPlaybackPrefs is the series-scoped playback selection cache (ADR-006
// player prefs 修订): every episode of a series shares one remembered audio
// track, subtitle track and volume. AudioTrackName/SubtitleName store the track
// LABEL (e.g. "简体中文") because the same choice must resolve to a different
// concrete track inside each episode — the player matches the name against the
// current episode's track list. A series row wins over every member's own video
// row (SeriesPlaybackPrefs resolution), so a video row acts only as the fallback
// when the series has no record (e.g. a previously standalone episode).
type SeriesPlaybackPrefs struct {
	SeriesID       string   `json:"series_id"`
	User           string   `json:"-"`
	AudioTrackName *string  `json:"audio_track_name,omitempty"`
	SubtitleName   *string  `json:"subtitle_name,omitempty"`
	Volume         *float64 `json:"volume,omitempty"`
	Muted          *bool    `json:"muted,omitempty"`
	UpdatedAt      string   `json:"updated_at"`
}

// SeriesPlaybackPrefsPatch is a partial update for a series prefs row.
type SeriesPlaybackPrefsPatch struct {
	AudioTrackName *string
	SubtitleName   *string
	Volume         *float64
	Muted          *bool
}

// PlaybackPrefsRepo persists playback selection caches (ADR-006 player prefs),
// both the per-video rows and the series-scoped rows shared by every episode.
type PlaybackPrefsRepo interface {
	Get(ctx context.Context, videoID, user string) (PlaybackPrefs, error)
	Patch(ctx context.Context, videoID, user string, patch PlaybackPrefsPatch) error
	// Delete removes one video's pref row and reports how many rows were removed.
	Delete(ctx context.Context, videoID, user string) (int64, error)
	// DeleteAll removes every pref row of the user and reports how many were removed.
	DeleteAll(ctx context.Context, user string) (int64, error)
	// ListAll returns every pref row of the user (cache manager listing).
	ListAll(ctx context.Context, user string) ([]PlaybackPrefs, error)

	GetSeries(ctx context.Context, seriesID, user string) (SeriesPlaybackPrefs, error)
	PatchSeries(ctx context.Context, seriesID, user string, patch SeriesPlaybackPrefsPatch) error
	// DeleteSeries removes one series' pref row and reports how many were removed.
	DeleteSeries(ctx context.Context, seriesID, user string) (int64, error)
	// DeleteAllSeries removes every series pref row of the user.
	DeleteAllSeries(ctx context.Context, user string) (int64, error)
	// ListAllSeries returns every series pref row of the user (cache manager listing).
	ListAllSeries(ctx context.Context, user string) ([]SeriesPlaybackPrefs, error)
}
