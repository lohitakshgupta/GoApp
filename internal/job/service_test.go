package job_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lohitakshgupta/GoApp/internal/job"
	"github.com/lohitakshgupta/GoApp/internal/observability"
	"github.com/lohitakshgupta/GoApp/internal/storage"
)

func TestServiceRetriesTransientFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewMemoryStore(time.Now)
	metrics := observability.NewMetrics()
	service := job.NewService(store, job.NewDemoProcessor(logger), metrics, logger)

	created, err := service.Submit(context.Background(), job.CreateRequest{
		Type:           "flaky",
		Payload:        []byte(`{"fail_until_attempt":1}`),
		MaxAttempts:    3,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if retry := service.Execute(context.Background(), created.ID); !retry {
		t.Fatal("expected first attempt to request retry")
	}

	afterFirstAttempt, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job after first attempt: %v", err)
	}
	if afterFirstAttempt.State != job.StateQueued {
		t.Fatalf("state after transient failure = %q, want %q", afterFirstAttempt.State, job.StateQueued)
	}
	if afterFirstAttempt.Attempts != 1 {
		t.Fatalf("attempts after first failure = %d, want 1", afterFirstAttempt.Attempts)
	}

	if retry := service.Execute(context.Background(), created.ID); retry {
		t.Fatal("did not expect retry after successful second attempt")
	}

	completed, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if completed.State != job.StateSucceeded {
		t.Fatalf("final state = %q, want %q", completed.State, job.StateSucceeded)
	}
	if completed.Result != "recovered after retry" {
		t.Fatalf("result = %q", completed.Result)
	}
}

func TestServiceMarksTimedOutJobsTerminal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewMemoryStore(time.Now)
	metrics := observability.NewMetrics()
	service := job.NewService(store, blockingProcessor{}, metrics, logger)

	created, err := service.Submit(context.Background(), job.CreateRequest{
		Type:           "slow",
		MaxAttempts:    3,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if retry := service.Execute(context.Background(), created.ID); retry {
		t.Fatal("timed-out job should not retry")
	}

	completed, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get timed-out job: %v", err)
	}
	if completed.State != job.StateTimedOut {
		t.Fatalf("state = %q, want %q", completed.State, job.StateTimedOut)
	}
	if completed.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", completed.Attempts)
	}
}

type blockingProcessor struct{}

func (blockingProcessor) Process(ctx context.Context, _ job.Job) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
