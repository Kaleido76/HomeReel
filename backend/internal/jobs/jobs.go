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
	TypeProbe        = "probe"
	TypeThumbnail    = "thumbnail"
	TypeScanSource   = "scan_source"
	TypeMarkResource = "mark_resource"
	TypeFsCopy       = "fscopy"
	TypeFsMove       = "fsmove"
	TypeConvert      = "convert"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is a persisted unit of work in the queue. Progress is a fraction in
// [0,1]; a negative value means the job's end is unknown. Subtask and
// SubtaskProgress describe the current serial sub-task and are transient live
// state filled by Service.AttachLive, not persisted columns.
type Job struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Name            string  `json:"name"`
	Target          string  `json:"target"`
	Extra           string  `json:"extra"`
	Status          string  `json:"status"`
	Progress        float64 `json:"progress"`
	Error           string  `json:"error"`
	Internal        bool    `json:"internal"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	Subtask         string   `json:"subtask,omitempty"`
	SubtaskProgress float64  `json:"subtask_progress,omitempty"`
	// EtaSeconds is the live estimate of remaining work (transient, filled by
	// AttachLive); nil = unknown.
	EtaSeconds *float64 `json:"eta_seconds,omitempty"`
}

// Repo persists jobs (SQLite implementation lives in the store package).
type Repo interface {
	Enqueue(ctx context.Context, j Job) error
	ClaimNext(ctx context.Context) (Job, bool, error)
	MarkProgress(ctx context.Context, id string, progress float64) error
	MarkDone(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, errMsg string) error
	List(ctx context.Context, limit int) ([]Job, error)
	HasActive(ctx context.Context, typ, target string) (bool, error)
	ResetRunning(ctx context.Context) error
}

// LiveStatus holds transient per-job state that is not worth persisting, such
// as the current serial sub-task line of a running long task. It is shared by
// the Service (read via AttachLive) and the Worker (written by each job's
// reporter).
type LiveStatus struct {
	mu sync.Mutex
	m  map[string]subtaskState
}

type subtaskState struct {
	text string
	pct  float64 // -1 = unknown
	eta  float64 // estimated remaining seconds, -1 = unknown
}

// NewLiveStatus builds an empty live-status registry.
func NewLiveStatus() *LiveStatus { return &LiveStatus{m: map[string]subtaskState{}} }

// SetSubtask replaces the current sub-task line of a running job.
func (l *LiveStatus) SetSubtask(id, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur, ok := l.m[id]
	if !ok {
		cur.pct = -1
	}
	cur.text = text
	l.m[id] = cur
}

// SetSubtaskProgress sets the current sub-task's percentage in [0,100].
func (l *LiveStatus) SetSubtaskProgress(id string, pct float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur := l.m[id]
	cur.pct = pct
	l.m[id] = cur
}

// SetEta records the estimated remaining seconds of a running job (negative =
// unknown). It is derived from progress + elapsed time and is transient like
// the sub-task line.
func (l *LiveStatus) SetEta(id string, eta float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur := l.m[id]
	cur.eta = eta
	l.m[id] = cur
}

// Get returns the current sub-task line (text), its percentage (-1 unknown)
// and the estimated remaining seconds (-1 unknown).
func (l *LiveStatus) Get(id string) (text string, pct float64, eta float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.m[id]
	eta = s.eta
	if eta == 0 {
		eta = -1
	}
	return s.text, s.pct, eta
}

// Remove drops the live state of a finished job.
func (l *LiveStatus) Remove(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, id)
}

// Service is the enqueue/list half of the queue.
type Service struct {
	repo Repo
	live *LiveStatus
}

// NewService builds the queue service.
func NewService(repo Repo, live *LiveStatus) *Service {
	return &Service{repo: repo, live: live}
}

// Enqueue appends a user-facing long-running job in queued state. Name is the
// human-readable title shown in the task panel (e.g. "扫描视频库 · 电影").
func (s *Service) Enqueue(ctx context.Context, typ, target, name, extra string) (string, error) {
	return s.enqueue(ctx, Job{Type: typ, Target: target, Name: name, Extra: extra})
}

// EnqueueInternal appends an internal job (short, maintenance-like work such as
// probe/thumbnail). Internal jobs are hidden from the task panel and never
// surface lifecycle notifications.
func (s *Service) EnqueueInternal(ctx context.Context, typ, target, extra string) (string, error) {
	return s.enqueue(ctx, Job{Type: typ, Target: target, Extra: extra, Internal: true})
}

func (s *Service) enqueue(ctx context.Context, j Job) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	j.ID = ulid.Make().String()
	j.Status = StatusQueued
	j.Progress = -1
	j.CreatedAt = now
	j.UpdatedAt = now
	if err := s.repo.Enqueue(ctx, j); err != nil {
		return "", err
	}
	return j.ID, nil
}

// List returns the most recent jobs, newest first.
func (s *Service) List(ctx context.Context, limit int) ([]Job, error) {
	return s.repo.List(ctx, limit)
}

// AttachLive merges the transient sub-task state onto a job list so handlers
// can expose the current serial sub-task line and a remaining-time estimate
// alongside each running job.
func (s *Service) AttachLive(list []Job) {
	for i := range list {
		text, pct, eta := s.live.Get(list[i].ID)
		list[i].Subtask = text
		list[i].SubtaskProgress = pct
		if eta >= 0 {
			list[i].EtaSeconds = &eta
		}
	}
}

// HasActive reports whether a job of typ targeting resource target is queued
// or running. Callers use it to refuse conflicting operations while a long
// task owns a resource — e.g. a rescan locks its storage volume until the
// scan result is persisted.
func (s *Service) HasActive(ctx context.Context, typ, target string) (bool, error) {
	return s.repo.HasActive(ctx, typ, target)
}

// Reporter is the live-status channel of a running job. Progress is persisted
// (overall bar, crash recovery); the sub-task line is transient in-memory
// state that is replaced in place as a long task serially steps through its
// child work.
type Reporter interface {
	// Progress sets the job's overall progress in [0,1].
	Progress(fraction float64)
	// Subtask replaces the current sub-task status line.
	Subtask(text string)
	// SubtaskProgress sets the current sub-task's percentage in [0,100].
	SubtaskProgress(pct float64)
}

// Handler executes a job and may report progress through report.
type Handler func(ctx context.Context, j Job, report Reporter) error

// Worker pulls queued jobs and runs them through the handler with bounded
// concurrency. Running jobs left over from a previous process are requeued on
// start (crash recovery, ADR-008).
type Worker struct {
	repo        Repo
	handler     Handler
	concurrency int
	live        *LiveStatus
	notify      func(ctx context.Context, j Job, err error)
}

// NewWorker builds a worker pool.
func NewWorker(repo Repo, handler Handler, concurrency int, live *LiveStatus) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{repo: repo, handler: handler, concurrency: concurrency, live: live}
}

// SetNotify installs a lifecycle callback invoked with the finished job and a
// nil error on success, or the failure reason otherwise. It runs after the job
// row is persisted, so listeners always observe the final status. Internal
// jobs are not reported.
func (w *Worker) SetNotify(fn func(ctx context.Context, j Job, err error)) {
	w.notify = fn
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
			var runErr error
			defer func() {
				if p := recover(); p != nil {
					runErr = fmt.Errorf("panic: %v", p)
				}
				final := j
				if runErr != nil {
					slog.Warn("job failed", "id", j.ID, "type", j.Type, "err", runErr)
					_ = w.repo.MarkFailed(ctx, j.ID, runErr.Error())
					final.Status = StatusFailed
					final.Error = runErr.Error()
				} else {
					_ = w.repo.MarkDone(ctx, j.ID)
					final.Status = StatusDone
				}
				if w.live != nil {
					w.live.Remove(j.ID)
				}
				if w.notify != nil && !j.Internal {
					w.notify(ctx, final, runErr)
				}
			}()
			report := newJobReporter(ctx, w.repo, j.ID, w.live)
			runErr = w.handler(ctx, j, report)
			report.flush()
		}(job)
	}
}

// jobReporter implements Reporter for one running job: overall progress is
// persisted with throttling so a long scan doesn't hammer SQLite, and the
// sub-task line lives in the shared in-memory LiveStatus.
type jobReporter struct {
	ctx      context.Context
	repo     Repo
	id       string
	live     *LiveStatus
	started  time.Time
	mu       sync.Mutex
	last     float64
	lastTime time.Time
}

const (
	progressInterval = 250 * time.Millisecond
	progressMinDelta = 0.01
)

func newJobReporter(ctx context.Context, repo Repo, id string, live *LiveStatus) *jobReporter {
	return &jobReporter{ctx: ctx, repo: repo, id: id, live: live, started: time.Now(), last: -1}
}

// Progress records a new overall value, persisting it if it is far enough
// from the last written value. Values outside [0,1] are ignored. The remaining
// time is estimated from the elapsed wall time and the current fraction, and is
// kept fresh in the shared live registry on every report (independent of the
// throttled DB write).
func (r *jobReporter) Progress(fraction float64) {
	if fraction < 0 || fraction > 1 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if r.live != nil {
		r.live.SetEta(r.id, computeETA(r.started, now, fraction))
	}
	if r.last >= 0 && now.Sub(r.lastTime) < progressInterval && absFloat(fraction-r.last) < progressMinDelta {
		return
	}
	r.last = fraction
	r.lastTime = now
	_ = r.repo.MarkProgress(r.ctx, r.id, fraction)
}

// computeETA estimates the seconds left from the time spent and the fraction
// completed; unknown (negative) while nothing is done, zero once complete.
func computeETA(started, now time.Time, fraction float64) float64 {
	if fraction <= 0 {
		return -1
	}
	if fraction >= 1 {
		return 0
	}
	elapsed := now.Sub(started).Seconds()
	return elapsed * (1 - fraction) / fraction
}

// flush persists the most recent value, covering a progress report that was
// skipped by throttling right before the handler returned.
func (r *jobReporter) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last >= 0 {
		_ = r.repo.MarkProgress(r.ctx, r.id, r.last)
	}
}

// Subtask replaces the current sub-task status line (in-memory only).
func (r *jobReporter) Subtask(text string) {
	if r.live != nil {
		r.live.SetSubtask(r.id, text)
	}
}

// SubtaskProgress sets the current sub-task's percentage.
func (r *jobReporter) SubtaskProgress(pct float64) {
	if r.live != nil {
		r.live.SetSubtaskProgress(r.id, pct)
	}
}

func absFloat(n float64) float64 {
	if n < 0 {
		return -n
	}
	return n
}
