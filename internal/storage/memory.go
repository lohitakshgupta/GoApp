package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/lohitakshgupta/GoApp/internal/job"
)

type MemoryStore struct {
	mu  sync.RWMutex
	now func() time.Time
	db  map[string]job.Job
}

func NewMemoryStore(now func() time.Time) *MemoryStore {
	return &MemoryStore{
		now: now,
		db:  make(map[string]job.Job),
	}
}

func (s *MemoryStore) Create(ctx context.Context, j job.Job) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.db[j.ID]; exists {
		return job.Job{}, job.ErrStoreConflict
	}
	s.db[j.ID] = j
	return j, nil
}

func (s *MemoryStore) Get(ctx context.Context, id string) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	j, ok := s.db[id]
	if !ok {
		return job.Job{}, job.ErrJobNotFound
	}
	return j, nil
}

func (s *MemoryStore) List(ctx context.Context) ([]job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]job.Job, 0, len(s.db))
	for _, j := range s.db {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, k int) bool {
		return jobs[i].CreatedAt.After(jobs[k].CreatedAt)
	})
	return jobs, nil
}

func (s *MemoryStore) MarkRunning(ctx context.Context, id string) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.db[id]
	if !ok {
		return job.Job{}, job.ErrJobNotFound
	}
	if j.State != job.StateQueued {
		return job.Job{}, job.ErrStoreConflict
	}

	now := s.now().UTC()
	j.State = job.StateRunning
	j.Attempts++
	j.UpdatedAt = now
	j.StartedAt = &now
	s.db[id] = j
	return j, nil
}

func (s *MemoryStore) MarkSucceeded(ctx context.Context, id, result string) (job.Job, error) {
	return s.finish(ctx, id, job.StateSucceeded, result, "")
}

func (s *MemoryStore) MarkFailed(ctx context.Context, id string, state job.State, reason string) (job.Job, error) {
	return s.finish(ctx, id, state, "", reason)
}

func (s *MemoryStore) finish(ctx context.Context, id string, state job.State, result, reason string) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.db[id]
	if !ok {
		return job.Job{}, job.ErrJobNotFound
	}
	if j.State != job.StateRunning {
		return job.Job{}, job.ErrStoreConflict
	}

	now := s.now().UTC()
	j.State = state
	j.Result = result
	j.LastError = reason
	j.UpdatedAt = now
	if state == job.StateSucceeded || state == job.StateFailed || state == job.StateTimedOut {
		j.FinishedAt = &now
	}
	s.db[id] = j
	return j, nil
}
