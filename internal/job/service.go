package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Store interface {
	Create(ctx context.Context, j Job) (Job, error)
	Get(ctx context.Context, id string) (Job, error)
	List(ctx context.Context) ([]Job, error)
	MarkRunning(ctx context.Context, id string) (Job, error)
	MarkSucceeded(ctx context.Context, id, result string) (Job, error)
	MarkFailed(ctx context.Context, id string, state State, reason string) (Job, error)
}

type Metrics interface {
	JobCreated()
	JobStarted()
	JobCompleted(state State, duration time.Duration)
	JobRetried()
}

type Service struct {
	store     Store
	processor Processor
	metrics   Metrics
	logger    *slog.Logger
}

func NewService(store Store, processor Processor, metrics Metrics, logger *slog.Logger) *Service {
	return &Service{
		store:     store,
		processor: processor,
		metrics:   metrics,
		logger:    logger,
	}
}

func (s *Service) Submit(ctx context.Context, req CreateRequest) (Job, error) {
	req.Type = strings.TrimSpace(req.Type)
	if req.Type == "" {
		return Job{}, fmt.Errorf("%w: type is required", ErrInvalidJob)
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 3
	}
	if req.MaxAttempts > 10 {
		return Job{}, fmt.Errorf("%w: max_attempts must be <= 10", ErrInvalidJob)
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 5
	}
	if req.TimeoutSeconds > 60 {
		return Job{}, fmt.Errorf("%w: timeout_seconds must be <= 60", ErrInvalidJob)
	}

	now := time.Now().UTC()
	j := Job{
		ID:             newID(),
		Type:           req.Type,
		Payload:        req.Payload,
		State:          StateQueued,
		MaxAttempts:    req.MaxAttempts,
		Timeout:        time.Duration(req.TimeoutSeconds) * time.Second,
		TimeoutSeconds: req.TimeoutSeconds,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := s.store.Create(ctx, j)
	if err != nil {
		return Job{}, err
	}
	s.metrics.JobCreated()
	s.logger.Info("job submitted", "job_id", created.ID, "type", created.Type, "max_attempts", created.MaxAttempts)
	return created, nil
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Job, error) {
	return s.store.List(ctx)
}

func (s *Service) Execute(ctx context.Context, id string) bool {
	j, err := s.store.MarkRunning(ctx, id)
	if err != nil {
		s.logger.Warn("job could not start", "job_id", id, "error", err)
		return false
	}

	s.metrics.JobStarted()
	start := time.Now()
	logger := s.logger.With("job_id", j.ID, "attempt", j.Attempts, "type", j.Type)
	logger.Info("job started")

	runCtx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()

	result, err := s.processor.Process(runCtx, j)
	duration := time.Since(start)

	if err == nil {
		if _, storeErr := s.store.MarkSucceeded(context.Background(), j.ID, result); storeErr != nil {
			logger.Error("job succeeded but state update failed", "error", storeErr)
			return false
		}
		s.metrics.JobCompleted(StateSucceeded, duration)
		logger.Info("job succeeded", "duration_ms", duration.Milliseconds())
		return false
	}

	state := StateFailed
	reason := err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		state = StateTimedOut
		reason = "job timed out"
	}

	terminal := j.Attempts >= j.MaxAttempts || state == StateTimedOut
	nextState := StateQueued
	if terminal {
		nextState = state
	}

	updated, storeErr := s.store.MarkFailed(context.Background(), j.ID, nextState, reason)
	if storeErr != nil {
		logger.Error("job failed but state update failed", "error", storeErr)
		return false
	}

	if terminal {
		s.metrics.JobCompleted(updated.State, duration)
		logger.Warn("job finished unsuccessfully", "state", updated.State, "duration_ms", duration.Milliseconds(), "error", reason)
		return false
	}

	s.metrics.JobRetried()
	logger.Warn("job will be retried", "next_state", updated.State, "error", reason)
	return true
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
