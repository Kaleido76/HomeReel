package search

import (
	"context"
	"strings"

	"videomesh/backend/internal/domain"
)

// Options refines a search query.
type Options struct {
	Limit int
}

// Provider is the search abstraction (ADR-009). Controllers depend only on
// this interface; the FTS5 implementation can be swapped for Meilisearch or an
// AI provider without touching callers.
type Provider interface {
	Search(ctx context.Context, q string, opts Options) ([]domain.Video, error)
}

// ftsQuery converts a free-text query into a safe FTS5 MATCH expression.
// Each term becomes a quoted prefix match ("word"*); quotes are escaped and
// terms are implicitly ANDed.
func ftsQuery(q string) string {
	terms := strings.Fields(q)
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, `"`+t+`"*`)
	}
	return strings.Join(parts, " ")
}
