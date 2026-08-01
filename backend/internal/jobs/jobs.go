package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Job types (ADR-008).
const (
	TypeProbe     = "probe"
	TypeThumbnail = "thumbnail"
	TypeRescan    = "rescan"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is a persisted unit of work in the queue.
type Job struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Target    string  `json:"target"`
	Extra     string  `json:"extra"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	Error     string  `json:"error"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// Repo persists jobs (SQLite implementation lives in the store package).
type Repo interface {
	Enqueue(ctx context.Context, j Job) error
	ClaimNext(ctx context.Context) (Job, bool, error)
	MarkDone(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, errMsg string) error
	List(ctx context.Context, limit int) ([]Job, error)
	ResetRunning(ctx context.Context) error
}

// Service is the enqueue/list half of the queue.
type Service struct {
	repo Repo
}

// NewService builds the queue service.
func NewService(repo Repo) *Service { return &Service{repo: repo} }

// Enqueue appends a job in queued state.
func (s *Service) Enqueue(ctx context.Context, typ, target, extra string) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	j := Job{
		ID:        ulid.Make().String(),
		Type:      typ,
		Target:    target,
		Extra:     extra,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Enqueue(ctx, j); err != nil {
		return "", err
	}
	return j.ID, nil
}

// List returns the most recent jobs, newest first.
func (s *Service) List(ctx context.Context, limit int) ([]Job, error) {
	return s.repo.List(ctx, limit)
}

// Handler executes a job.
type Handler func(ctx context.Context, j Job) error

// Worker pulls queued jobs and runs them through the handler with bounded
// concurrency. Running jobs left over from a previous process are requeued on
// start (crash recovery, ADR-008).
type Worker struct {
	repo        Repo
	handler     Handler
	concurrency int
}

// NewWorker builds a worker pool.
func NewWorker(repo Repo, handler Handler, concurrency int) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{repo: repo, handler: handler, concurrency: concurrency}
}

// Run processes jobs until ctx is cancelled. The goroutine dispatcher loop
// keeps up to concurrency jobs running at once.
func (w *Worker) Run(ctx context.Context) {
	if err := w.repo.ResetRunning(ctx); err != nil {
		slog.Error("reset running jobs", "err", err)
	}
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}
		job, ok, err := w.repo.ClaimNext(ctx)
		if err != nil {
			slog.Error("claim job", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if p := recover(); p != nil {
					_ = w.repo.MarkFailed(ctx, j.ID, fmt.Sprintf("panic: %v", p))
				}
			}()
			if err := w.handler(ctx, j); err != nil {
				slog.Warn("job failed", "id", j.ID, "type", j.Type, "err", err)
				_ = w.repo.MarkFailed(ctx, j.ID, err.Error())
				return
			}
			_ = w.repo.MarkDone(ctx, j.ID)
		}(job)
	}
}
