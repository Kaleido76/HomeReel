package search

import (
	"context"
	"database/sql"

	"videomesh/backend/internal/domain"
)

// FTS5 is the SQLite full-text implementation of Provider (ADR-009). It
// matches against the videos_fts virtual table and resolves full videos
// through the repo, so callers always receive complete records.
type FTS5 struct {
	db     *sql.DB
	videos domain.VideoRepo
}

// NewFTS5 builds a Provider backed by the videos_fts external-content table.
func NewFTS5(database *sql.DB, videos domain.VideoRepo) Provider {
	return &FTS5{db: database, videos: videos}
}

func (f *FTS5) Search(ctx context.Context, q string, opts Options) ([]domain.Video, error) {
	if q == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := f.db.QueryContext(ctx, `
		SELECT v.id
		FROM videos_fts f JOIN videos v ON v.rowid = f.rowid
		WHERE videos_fts MATCH ?
		ORDER BY bm25(videos_fts)
		LIMIT ?`, ftsQuery(q), limit)
	if err != nil {
		return nil, err
	}
	// Collect IDs first and close rows before resolving videos, so the single
	// pooled connection (SetMaxOpenConns(1)) is free for the per-ID Get calls.
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := []domain.Video{}
	for _, id := range ids {
		v, err := f.videos.Get(ctx, id)
		if err != nil {
			if err == domain.ErrNotFound {
				continue
			}
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
