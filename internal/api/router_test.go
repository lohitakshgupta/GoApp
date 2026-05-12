package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lohitakshgupta/GoApp/internal/api"
	"github.com/lohitakshgupta/GoApp/internal/job"
	"github.com/lohitakshgupta/GoApp/internal/observability"
)

func TestCreateJobReturnsAcceptedAndEnqueues(t *testing.T) {
	service := &fakeService{}
	queue := &fakeQueue{}
	router := api.NewRouter(api.Config{
		Service: service,
		Pool:    queue,
		Metrics: observability.NewMetrics(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{
		"type": "echo",
		"payload": {"message": "hello"},
		"max_attempts": 2,
		"timeout_seconds": 5
	}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if queue.enqueued != "job-1" {
		t.Fatalf("enqueued job = %q, want job-1", queue.enqueued)
	}

	var got job.Job
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "job-1" || got.State != job.StateQueued {
		t.Fatalf("unexpected job response: %+v", got)
	}
}

func TestGetMissingJobReturnsNotFound(t *testing.T) {
	router := api.NewRouter(api.Config{
		Service: &fakeService{},
		Pool:    &fakeQueue{},
		Metrics: observability.NewMetrics(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/missing", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

type fakeService struct {
	mu  sync.Mutex
	job job.Job
}

func (s *fakeService) Submit(_ context.Context, req job.CreateRequest) (job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.job = job.Job{
		ID:             "job-1",
		Type:           req.Type,
		Payload:        req.Payload,
		State:          job.StateQueued,
		MaxAttempts:    req.MaxAttempts,
		TimeoutSeconds: req.TimeoutSeconds,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return s.job, nil
}

func (s *fakeService) Get(_ context.Context, id string) (job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.ID != id {
		return job.Job{}, job.ErrJobNotFound
	}
	return s.job, nil
}

func (s *fakeService) List(context.Context) ([]job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.ID == "" {
		return nil, nil
	}
	return []job.Job{s.job}, nil
}

type fakeQueue struct {
	enqueued string
}

func (q *fakeQueue) Enqueue(_ context.Context, id string) error {
	q.enqueued = id
	return nil
}
