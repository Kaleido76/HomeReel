package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/oklog/ulid/v2"

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

const seriesCols = `se.id, se.show_id, s.name AS title, se.number AS season_number, se.kind, se.root_path,
	s.overview, s.rating, s.poster_path, s.backdrop_path, s.metadata_source,
	se.sort_manual, s.created_at, s.updated_at`

func scanSeries(row scanner, series domain.Series) (domain.Series, error) {
	var (
		overview      sql.NullString
		rating        sql.NullFloat64
		poster        sql.NullString
		backdrop      sql.NullString
		rootPath      sql.NullString
		sortManual    sql.NullInt64
		createdAt     string
		updatedAt     string
		memberCount   int
		linkCount     int
		totalDuration sql.NullFloat64
	)
	if err := row.Scan(&series.ID, &series.ShowID, &series.Title, &series.SeasonNumber, &series.Kind,
		&rootPath, &overview, &rating, &poster, &backdrop, &series.MetadataSource,
		&sortManual, &createdAt, &updatedAt, &memberCount, &linkCount, &totalDuration); err != nil {
		return domain.Series{}, err
	}
	if rootPath.Valid {
		series.RootPath = rootPath.String
	}
	if overview.Valid {
		series.Overview = overview.String
	}
	if rating.Valid {
		series.Rating = rating.Float64
	}
	if poster.Valid {
		series.PosterPath = poster.String
	}
	if backdrop.Valid {
		series.BackdropPath = backdrop.String
	}
	if sortManual.Valid {
		series.SortManual = sortManual.Int64 != 0
	}
	series.MemberCount = memberCount
	series.LinkCount = linkCount
	if totalDuration.Valid {
		series.TotalDuration = totalDuration.Float64
	}
	series.Name = seriesDisplayName(series.Title, series.SeasonNumber)
	return series, nil
}

// seriesDisplayName renders the series display name as just the series title
// (the folder name it came from). There is no season numbering on a series — a
// series has only a name that mirrors its folder, so "第 N 季" is not appended
// (2026-08: 不要给系列添加编号).
func seriesDisplayName(title string, _ int) string {
	return title
}

func (r *seriesRepo) List(ctx context.Context, q domain.SeriesQuery) ([]domain.Series, error) {
	var where []string
	var args []any
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = append(where, `s.name LIKE ?`)
		args = append(args, like)
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
			(SELECT COUNT(*) FROM link_group_members lm
				WHERE lm.group_id = (SELECT group_id FROM link_group_members WHERE series_id = se.id)
				  AND lm.series_id != se.id) AS link_count,
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
			(SELECT COUNT(*) FROM link_group_members lm
				WHERE lm.group_id = (SELECT group_id FROM link_group_members WHERE series_id = se.id)
				  AND lm.series_id != se.id) AS link_count,
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

// FindByRoot resolves the series bound to a root directory. Series identity is
// the root path — this is how scans and manual marking decide membership.
func (r *seriesRepo) FindByRoot(ctx context.Context, rootPath string) (domain.Series, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+seriesCols+`,
			(SELECT COUNT(*) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode') AS member_count,
			(SELECT COUNT(*) FROM link_group_members lm
				WHERE lm.group_id = (SELECT group_id FROM link_group_members WHERE series_id = se.id)
				  AND lm.series_id != se.id) AS link_count,
			COALESCE((SELECT SUM(v.duration) FROM videos v
				WHERE v.show_id = se.show_id AND v.season_number = se.number AND v.kind = 'episode'), 0) AS total_duration
		FROM seasons se JOIN shows s ON s.id = se.show_id
		WHERE se.root_path = ?`, rootPath)
	series, err := scanSeries(row, domain.Series{})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Series{}, domain.ErrNotFound
	}
	return series, err
}

// CreateAtRoot creates a dedicated show+season bound to rootPath (series name =
// folder name, season number 1) in one transaction. Series are never merged by
// name — every root gets its own show — so two folders with the same name stay
// two independent series. Idempotent per root path.
func (r *seriesRepo) CreateAtRoot(ctx context.Context, name, rootPath string) (domain.Series, error) {
	if existing, err := r.FindByRoot(ctx, rootPath); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Series{}, err
	}
	now := domain.Now()
	showID := ulid.Make().String()
	seasonID := ulid.Make().String()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Series{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shows (id, name, overview, rating, poster_path,
			backdrop_path, metadata_source, created_at, updated_at)
		VALUES (?, ?, NULL, NULL, NULL, NULL, 'manual', ?, ?)`,
		showID, name, now, now); err != nil {
		return domain.Series{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO seasons (id, show_id, number, name, kind, root_path)
		VALUES (?, ?, 1, ?, 'tv', ?)`,
		seasonID, showID, defaultSeasonName(1), rootPath); err != nil {
		return domain.Series{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Series{}, err
	}
	return r.FindByRoot(ctx, rootPath)
}

// BindMembers assigns the given videos to the series in one transaction: each
// member is set kind='episode' with its position and file-derived title and is
// re-pointed at the series' show (detaching it from whatever series it was in).
func (r *seriesRepo) BindMembers(ctx context.Context, seriesID string, members []domain.EpisodeAssign) error {
	if len(members) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var showID string
	var season int
	if err := tx.QueryRowContext(ctx,
		`SELECT show_id, number FROM seasons WHERE id = ?`, seriesID).Scan(&showID, &season); err != nil {
		return err
	}
	now := domain.Now()
	for _, m := range members {
		titleSource := m.TitleSource
		if titleSource == "" {
			titleSource = domain.TitleSourceFile
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE videos SET kind = 'episode', title = ?, show_id = ?, season_number = ?,
				episode_number = ?, episode_title = ?, title_source = ?, updated_at = ?
			WHERE id = ?`,
			m.Title, showID, season, m.EpisodeNumber, nullString(m.Title), titleSource, now, m.VideoID); err != nil {
			return err
		}
		if err := rebuildSearchText(ctx, tx, m.VideoID); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// SetMemberOrder persists a manual member order (拖拽重排, ADR-015 修订):
// videoIDs must be exactly the series' members, and each becomes episode_number
// 1..N in that order. It also flags seasons.sort_manual so later scans/syncs
// keep the order and only append newly imported files.
func (r *seriesRepo) SetMemberOrder(ctx context.Context, seriesID string, videoIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var showID string
	var season int
	if err := tx.QueryRowContext(ctx,
		`SELECT show_id, number FROM seasons WHERE id = ?`, seriesID).Scan(&showID, &season); err != nil {
		return err
	}
	now := domain.Now()
	for i, vid := range videoIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE videos SET episode_number = ?, updated_at = ?
			WHERE id = ? AND show_id = ? AND season_number = ? AND kind = 'episode'`,
			i+1, now, vid, showID, season); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE seasons SET sort_manual = 1 WHERE id = ?`, seriesID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetSortManual flags/unflags seasons.sort_manual. Clearing it restores the
// automatic file-name order on the next scan/sync (「按文件名字典序重新刷新排序」);
// setting it marks the series as manually ordered (拖拽重排).
func (r *seriesRepo) SetSortManual(ctx context.Context, seriesID string, manual bool) error {
	v := 0
	if manual {
		v = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE seasons SET sort_manual = ? WHERE id = ?`, v, seriesID)
	return err
}

func (r *seriesRepo) GetMembers(ctx context.Context, id string) ([]domain.SeriesMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, v.title, v.title_source, v.episode_number, v.episode_title, v.duration, v.thumb_path,
			v.relative_path, v.codec, v.audio_codec, v.container, v.segmented,
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
			m         domain.SeriesMember
			title     sql.NullString
			titleSrc  sql.NullString
			epTitle   sql.NullString
			duration  sql.NullFloat64
			thumb     sql.NullString
			codec     sql.NullString
			audio     sql.NullString
			container sql.NullString
			segmented sql.NullInt64
		)
		if err := rows.Scan(&m.VideoID, &title, &titleSrc, &m.EpisodeNumber, &epTitle, &duration,
			&thumb, &m.RelativePath, &codec, &audio, &container, &segmented, &m.Progress); err != nil {
			return nil, err
		}
		if title.Valid {
			m.Title = title.String
		}
		if titleSrc.Valid {
			m.TitleSource = titleSrc.String
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
		if codec.Valid {
			m.Codec = codec.String
		}
		if audio.Valid {
			m.AudioCodec = audio.String
		}
		if container.Valid {
			m.Container = container.String
		}
		if segmented.Valid {
			m.Segmented = segmented.Int64 != 0
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *seriesRepo) GetLinks(ctx context.Context, id string) ([]domain.SeriesLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT lm.series_id, lm.sort_index, s.name AS title, se.number AS number
		FROM link_group_members lm
		JOIN link_group_members me ON me.group_id = lm.group_id AND me.series_id = ?
		JOIN seasons se ON se.id = lm.series_id
		JOIN shows s ON s.id = se.show_id
		WHERE lm.series_id != ?
		ORDER BY lm.sort_index, s.name`, id, id)
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
		if err := rows.Scan(&link.LinkedID, &link.SortIndex, &title, &number); err != nil {
			return nil, err
		}
		link.SeriesID = id
		link.LinkedTitle = title
		link.LinkedName = seriesDisplayName(title, number)
		out = append(out, link)
	}
	return out, rows.Err()
}

// SetLinks replaces the series' link group: the resulting membership is exactly
// `{seriesID} ∪ linkedIDs` — the caller sends the full desired set (管理关联
// 全量提交，方案 B). Unchecked series leave the group (取消勾选即不再互相关联);
// each series belongs to at most one group, so members are pulled out of any
// other group they were in. Groups that drop below two members are removed.
func (r *seriesRepo) SetLinks(ctx context.Context, seriesID string, linkedIDs []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	members := map[string]bool{seriesID: true}
	for _, id := range linkedIDs {
		if id != seriesID {
			members[id] = true
		}
	}
	ids := make([]string, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// 若只剩自身 → 无关联：清掉自身所在组即可。
	if len(ids) < 2 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM link_group_members WHERE series_id = ?`, seriesID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM link_groups WHERE NOT EXISTS (
			SELECT 1 FROM link_group_members WHERE group_id = link_groups.id)`); err != nil {
			return err
		}
		return tx.Commit()
	}

	// 所有成员脱离旧组（每系列至多一组），清掉空组与只剩一员的组。
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM link_group_members WHERE series_id = ?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM link_groups WHERE NOT EXISTS (
		SELECT 1 FROM link_group_members WHERE group_id = link_groups.id)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM link_groups WHERE (
		SELECT COUNT(*) FROM link_group_members WHERE group_id = link_groups.id) < 2`); err != nil {
		return err
	}

	gid := ulid.Make().String()
	now := domain.Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO link_groups (id, created_at) VALUES (?, ?)`, gid, now); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO link_group_members (group_id, series_id, sort_index, created_at)
			VALUES (?, ?, ?, ?)`, gid, id, i, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *seriesRepo) RemoveLink(ctx context.Context, seriesID, linkedID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var gid string
	err = tx.QueryRowContext(ctx, `
		SELECT group_id FROM link_group_members WHERE series_id = ?`, seriesID).Scan(&gid)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM link_group_members WHERE group_id = ? AND series_id = ?`, gid, linkedID); err != nil {
		return err
	}
	// 组里只剩一个成员 → 整组删除（一个系列无关联可言）。
	var n int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM link_group_members WHERE group_id = ?`, gid).Scan(&n); err != nil {
		return err
	}
	if n < 2 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM link_groups WHERE id = ?`, gid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *seriesRepo) SyncShowLinks(ctx context.Context, showID string) error {
	return ensureShowGroup(ctx, r.db, r, showID)
}

// ensureShowGroup merges every season of a show into one link group, so the
// parts of a series display together. It is idempotent.
func ensureShowGroup(ctx context.Context, db *sql.DB, links domain.SeriesRepo, showID string) error {
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM seasons WHERE show_id = ? ORDER BY number`, showID)
	if err != nil {
		return err
	}
	seasons := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		seasons = append(seasons, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(seasons) < 2 {
		return nil
	}
	return links.SetLinks(ctx, seasons[0], seasons[1:])
}
