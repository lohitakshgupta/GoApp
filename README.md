# GoApp: Job Runner Service

GoApp is a small Go backend service that accepts jobs through an HTTP API, processes them asynchronously with a bounded worker pool, tracks job state, retries transient failures, enforces execution timeouts, and exposes health and metrics endpoints.

The goal of the project is to demonstrate practical backend/platform engineering judgment in a compact codebase: idiomatic Go, clear package boundaries, intentional concurrency, graceful shutdown, structured logging, testable service logic, and production-minded tradeoffs without turning the repository into a large framework exercise.

## What It Demonstrates

- HTTP API design using the Go standard library.
- Microservice-style layering between handlers, domain logic, workers, storage, and observability.
- Worker-pool concurrency using goroutines and a bounded channel queue.
- Per-job timeout handling with `context.Context`.
- Retry behavior for transient failures.
- Graceful shutdown on `SIGINT` and `SIGTERM`.
- Structured JSON logging with `log/slog`.
- Prometheus-style metrics output.
- Realistic tests for API behavior and job state transitions.
- Docker and GitHub Actions CI for repeatable build and validation.

## Architecture

```text
client
  |
  v
HTTP API
  |
  | create/list/get jobs
  v
job.Service  <---->  storage.Store
  |
  | enqueue job ID
  v
worker.Pool
  |
  | execute with context timeout
  v
job.Processor
```

The service stores jobs in an in-memory repository for simplicity. The storage boundary is intentionally hidden behind a small `job.Store` interface so a durable implementation, such as Postgres or Redis, can be introduced without changing the HTTP or worker packages.

## Repository Structure

```text
.
├── cmd/server                 # Process entrypoint and runtime wiring
├── internal/api               # HTTP routing, JSON handlers, error responses
├── internal/job               # Domain model, orchestration, retries, timeouts
├── internal/observability     # Lightweight metrics collector
├── internal/storage           # Concurrency-safe in-memory store
├── internal/worker            # Bounded queue and worker pool
├── .github/workflows/ci.yml   # GitHub Actions CI
├── Dockerfile                 # Multi-stage container build
├── Makefile                   # Common local commands
└── go.mod
```

## Requirements

- Go 1.22+
- Docker, optional
- Make, optional but convenient

## Build And Run

Run locally:

```sh
make run
```

Or without `make`:

```sh
go run ./cmd/server
```

Build a local binary:

```sh
go build -o bin/job-runner ./cmd/server
./bin/job-runner
```

The service listens on `:8080` by default.

## Configuration

Configuration is environment-variable based:

| Variable | Default | Description |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `WORKERS` | `4` | Number of worker goroutines |
| `SHUTDOWN_TIMEOUT_SECONDS` | `10` | Shutdown grace period |

Example:

```sh
ADDR=:9000 WORKERS=8 make run
```

## API Usage

Create an `echo` job:

```sh
curl -i localhost:8080/v1/jobs \
  -H 'content-type: application/json' \
  -d '{
    "type": "echo",
    "payload": {"message": "hello from GoApp"},
    "max_attempts": 3,
    "timeout_seconds": 5
  }'
```

Create a flaky job that fails once and then succeeds on retry:

```sh
curl -i localhost:8080/v1/jobs \
  -H 'content-type: application/json' \
  -d '{
    "type": "flaky",
    "payload": {"fail_until_attempt": 1},
    "max_attempts": 3,
    "timeout_seconds": 5
  }'
```

Create a job that demonstrates timeout handling:

```sh
curl -i localhost:8080/v1/jobs \
  -H 'content-type: application/json' \
  -d '{
    "type": "sleep",
    "payload": {"duration_ms": 3000},
    "max_attempts": 3,
    "timeout_seconds": 1
  }'
```

List jobs:

```sh
curl localhost:8080/v1/jobs
```

Get one job:

```sh
curl localhost:8080/v1/jobs/{job_id}
```

Health and readiness:

```sh
curl localhost:8080/healthz
curl localhost:8080/readyz
```

Metrics:

```sh
curl localhost:8080/metrics
```

## Supported Demo Job Types

| Type | Behavior |
| --- | --- |
| `echo` | Returns `payload.message`, or `ok` if no message is provided |
| `sleep` | Sleeps for `payload.duration_ms` while honoring context cancellation |
| `flaky` | Fails until `payload.fail_until_attempt`, then succeeds |
| `fail` | Always fails, useful for observing retry exhaustion |

## Job State Flow

```text
queued -> running -> succeeded
queued -> running -> queued -> running -> succeeded
queued -> running -> failed
queued -> running -> timed_out
```

Jobs begin in `queued`, move to `running` when a worker starts processing, and finish as `succeeded`, `failed`, or `timed_out`. A non-terminal failure moves back to `queued` so it can be retried by the worker pool.

## Testing

Run unit tests:

```sh
make test
```

Run tests with the race detector:

```sh
make race
```

Run formatting and static checks:

```sh
make fmt
make vet
```

CI runs:

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

## Docker

Build the image:

```sh
make docker-build
```

Run it:

```sh
docker run --rm -p 8080:8080 job-runner-service:local
```

The Dockerfile uses a multi-stage build and runs the final static binary as a non-root user.

## Design Tradeoffs

- In-memory storage keeps the project focused on service structure and concurrency. A real deployment would use durable storage and likely a durable queue.
- The worker queue carries job IDs instead of full job payloads so storage remains the source of truth.
- The queue is bounded to create backpressure instead of allowing unbounded memory growth.
- Retries are explicit and visible in state transitions. Timeout failures are terminal because retrying a deterministic timeout can amplify load.
- State updates after execution use a fresh background context so the service can still record job outcomes if a client disconnects.
- Metrics are intentionally lightweight Prometheus-style text. A production service would likely use OpenTelemetry and richer latency histograms.
- The demo processor does not execute shell commands or arbitrary user code. It is safe by design while still demonstrating asynchronous execution, retries, and cancellation.

## Production Extensions

Natural next steps for a larger version of this service would include:

- Durable job storage.
- Persistent queue or stream processing.
- Idempotency keys for job submission.
- Dead-letter handling.
- OpenTelemetry traces and histograms.
- Authentication and authorization.
- Deployment manifests and autoscaling signals.
