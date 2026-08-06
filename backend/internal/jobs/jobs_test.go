package jobs

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memRepo struct {
	mu   sync.Mutex
	jobs []Job
	next int
}

func (m *memRepo) Enqueue(_ context.Context, j Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = append(m.jobs, j)
	return nil
}

func (m *memRepo) ClaimNext(_ context.Context) (Job, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for m.next < len(m.jobs) {
		j := m.jobs[m.next]
		m.next++
		if j.Status == StatusQueued {
			j.Status = StatusRunning
			m.jobs[m.next-1] = j
			return j, true, nil
		}
	}
	return Job{}, false, nil
}

func (m *memRepo) MarkProgress(_ context.Context, id string, progress float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		if m.jobs[i].ID == id {
			m.jobs[i].Progress = progress
		}
	}
	return nil
}

func (m *memRepo) MarkDone(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		if m.jobs[i].ID == id {
			m.jobs[i].Status = StatusDone
			m.jobs[i].Progress = 1
		}
	}
	return nil
}

func (m *memRepo) MarkFailed(_ context.Context, id, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		if m.jobs[i].ID == id {
			m.jobs[i].Status = StatusFailed
			m.jobs[i].Error = msg
		}
	}
	return nil
}

func (m *memRepo) List(_ context.Context, _ int) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, len(m.jobs))
	copy(out, m.jobs)
	return out, nil
}

func (m *memRepo) HasActive(_ context.Context, typ, target string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Type == typ && j.Target == target && (j.Status == StatusQueued || j.Status == StatusRunning) {
			return true, nil
		}
	}
	return false, nil
}

func (m *memRepo) ResetRunning(_ context.Context) error { return nil }

func TestWorkerProcessesJobs(t *testing.T) {
	repo := &memRepo{}
	live := NewLiveStatus()
	svc := NewService(repo, live)

	var mu sync.Mutex
	processed := map[string]bool{}
	handler := func(_ context.Context, j Job, _ Reporter) error {
		mu.Lock()
		processed[j.Type] = true
		mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewWorker(repo, handler, 2, live).Run(ctx)
		close(done)
	}()

	if _, err := svc.Enqueue(ctx, "probe", "t1", "探测", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Enqueue(ctx, "probe", "t2", "探测", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Enqueue(ctx, "thumbnail", "t3", "缩略图", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := repo.List(context.Background(), 10)
		if len(jobs) == 3 {
			allDone := true
			for _, j := range jobs {
				if j.Status != StatusDone {
					allDone = false
				}
			}
			if allDone {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done

	jobs, _ := repo.List(context.Background(), 10)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	for _, j := range jobs {
		if j.Status != StatusDone {
			t.Fatalf("job %s status = %s, want done", j.ID, j.Status)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !processed["probe"] || !processed["thumbnail"] {
		t.Fatalf("handler not called for all types: %v", processed)
	}
}

func TestWorkerReportsProgress(t *testing.T) {
	repo := &memRepo{}
	live := NewLiveStatus()
	svc := NewService(repo, live)
	reported := make(chan struct{})
	release := make(chan struct{})
	handler := func(_ context.Context, _ Job, report Reporter) error {
		report.Progress(0.75)
		close(reported)
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewWorker(repo, handler, 1, live).Run(ctx)
		close(done)
	}()
	if _, err := svc.Enqueue(ctx, "rescan", "vol1", "扫描视频库 · 电影", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	select {
	case <-reported:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not report progress")
	}
	jobs, _ := repo.List(context.Background(), 10)
	if len(jobs) != 1 || jobs[0].Progress != 0.75 {
		t.Fatalf("running progress = %+v, want 0.75", jobs)
	}
	close(release)
	cancel()
	<-done
}

func TestReporterSubtask(t *testing.T) {
	repo := &memRepo{}
	live := NewLiveStatus()
	svc := NewService(repo, live)
	release := make(chan struct{})
	handler := func(_ context.Context, _ Job, report Reporter) error {
		report.Subtask("探测 a.mp4")
		report.SubtaskProgress(65)
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewWorker(repo, handler, 1, live).Run(ctx)
		close(done)
	}()
	id, err := svc.Enqueue(ctx, "rescan", "vol1", "扫描视频库 · 电影", "")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if text, _ := live.Get(id); text == "探测 a.mp4" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	text, pct := live.Get(id)
	if text != "探测 a.mp4" || pct != 65 {
		t.Fatalf("subtask = %q %v, want 探测 a.mp4 65", text, pct)
	}
	// AttachLive merges the live sub-task onto a job listing for the API.
	jobs, _ := repo.List(ctx, 10)
	svc.AttachLive(jobs)
	if len(jobs) != 1 || jobs[0].Subtask != "探测 a.mp4" || jobs[0].SubtaskProgress != 65 {
		t.Fatalf("attached job = %+v", jobs)
	}
	close(release)
	cancel()
	<-done
}

func TestWorkerMarksFailedAndNotifies(t *testing.T) {
	repo := &memRepo{}
	live := NewLiveStatus()
	svc := NewService(repo, live)
	handler := func(_ context.Context, _ Job, _ Reporter) error {
		return &errBoom{}
	}
	var mu sync.Mutex
	notified := map[string]error{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker := NewWorker(repo, handler, 1, live)
		worker.SetNotify(func(_ context.Context, j Job, err error) {
			mu.Lock()
			notified[j.ID] = err
			mu.Unlock()
		})
		worker.Run(ctx)
		close(done)
	}()
	id, err := svc.Enqueue(ctx, "rescan", "vol1", "扫描视频库 · 电影", "")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := repo.List(context.Background(), 10)
		if len(jobs) == 1 && jobs[0].Status != StatusQueued {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	jobs, _ := repo.List(context.Background(), 10)
	if len(jobs) != 1 || jobs[0].Status != StatusFailed {
		t.Fatalf("job should be failed: %+v", jobs)
	}
	mu.Lock()
	defer mu.Unlock()
	if err := notified[id]; err == nil || err.Error() != "boom" {
		t.Fatalf("notify err = %v, want boom", err)
	}
}

func TestHasActive(t *testing.T) {
	repo := &memRepo{}
	svc := NewService(repo, NewLiveStatus())
	if active, _ := svc.HasActive(context.Background(), "rescan", "vol1"); active {
		t.Fatalf("active before enqueue = true")
	}
	if _, err := svc.Enqueue(context.Background(), "rescan", "vol1", "扫描视频库 · 电影", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if active, _ := svc.HasActive(context.Background(), "rescan", "vol1"); !active {
		t.Fatalf("active after enqueue = false")
	}
	if active, _ := svc.HasActive(context.Background(), "rescan", "vol2"); active {
		t.Fatalf("active for other target = true")
	}
}

type errBoom struct{}

func (e *errBoom) Error() string { return "boom" }
