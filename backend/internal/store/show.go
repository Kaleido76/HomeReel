package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/oklog/ulid/v2"

	"homereel/backend/internal/domain"
)

type showRepo struct {
	db *sql.DB
}

// NewShowRepo returns a SQLite-backed domain.ShowRepo.
func NewShowRepo(database *sql.DB) domain.ShowRepo {
	return &showRepo{db: database}
}

const showCols = `id, name, overview, year, rating, genre, poster_path, backdrop_path,
	metadata_source, created_at, updated_at`

func (r *showRepo) List(ctx context.Context) ([]domain.Show, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+showCols+`,
			(SELECT COUNT(*) FROM seasons se WHERE se.show_id = s.id) AS season_count,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = s.id AND v.kind = 'episode') AS episode_count,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = s.id AND v.kind = 'episode'
				AND NOT EXISTS (SELECT 1 FROM history h WHERE h.video_id = v.id AND h.progress > 0)) AS unwatched_count
		FROM shows s ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Show{}
	for rows.Next() {
		var (
			sh             domain.Show
			overview       sql.NullString
			year           sql.NullInt64
			rating         sql.NullFloat64
			genre          sql.NullString
			poster         sql.NullString
			backdrop       sql.NullString
			seasonCount    int
			episodeCount   int
			unwatchedCount int
		)
		if err := rows.Scan(&sh.ID, &sh.Name, &overview, &year, &rating, &genre,
			&poster, &backdrop, &sh.MetadataSource, &sh.CreatedAt, &sh.UpdatedAt,
			&seasonCount, &episodeCount, &unwatchedCount); err != nil {
			return nil, err
		}
		if overview.Valid {
			sh.Overview = overview.String
		}
		if year.Valid {
			sh.Year = int(year.Int64)
		}
		if rating.Valid {
			sh.Rating = rating.Float64
		}
		if genre.Valid {
			sh.Genre = genre.String
		}
		if poster.Valid {
			sh.PosterPath = poster.String
		}
		if backdrop.Valid {
			sh.BackdropPath = backdrop.String
		}
		sh.SeasonCount = seasonCount
		sh.EpisodeCount = episodeCount
		sh.UnwatchedCount = unwatchedCount
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (r *showRepo) Get(ctx context.Context, id string) (domain.Show, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+showCols+`,
			(SELECT COUNT(*) FROM seasons se WHERE se.show_id = s.id) AS season_count,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = s.id AND v.kind = 'episode') AS episode_count,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = s.id AND v.kind = 'episode'
				AND NOT EXISTS (SELECT 1 FROM history h WHERE h.video_id = v.id AND h.progress > 0)) AS unwatched_count
		FROM shows s WHERE s.id = ?`, id)
	sh, err := scanShow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Show{}, domain.ErrNotFound
	}
	return sh, err
}

func scanShow(row scanner) (domain.Show, error) {
	var (
		sh             domain.Show
		overview       sql.NullString
		year           sql.NullInt64
		rating         sql.NullFloat64
		genre          sql.NullString
		poster         sql.NullString
		backdrop       sql.NullString
		seasonCount    int
		episodeCount   int
		unwatchedCount int
	)
	if err := row.Scan(&sh.ID, &sh.Name, &overview, &year, &rating, &genre,
		&poster, &backdrop, &sh.MetadataSource, &sh.CreatedAt, &sh.UpdatedAt,
		&seasonCount, &episodeCount, &unwatchedCount); err != nil {
		return domain.Show{}, err
	}
	if overview.Valid {
		sh.Overview = overview.String
	}
	if year.Valid {
		sh.Year = int(year.Int64)
	}
	if rating.Valid {
		sh.Rating = rating.Float64
	}
	if genre.Valid {
		sh.Genre = genre.String
	}
	if poster.Valid {
		sh.PosterPath = poster.String
	}
	if backdrop.Valid {
		sh.BackdropPath = backdrop.String
	}
	sh.SeasonCount = seasonCount
	sh.EpisodeCount = episodeCount
	sh.UnwatchedCount = unwatchedCount
	return sh, nil
}

func (r *showRepo) GetSeasons(ctx context.Context, showID string) ([]domain.Season, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT se.id, se.show_id, se.number, se.name, se.overview, se.poster_path,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode') AS episode_count
		FROM seasons se WHERE se.show_id = ? ORDER BY se.number`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Season{}
	for rows.Next() {
		var (
			se       domain.Season
			name     sql.NullString
			overview sql.NullString
			poster   sql.NullString
			count    int
		)
		if err := rows.Scan(&se.ID, &se.ShowID, &se.Number, &name, &overview, &poster, &count); err != nil {
			return nil, err
		}
		if name.Valid {
			se.Name = name.String
		}
		if overview.Valid {
			se.Overview = overview.String
		}
		if poster.Valid {
			se.PosterPath = poster.String
		}
		se.EpisodeCount = count
		out = append(out, se)
	}
	return out, rows.Err()
}

func (r *showRepo) GetEpisodes(ctx context.Context, showID string, seasonNumber int) ([]domain.Episode, error) {
	query := `
		SELECT v.id, v.show_id, v.season_number, v.episode_number, v.title, v.episode_title,
			v.relative_path, v.duration, v.thumb_path, v.storage_id,
			COALESCE(st.available, 0) AS available,
			COALESCE(h.progress, 0) AS progress
		FROM videos v
		LEFT JOIN storages st ON st.id = v.storage_id
		LEFT JOIN history h ON h.video_id = v.id AND h.user = 'local'
		WHERE v.show_id = ? AND v.kind = 'episode'`
	args := []any{showID}
	if seasonNumber > 0 {
		query += ` AND v.season_number = ?`
		args = append(args, seasonNumber)
	}
	query += ` ORDER BY v.season_number, v.episode_number`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Episode{}
	for rows.Next() {
		var (
			ep       domain.Episode
			title    sql.NullString
			thumb    sql.NullString
			duration sql.NullFloat64
			avail    bool
		)
		if err := rows.Scan(&ep.VideoID, &ep.ShowID, &ep.SeasonNumber, &ep.EpisodeNumber,
			&title, &ep.EpisodeTitle, &ep.RelativePath, &duration, &thumb, &ep.StorageID,
			&avail, &ep.Progress); err != nil {
			return nil, err
		}
		if title.Valid {
			ep.Title = title.String
		}
		if duration.Valid {
			ep.Duration = duration.Float64
		}
		if thumb.Valid {
			ep.ThumbPath = thumb.String
		}
		ep.StorageAvailable = avail
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (r *showRepo) FindByName(ctx context.Context, name string) (domain.Show, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+showCols+`,
			(SELECT COUNT(*) FROM seasons se WHERE se.show_id = s.id) AS season_count,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = s.id AND v.kind = 'episode') AS episode_count,
			0 AS unwatched_count
		FROM shows s WHERE LOWER(TRIM(s.name)) = LOWER(?) LIMIT 1`, name)
	sh, err := scanShow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Show{}, domain.ErrNotFound
	}
	return sh, err
}

func (r *showRepo) Create(ctx context.Context, s domain.Show) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO shows (id, name, overview, year, rating, genre, poster_path,
			backdrop_path, metadata_source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, nullString(s.Overview), nullInt(s.Year), nullFloat(s.Rating),
		nullString(s.Genre), nullString(s.PosterPath), nullString(s.BackdropPath),
		s.MetadataSource, s.CreatedAt, s.UpdatedAt)
	return err
}

func (r *showRepo) EnsureSeason(ctx context.Context, showID string, number int, kind string) (domain.Season, error) {
	if kind == "" {
		kind = "tv"
	}
	id := ulid.Make().String()
	seasonName := defaultSeasonName(number)
	if kind == "movie" {
		seasonName = "第 " + strconv.Itoa(number) + "部"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO seasons (id, show_id, number, name, kind) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(show_id, number) DO NOTHING`,
		id, showID, number, seasonName, kind)
	if err != nil {
		return domain.Season{}, err
	}
	var (
		se       domain.Season
		sname    sql.NullString
		overview sql.NullString
		poster   sql.NullString
		count    int
	)
	if err := r.db.QueryRowContext(ctx, `
		SELECT se.id, se.show_id, se.number, se.name, se.overview, se.poster_path,
			(SELECT COUNT(*) FROM videos v WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode') AS episode_count
		FROM seasons se WHERE se.show_id = ? AND se.number = ?`, showID, number).
		Scan(&se.ID, &se.ShowID, &se.Number, &sname, &overview, &poster, &count); err != nil {
		return domain.Season{}, err
	}
	if sname.Valid {
		se.Name = sname.String
	}
	if overview.Valid {
		se.Overview = overview.String
	}
	if poster.Valid {
		se.PosterPath = poster.String
	}
	se.EpisodeCount = count
	return se, nil
}

func defaultSeasonName(number int) string {
	return "第 " + strconv.Itoa(number) + " 季"
}

func (r *showRepo) UpdateMetadata(ctx context.Context, s domain.Show) error {
	var oldName string
	_ = r.db.QueryRowContext(ctx, `SELECT name FROM shows WHERE id = ?`, s.ID).Scan(&oldName)
	if _, err := r.db.ExecContext(ctx, `
		UPDATE shows SET name = ?, overview = ?, year = ?, rating = ?, genre = ?,
			poster_path = ?, backdrop_path = ?, metadata_source = ?, updated_at = ?
		WHERE id = ?`,
		s.Name, nullString(s.Overview), nullInt(s.Year), nullFloat(s.Rating),
		nullString(s.Genre), nullString(s.PosterPath), nullString(s.BackdropPath),
		s.MetadataSource, nowRFC3339(), s.ID); err != nil {
		return err
	}
	if oldName != "" && oldName != s.Name {
		rows, err := r.db.QueryContext(ctx,
			`SELECT id FROM videos WHERE show_id = ?`, s.ID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		// Close before rebuilding search_text so the single pooled connection
		// is not held by this result set.
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range ids {
			if err := rebuildSearchText(ctx, r.db, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *showRepo) RemoveEmptyShow(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM shows WHERE id = ? AND NOT EXISTS (SELECT 1 FROM videos WHERE show_id = ?)`,
		id, id)
	return err
}
