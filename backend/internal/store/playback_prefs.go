package store

import (
	"context"
	"database/sql"
	"errors"

	"homereel/backend/internal/domain"
)

type playbackPrefsRepo struct {
	db *sql.DB
}

// NewPlaybackPrefsRepo returns a SQLite-backed domain.PlaybackPrefsRepo.
func NewPlaybackPrefsRepo(database *sql.DB) domain.PlaybackPrefsRepo {
	return &playbackPrefsRepo{db: database}
}

const playbackPrefsCols = "video_id, user, audio_track, subtitle_id, volume, muted, updated_at"

func (r *playbackPrefsRepo) Get(ctx context.Context, videoID, user string) (domain.PlaybackPrefs, error) {
	var p domain.PlaybackPrefs
	var audio sql.NullInt64
	var subtitle sql.NullString
	var volume sql.NullFloat64
	var muted sql.NullBool
	err := r.db.QueryRowContext(ctx,
		`SELECT `+playbackPrefsCols+` FROM playback_prefs WHERE video_id = ? AND user = ?`,
		videoID, user).Scan(&p.VideoID, &p.User, &audio, &subtitle, &volume, &muted, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlaybackPrefs{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PlaybackPrefs{}, err
	}
	p.AudioTrack = nullIntPtr(audio)
	p.SubtitleID = nullStrPtr(subtitle)
	p.Volume = nullFloatPtr(volume)
	p.Muted = nullBoolPtr(muted)
	return p, nil
}

func (r *playbackPrefsRepo) Patch(ctx context.Context, videoID, user string, patch domain.PlaybackPrefsPatch) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO playback_prefs (video_id, user, audio_track, subtitle_id, volume, muted, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(video_id, user) DO UPDATE SET
			audio_track = COALESCE(excluded.audio_track, playback_prefs.audio_track),
			subtitle_id = COALESCE(excluded.subtitle_id, playback_prefs.subtitle_id),
			volume      = COALESCE(excluded.volume, playback_prefs.volume),
			muted       = COALESCE(excluded.muted, playback_prefs.muted),
			updated_at  = excluded.updated_at`,
		videoID, user,
		intNull(patch.AudioTrack), strNull(patch.SubtitleID), floatNull(patch.Volume), boolNull(patch.Muted),
		domain.Now())
	return err
}

func (r *playbackPrefsRepo) Delete(ctx context.Context, videoID, user string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM playback_prefs WHERE video_id = ? AND user = ?`, videoID, user)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *playbackPrefsRepo) DeleteAll(ctx context.Context, user string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM playback_prefs WHERE user = ?`, user)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *playbackPrefsRepo) ListAll(ctx context.Context, user string) ([]domain.PlaybackPrefs, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+playbackPrefsCols+` FROM playback_prefs WHERE user = ? ORDER BY updated_at DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlaybackPrefs
	for rows.Next() {
		var p domain.PlaybackPrefs
		var audio sql.NullInt64
		var subtitle sql.NullString
		var volume sql.NullFloat64
		var muted sql.NullBool
		if err := rows.Scan(&p.VideoID, &p.User, &audio, &subtitle, &volume, &muted, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.AudioTrack = nullIntPtr(audio)
		p.SubtitleID = nullStrPtr(subtitle)
		p.Volume = nullFloatPtr(volume)
		p.Muted = nullBoolPtr(muted)
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- series-scoped prefs (ADR-006 player prefs 修订) ---

const seriesPlaybackPrefsCols = "series_id, user, audio_track_name, subtitle_name, volume, muted, updated_at"

func (r *playbackPrefsRepo) GetSeries(ctx context.Context, seriesID, user string) (domain.SeriesPlaybackPrefs, error) {
	var p domain.SeriesPlaybackPrefs
	var audio sql.NullString
	var subtitle sql.NullString
	var volume sql.NullFloat64
	var muted sql.NullBool
	err := r.db.QueryRowContext(ctx,
		`SELECT `+seriesPlaybackPrefsCols+` FROM series_playback_prefs WHERE series_id = ? AND user = ?`,
		seriesID, user).Scan(&p.SeriesID, &p.User, &audio, &subtitle, &volume, &muted, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SeriesPlaybackPrefs{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.SeriesPlaybackPrefs{}, err
	}
	p.AudioTrackName = nullStrPtr(audio)
	p.SubtitleName = nullStrPtr(subtitle)
	p.Volume = nullFloatPtr(volume)
	p.Muted = nullBoolPtr(muted)
	return p, nil
}

func (r *playbackPrefsRepo) PatchSeries(ctx context.Context, seriesID, user string, patch domain.SeriesPlaybackPrefsPatch) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO series_playback_prefs (series_id, user, audio_track_name, subtitle_name, volume, muted, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(series_id, user) DO UPDATE SET
			audio_track_name = COALESCE(excluded.audio_track_name, series_playback_prefs.audio_track_name),
			subtitle_name    = COALESCE(excluded.subtitle_name, series_playback_prefs.subtitle_name),
			volume           = COALESCE(excluded.volume, series_playback_prefs.volume),
			muted            = COALESCE(excluded.muted, series_playback_prefs.muted),
			updated_at       = excluded.updated_at`,
		seriesID, user,
		strNull(patch.AudioTrackName), strNull(patch.SubtitleName),
		floatNull(patch.Volume), boolNull(patch.Muted),
		domain.Now())
	return err
}

func (r *playbackPrefsRepo) DeleteSeries(ctx context.Context, seriesID, user string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM series_playback_prefs WHERE series_id = ? AND user = ?`, seriesID, user)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *playbackPrefsRepo) DeleteAllSeries(ctx context.Context, user string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM series_playback_prefs WHERE user = ?`, user)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *playbackPrefsRepo) ListAllSeries(ctx context.Context, user string) ([]domain.SeriesPlaybackPrefs, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+seriesPlaybackPrefsCols+` FROM series_playback_prefs WHERE user = ? ORDER BY updated_at DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SeriesPlaybackPrefs
	for rows.Next() {
		var p domain.SeriesPlaybackPrefs
		var audio sql.NullString
		var subtitle sql.NullString
		var volume sql.NullFloat64
		var muted sql.NullBool
		if err := rows.Scan(&p.SeriesID, &p.User, &audio, &subtitle, &volume, &muted, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.AudioTrackName = nullStrPtr(audio)
		p.SubtitleName = nullStrPtr(subtitle)
		p.Volume = nullFloatPtr(volume)
		p.Muted = nullBoolPtr(muted)
		out = append(out, p)
	}
	return out, rows.Err()
}

func intNull(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func strNull(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func floatNull(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolNull(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullStrPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullBoolPtr(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	return &v.Bool
}
