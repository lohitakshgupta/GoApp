package worker

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lohitakshgupta/GoApp/internal/job"
)

type Executor interface {
	Execute(ctx context.Context, id string) bool
}

type Pool struct {
	workers  int
	executor Executor
	logger   *slog.Logger
	queue    chan string
	wg       sync.WaitGroup
}

func NewPool(workers int, executor Executor, logger *slog.Logger) *Pool {
	if workers <= 0 {
		workers = 1
	}
	return &Pool{
		workers:  workers,
		executor: executor,
		logger:   logger,
		queue:    make(chan string, workers*8),
	}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		workerID := i + 1
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.logger.Info("worker started", "worker_id", workerID)
			for {
				select {
				case <-ctx.Done():
					p.logger.Info("worker stopped", "worker_id", workerID)
					return
				case id := <-p.queue:
					if retry := p.executor.Execute(ctx, id); retry {
						if err := p.Enqueue(ctx, id); err != nil {
							p.logger.Error("failed to requeue job", "worker_id", workerID, "job_id", id, "error", err)
						}
					}
				}
			}
		}()
	}
}

func (p *Pool) Enqueue(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.queue <- id:
		return nil
	default:
		return job.ErrQueueUnavailable
	}
}

func (p *Pool) Stop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		p.logger.Warn("worker shutdown timed out")
	case <-done:
	}
}
