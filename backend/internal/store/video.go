package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/files"
)

type videoRepo struct {
	db *sql.DB
}

// NewVideoRepo returns a SQLite-backed domain.VideoRepo.
func NewVideoRepo(database *sql.DB) domain.VideoRepo {
	return &videoRepo{db: database}
}

// scanner is the shared row-scan abstraction for both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// queryer abstracts *sql.DB and *sql.Tx so FTS search_text rebuilds can run
// inside a grouping transaction.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const videoCols = `id, source_id, file_id, relative_path, path, size, mtime, title,
	kind, description, duration, codec, audio_codec, container, segmented, width, height, fps, file_size,
	cover_path, thumb_path, backdrop_path, show_id, season_number, episode_number, episode_title,
	year, rating, genre, overview, studio, cast_text, metadata_source,
	created_at, updated_at, last_scanned_at`

// qualify prefixes every column with a table alias so videoCols can be used in
// JOIN queries without ambiguous names.
func qualify(cols, alias string) string {
	parts := strings.Split(cols, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(out, ", ")
}

func scanVideo(row scanner) (domain.Video, error) {
	var (
		v             domain.Video
		duration      sql.NullFloat64
		codec         sql.NullString
		audioCodec    sql.NullString
		container     sql.NullString
		segmented     sql.NullInt64
		width         sql.NullInt64
		height        sql.NullInt64
		fps           sql.NullFloat64
		fileSize      sql.NullInt64
		cover         sql.NullString
		thumb         sql.NullString
		backdrop      sql.NullString
		showID        sql.NullString
		seasonNumber  sql.NullInt64
		episodeNumber sql.NullInt64
		episodeTitle  sql.NullString
		year          sql.NullInt64
		rating        sql.NullFloat64
		genre         sql.NullString
		overview      sql.NullString
		studio        sql.NullString
		castText      sql.NullString
	)
	if err := row.Scan(&v.ID, &v.SourceID, &v.FileID, &v.RelativePath, &v.Path,
		&v.Size, &v.MTime, &v.Title, &v.Kind, &v.Description, &duration, &codec,
		&audioCodec, &container, &segmented, &width, &height, &fps, &fileSize,
		&cover, &thumb, &backdrop, &showID, &seasonNumber, &episodeNumber, &episodeTitle,
		&year, &rating, &genre, &overview, &studio, &castText, &v.MetadataSource,
		&v.CreatedAt, &v.UpdatedAt, &v.LastScannedAt); err != nil {
		return domain.Video{}, err
	}
	if duration.Valid {
		v.Duration = duration.Float64
	}
	if codec.Valid {
		v.Codec = codec.String
	}
	if audioCodec.Valid {
		v.AudioCodec = audioCodec.String
	}
	if container.Valid {
		v.Container = container.String
	}
	if segmented.Valid {
		v.Segmented = segmented.Int64 != 0
	}
	if width.Valid {
		v.Width = int(width.Int64)
	}
	if height.Valid {
		v.Height = int(height.Int64)
	}
	if fps.Valid {
		v.FPS = fps.Float64
	}
	if fileSize.Valid {
		v.FileSize = fileSize.Int64
	}
	if cover.Valid {
		v.CoverPath = cover.String
	}
	if thumb.Valid {
		v.ThumbPath = thumb.String
	}
	if backdrop.Valid {
		v.BackdropPath = backdrop.String
	}
	if showID.Valid {
		v.ShowID = showID.String
	}
	if seasonNumber.Valid {
		v.SeasonNumber = int(seasonNumber.Int64)
	}
	if episodeNumber.Valid {
		v.EpisodeNumber = int(episodeNumber.Int64)
	}
	if episodeTitle.Valid {
		v.EpisodeTitle = episodeTitle.String
	}
	if year.Valid {
		v.Year = int(year.Int64)
	}
	if rating.Valid {
		v.Rating = rating.Float64
	}
	if genre.Valid {
		v.Genre = genre.String
	}
	if overview.Valid {
		v.Overview = overview.String
	}
	if studio.Valid {
		v.Studio = studio.String
	}
	if castText.Valid {
		v.CastText = castText.String
	}
	return v, nil
}

func (r *videoRepo) Get(ctx context.Context, id string) (domain.Video, error) {
	return getVideo(ctx, r.db, id)
}

func getVideo(ctx context.Context, q queryer, id string) (domain.Video, error) {
	row := q.QueryRowContext(ctx, `SELECT `+videoCols+` FROM videos WHERE id = ?`, id)
	v, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, domain.ErrNotFound
	}
	return v, err
}

// listSort maps a user-facing sort key to a safe SQL ORDER BY expression.
var listSort = map[string]string{
	"title":    "title",
	"name":     "relative_path",
	"date":     "created_at",
	"duration": "duration",
	"rating":   "rating",
}

func (r *videoRepo) List(ctx context.Context, q domain.VideoQuery) (domain.VideoPage, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 24
	}
	if pageSize > 200 {
		pageSize = 200
	}

	var where []string
	var args []any
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = append(where, `(title LIKE ? OR relative_path LIKE ?)`)
		args = append(args, like, like)
	}
	if q.Desc != "" {
		like := "%" + q.Desc + "%"
		where = append(where, `description LIKE ?`)
		args = append(args, like)
	}
	if q.Genre != "" {
		like := "%" + q.Genre + "%"
		where = append(where, `genre LIKE ?`)
		args = append(args, like)
	}
	if q.Year > 0 {
		where = append(where, `year = ?`)
		args = append(args, q.Year)
	}
	if q.Kind != "" {
		where = append(where, `kind = ?`)
		args = append(args, q.Kind)
	}
	if q.ShowID != "" {
		where = append(where, `show_id = ?`)
		args = append(args, q.ShowID)
	}
	if q.Ungrouped {
		where = append(where, `show_id IS NULL`)
	}
	for _, tag := range q.Tags {
		where = append(where, `EXISTS (SELECT 1 FROM video_tags vt WHERE vt.video_id = videos.id AND vt.tag = ?)`)
		args = append(args, tag)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM videos`+whereSQL, args...).Scan(&total); err != nil {
		return domain.VideoPage{}, err
	}

	order := "DESC"
	if q.Order == "asc" {
		order = "ASC"
	}
	sort := listSort[q.Sort]
	if sort == "" {
		sort = "created_at"
	}
	query := `SELECT ` + videoCols + ` FROM videos` + whereSQL +
		` ORDER BY ` + sort + ` ` + order +
		` LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.VideoPage{}, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0, pageSize)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return domain.VideoPage{}, err
		}
		out = append(out, v)
	}
	return domain.VideoPage{Videos: out, Total: total}, rows.Err()
}

// Create inserts a video with its full probe metadata in a single statement so
// a video never appears in the library with half its metadata (the scanner
// probes before creating). kind defaults to standalone.
func (r *videoRepo) Create(ctx context.Context, v domain.Video) error {
	if v.Kind == "" {
		v.Kind = "movie"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO videos (id, source_id, file_id, relative_path, path, size, mtime,
			title, kind, description, duration, codec, container, segmented, width, height,
			created_at, updated_at, last_scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, nullString(v.SourceID), v.FileID, v.RelativePath, v.Path, v.Size, v.MTime,
		v.Title, v.Kind, v.Description, nullFloat(v.Duration), nullString(v.Codec),
		nullString(v.Container), segmentedInt(v.Segmented), v.Width, v.Height,
		v.CreatedAt, v.UpdatedAt, v.LastScannedAt)
	if err != nil {
		return err
	}
	return rebuildSearchText(ctx, r.db, v.ID)
}

func (r *videoRepo) UpdateFingerprint(ctx context.Context, id, sourceID, path, relativePath string, size, mtime int64, lastScannedAt string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos SET source_id = ?, path = ?, relative_path = ?, size = ?, mtime = ?,
			updated_at = ?, last_scanned_at = ?
		WHERE id = ?`,
		nullString(sourceID), path, relativePath, size, mtime, domain.Now(), lastScannedAt, id)
	return err
}

func (r *videoRepo) Touch(ctx context.Context, id, lastScannedAt string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET last_scanned_at = ? WHERE id = ?`, lastScannedAt, id)
	return err
}

func (r *videoRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM videos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *videoRepo) UpdateProbe(ctx context.Context, v domain.Video) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos SET title = ?, duration = ?, codec = ?, container = ?,
			segmented = ?, width = ?, height = ?, updated_at = ?
		WHERE id = ?`,
		v.Title, v.Duration, nullString(v.Codec), nullString(v.Container),
		segmentedInt(v.Segmented), v.Width, v.Height, domain.Now(), v.ID)
	if err != nil {
		return err
	}
	return rebuildSearchText(ctx, r.db, v.ID)
}

func segmentedInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *videoRepo) UpdateCovers(ctx context.Context, id, coverPath, thumbPath string) error {
	sets := []string{}
	args := []any{}
	if coverPath != "" {
		sets = append(sets, "cover_path = ?")
		args = append(args, coverPath)
	}
	if thumbPath != "" {
		sets = append(sets, "thumb_path = ?")
		args = append(args, thumbPath)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, domain.Now(), id)
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return err
}

func (r *videoRepo) UpdateMetadata(ctx context.Context, id string, patch domain.VideoPatch) error {
	sets := []string{}
	args := []any{}
	if patch.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *patch.Title)
	}
	if patch.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *patch.Description)
	}
	if patch.Kind != nil {
		sets = append(sets, "kind = ?")
		args = append(args, *patch.Kind)
	}
	if patch.Year != nil {
		sets = append(sets, "year = ?")
		args = append(args, *patch.Year)
	}
	if patch.Rating != nil {
		sets = append(sets, "rating = ?")
		args = append(args, *patch.Rating)
	}
	if patch.Genre != nil {
		sets = append(sets, "genre = ?")
		args = append(args, *patch.Genre)
	}
	if patch.Overview != nil {
		sets = append(sets, "overview = ?")
		args = append(args, *patch.Overview)
	}
	if patch.Studio != nil {
		sets = append(sets, "studio = ?")
		args = append(args, *patch.Studio)
	}
	if patch.CastText != nil {
		sets = append(sets, "cast_text = ?")
		args = append(args, *patch.CastText)
	}
	if patch.BackdropPath != nil {
		sets = append(sets, "backdrop_path = ?")
		args = append(args, nullString(*patch.BackdropPath))
	}
	if patch.ShowID != nil {
		if *patch.ShowID == "" {
			sets = append(sets, "show_id = NULL, season_number = NULL, episode_number = NULL, episode_title = NULL")
		} else {
			sets = append(sets, "show_id = ?")
			args = append(args, *patch.ShowID)
		}
	}
	if patch.SeasonNumber != nil {
		sets = append(sets, "season_number = ?")
		args = append(args, *patch.SeasonNumber)
	}
	if patch.EpisodeNumber != nil {
		sets = append(sets, "episode_number = ?")
		args = append(args, *patch.EpisodeNumber)
	}
	if patch.EpisodeTitle != nil {
		sets = append(sets, "episode_title = ?")
		args = append(args, *patch.EpisodeTitle)
	}
	if patch.MetadataSource != nil {
		sets = append(sets, "metadata_source = ?")
		args = append(args, *patch.MetadataSource)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, domain.Now(), id)
	if _, err := r.db.ExecContext(ctx,
		`UPDATE videos SET `+strings.Join(sets, ", ")+`, updated_at = ? WHERE id = ?`, args...); err != nil {
		return err
	}
	return rebuildSearchText(ctx, r.db, id)
}

// AssignStandalone marks every given video as a standalone movie in a single
// statement (used by the scan-end grouping pass), then rebuilds each row's
// search text. Clearing the series linkage is atomic as one UPDATE.
func (r *videoRepo) AssignStandalone(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, domain.Now())
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE videos SET kind = 'movie', show_id = NULL, season_number = NULL,
			episode_number = NULL, episode_title = NULL, updated_at = ?
		WHERE id IN (`+placeholders+`)`, args...); err != nil {
		return err
	}
	for _, id := range ids {
		_ = rebuildSearchText(ctx, r.db, id)
	}
	return nil
}

func (r *videoRepo) SetTags(ctx context.Context, id string, tags []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM video_tags WHERE video_id = ?`, id); err != nil {
		return err
	}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO video_tags (video_id, tag) VALUES (?, ?)`, id, tag); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return rebuildSearchText(ctx, r.db, id)
}

func (r *videoRepo) Tags(ctx context.Context, id string) ([]string, error) {
	return getTags(ctx, r.db, id)
}

func getTags(ctx context.Context, q queryer, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT tag FROM video_tags WHERE video_id = ? ORDER BY tag`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func (r *videoRepo) AllTags(ctx context.Context) ([]domain.TagCount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tag, COUNT(*) FROM video_tags GROUP BY tag ORDER BY COUNT(*) DESC, tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TagCount{}
	for rows.Next() {
		var tc domain.TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

func (r *videoRepo) ListAll(ctx context.Context) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+videoCols+` FROM videos ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *videoRepo) ListBySource(ctx context.Context, sourceID string) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+videoCols+` FROM videos WHERE source_id = ? ORDER BY relative_path`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *videoRepo) ListSegmented(ctx context.Context) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+videoCols+` FROM videos WHERE segmented = 1 ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Video, 0)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *videoRepo) ContinueWatching(ctx context.Context, limit int) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+qualify(videoCols, "v")+` FROM videos v
		JOIN history h ON h.video_id = v.id AND h.user = 'local'
		WHERE h.progress > 0 AND (v.duration = 0 OR h.progress < v.duration - 20)
		ORDER BY h.updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// MarkMissingBySource deletes videos owned by source whose last_scanned_at is
// older than since, skipping any whose current absolute path falls under a
// child source root (those belong to the child's scan). It returns the deleted
// ids so callers can publish deletion events.
func (r *videoRepo) MarkMissingBySource(ctx context.Context, sourceID, since string, excludeRoots []string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, path FROM videos WHERE source_id = ? AND last_scanned_at < ?`, sourceID, since)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id, p string
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return nil, err
		}
		if files.UnderAnyRoot(p, excludeRoots) {
			continue
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, 0, len(ids))
		for _, id := range ids {
			args = append(args, id)
		}
		if _, err := r.db.ExecContext(ctx,
			`DELETE FROM videos WHERE id IN (`+placeholders+`)`, args...); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// rebuildSearchText rewrites the denormalised search_text column (title +
// description + tags + show name) so FTS5 stays searchable (ADR-009). The
// videos_au trigger keeps videos_fts in sync. It runs against a queryer so the
// rebuild can join a grouping transaction.
func rebuildSearchText(ctx context.Context, q queryer, id string) error {
	v, err := getVideo(ctx, q, id)
	if err != nil {
		return err
	}
	tags, err := getTags(ctx, q, id)
	if err != nil {
		return err
	}
	parts := []string{v.Title, v.Description}
	parts = append(parts, tags...)
	if v.ShowID != "" {
		var showName string
		if err := q.QueryRowContext(ctx,
			`SELECT name FROM shows WHERE id = ?`, v.ShowID).Scan(&showName); err == nil && showName != "" {
			parts = append(parts, showName)
		}
	}
	text := strings.Join(parts, " ")
	_, err = q.ExecContext(ctx,
		`UPDATE videos SET search_text = ? WHERE id = ?`, text, id)
	return err
}
