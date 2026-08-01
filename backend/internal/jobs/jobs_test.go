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

func (m *memRepo) MarkDone(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		if m.jobs[i].ID == id {
			m.jobs[i].Status = StatusDone
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

func (m *memRepo) ResetRunning(_ context.Context) error { return nil }

func TestWorkerProcessesJobs(t *testing.T) {
	repo := &memRepo{}
	svc := NewService(repo)

	var mu sync.Mutex
	processed := map[string]bool{}
	handler := func(_ context.Context, j Job) error {
		mu.Lock()
		processed[j.Type] = true
		mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker := NewWorker(repo, handler, 2)
		worker.Run(ctx)
		close(done)
	}()

	if _, err := svc.Enqueue(ctx, "probe", "t1", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Enqueue(ctx, "probe", "t2", ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Enqueue(ctx, "thumbnail", "t3", ""); err != nil {
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

func TestWorkerMarksFailed(t *testing.T) {
	repo := &memRepo{}
	svc := NewService(repo)
	handler := func(_ context.Context, j Job) error {
		return &errBoom{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewWorker(repo, handler, 1).Run(ctx)
		close(done)
	}()
	if _, err := svc.Enqueue(ctx, "probe", "t", ""); err != nil {
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
}

type errBoom struct{}

func (e *errBoom) Error() string { return "boom" }
