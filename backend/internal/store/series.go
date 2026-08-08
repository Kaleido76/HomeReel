package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"homereel/backend/internal/domain"
)

type seriesRepo struct {
	db *sql.DB
}

// NewSeriesRepo returns a SQLite-backed domain.SeriesRepo. A series is a
// seasons row (one season of a show / one part of a movie franchise).
func NewSeriesRepo(database *sql.DB) domain.SeriesRepo {
	return &seriesRepo{db: database}
}

const seriesCols = `se.id, se.show_id, s.name AS title, se.number AS season_number, se.kind,
	s.overview, s.year, s.rating, s.genre, s.poster_path, s.backdrop_path, s.metadata_source,
	s.created_at, s.updated_at`

func scanSeries(row scanner, series domain.Series) (domain.Series, error) {
	var (
		overview      sql.NullString
		year          sql.NullInt64
		rating        sql.NullFloat64
		genre         sql.NullString
		poster        sql.NullString
		backdrop      sql.NullString
		createdAt     string
		updatedAt     string
		memberCount   int
		linkCount     int
		totalDuration sql.NullFloat64
	)
	if err := row.Scan(&series.ID, &series.ShowID, &series.Title, &series.SeasonNumber, &series.Kind,
		&overview, &year, &rating, &genre, &poster, &backdrop, &series.MetadataSource,
		&createdAt, &updatedAt, &memberCount, &linkCount, &totalDuration); err != nil {
		return domain.Series{}, err
	}
	if overview.Valid {
		series.Overview = overview.String
	}
	if year.Valid {
		series.Year = int(year.Int64)
	}
	if rating.Valid {
		series.Rating = rating.Float64
	}
	if genre.Valid {
		series.Genre = genre.String
	}
	if poster.Valid {
		series.PosterPath = poster.String
	}
	if backdrop.Valid {
		series.BackdropPath = backdrop.String
	}
	series.MemberCount = memberCount
	series.LinkCount = linkCount
	if totalDuration.Valid {
		series.TotalDuration = totalDuration.Float64
	}
	series.Name = seriesDisplayName(series.Title, series.SeasonNumber)
	return series, nil
}

// seriesDisplayName renders a series name as "标题 第 N 季". There is no
// movie/tv structural type, so every numbered unit reads as a 季; any
// movie/tv distinction is a tag on the videos instead.
func seriesDisplayName(title string, number int) string {
	return title + " 第" + strconv.Itoa(number) + "季"
}

func (r *seriesRepo) List(ctx context.Context, q domain.SeriesQuery) ([]domain.Series, error) {
	var where []string
	var args []any
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = append(where, `(s.name LIKE ? OR s.overview LIKE ?)`)
		args = append(args, like, like)
	}
	if q.Genre != "" {
		like := "%" + q.Genre + "%"
		where = append(where, `s.genre LIKE ?`)
		args = append(args, like)
	}
	if q.Year > 0 {
		where = append(where, `s.year = ?`)
		args = append(args, q.Year)
	}
	for _, tag := range q.Tags {
		where = append(where, `EXISTS (SELECT 1 FROM videos v JOIN video_tags vt ON vt.video_id = v.id
			WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode' AND vt.tag = ?)`)
		args = append(args, tag)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+seriesCols+`,
			(SELECT COUNT(*) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode') AS member_count,
			(SELECT COUNT(*) FROM series_links sl
				WHERE sl.series_id = se.id OR sl.linked_series_id = se.id) AS link_count,
			COALESCE((SELECT SUM(v.duration) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode'), 0) AS total_duration
		FROM seasons se JOIN shows s ON s.id = se.show_id`+whereSQL+`
		ORDER BY s.name, se.number`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Series{}
	for rows.Next() {
		series, err := scanSeries(rows, domain.Series{})
		if err != nil {
			return nil, err
		}
		out = append(out, series)
	}
	return out, rows.Err()
}

func (r *seriesRepo) Get(ctx context.Context, id string) (domain.Series, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+seriesCols+`,
			(SELECT COUNT(*) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode') AS member_count,
			(SELECT COUNT(*) FROM series_links sl
				WHERE sl.series_id = se.id OR sl.linked_series_id = se.id) AS link_count,
			COALESCE((SELECT SUM(v.duration) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode'), 0) AS total_duration
		FROM seasons se JOIN shows s ON s.id = se.show_id
		WHERE se.id = ?`, id)
	series, err := scanSeries(row, domain.Series{})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Series{}, domain.ErrNotFound
	}
	return series, err
}

func (r *seriesRepo) FindID(ctx context.Context, showID string, seasonNumber int) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM seasons WHERE show_id = ? AND number = ?`, showID, seasonNumber).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return id, err
}

func (r *seriesRepo) GetMembers(ctx context.Context, id string) ([]domain.SeriesMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, v.title, v.episode_number, v.episode_title, v.duration, v.thumb_path,
			v.relative_path,
			COALESCE(h.progress, 0) AS progress
		FROM videos v
		JOIN seasons se ON se.id = ?
		LEFT JOIN history h ON h.video_id = v.id AND h.user = 'local'
		WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode'
		ORDER BY v.episode_number, v.title`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SeriesMember{}
	for rows.Next() {
		var (
			m        domain.SeriesMember
			title    sql.NullString
			epTitle  sql.NullString
			duration sql.NullFloat64
			thumb    sql.NullString
		)
		if err := rows.Scan(&m.VideoID, &title, &m.EpisodeNumber, &epTitle, &duration,
			&thumb, &m.RelativePath, &m.Progress); err != nil {
			return nil, err
		}
		if title.Valid {
			m.Title = title.String
		}
		if epTitle.Valid {
			m.EpisodeTitle = epTitle.String
		}
		if duration.Valid {
			m.Duration = duration.Float64
		}
		if thumb.Valid {
			m.ThumbPath = thumb.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *seriesRepo) GetLinks(ctx context.Context, id string) ([]domain.SeriesLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sl.series_id, sl.linked_series_id, sl.sort_index,
			s2.name AS title, se2.number AS number
		FROM series_links sl
		JOIN seasons se2 ON se2.id = sl.linked_series_id
		JOIN shows s2 ON s2.id = se2.show_id
		WHERE sl.series_id = ?
		UNION ALL
		SELECT sl.series_id, sl.linked_series_id, sl.sort_index,
			s1.name AS title, se1.number AS number
		FROM series_links sl
		JOIN seasons se1 ON se1.id = sl.series_id
		JOIN shows s1 ON s1.id = se1.show_id
		WHERE sl.linked_series_id = ?
		ORDER BY sort_index, title`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SeriesLink{}
	for rows.Next() {
		var (
			link   domain.SeriesLink
			title  string
			number int
		)
		if err := rows.Scan(&link.SeriesID, &link.LinkedID, &link.SortIndex, &title, &number); err != nil {
			return nil, err
		}
		link.LinkedTitle = title
		link.LinkedName = seriesDisplayName(title, number)
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *seriesRepo) AddLink(ctx context.Context, seriesID, linkedID string, sortIndex int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO series_links (series_id, linked_series_id, sort_index, created_at)
		VALUES (?, ?, ?, ?)`, seriesID, linkedID, sortIndex, domain.Now())
	return err
}

func (r *seriesRepo) RemoveLink(ctx context.Context, seriesID, linkedID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM series_links
		WHERE (series_id = ? AND linked_series_id = ?)
		   OR (series_id = ? AND linked_series_id = ?)`,
		seriesID, linkedID, linkedID, seriesID)
	return err
}

func (r *seriesRepo) SyncShowLinks(ctx context.Context, showID string) error {
	return ensureAdjacentLinks(ctx, r.db, r, showID)
}

// ensureAdjacentLinks creates ordered links between consecutive seasons of a
// show, so the parts of a series are shown together. It is idempotent.
func ensureAdjacentLinks(ctx context.Context, db *sql.DB, links domain.SeriesRepo, showID string) error {
	rows, err := db.QueryContext(ctx, `
		SELECT id, number FROM seasons WHERE show_id = ? ORDER BY number`, showID)
	if err != nil {
		return err
	}
	type kv struct {
		id     string
		number int
	}
	seasons := []kv{}
	for rows.Next() {
		var k kv
		if err := rows.Scan(&k.id, &k.number); err != nil {
			rows.Close()
			return err
		}
		seasons = append(seasons, k)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for i := 1; i < len(seasons); i++ {
		if err := links.AddLink(ctx, seasons[i-1].id, seasons[i].id, i-1); err != nil {
			return err
		}
	}
	return nil
}
