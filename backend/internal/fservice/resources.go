package fservice

import (
	"context"
	"encoding/json"
	"path/filepath"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/jobs"
)

// Manual series marking (文件页签「标记为系列」): a folder inside a media source
// is turned into a series whose members are its direct video children. Marking
// only enqueues a mark_resource job — the actual import/grouping runs in the
// scanner worker. Paths outside every media source are rejected (discrete
// resources no longer exist, 管理面定稿 2026-08).

// MarkResources enqueues a mark_resource job per marked path (kind must be
// "series").
func (s *Service) MarkResources(ctx context.Context, paths []string, kind string) ([]string, error) {
	if kind != "series" {
		return nil, domain.ErrInvalid
	}
	jobIDs := make([]string, 0, len(paths))
	for _, p := range paths {
		p = normalizeSourcePath(p)
		if p == "" {
			return jobIDs, domain.ErrInvalid
		}
		extra, err := json.Marshal(map[string]string{"path": p, "kind": kind})
		if err != nil {
			return jobIDs, err
		}
		id, err := s.jobs.Enqueue(ctx, jobs.TypeMarkResource, p,
			"标记系列 · "+filepath.Base(p), string(extra))
		if err != nil {
			return jobIDs, err
		}
		jobIDs = append(jobIDs, id)
	}
	return jobIDs, nil
}
