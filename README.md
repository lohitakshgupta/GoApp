# Job Runner Service

A compact Go backend service that accepts jobs over HTTP, processes them asynchronously with a worker pool, tracks state transitions, retries transient failures, enforces per-job timeouts, and exposes health and metrics endpoints.

The project is intentionally small, but it is structured like a service owned by a platform or distributed systems team: the HTTP layer, orchestration logic, storage, worker execution, and observability concerns are separate and testable.

## Architecture

```text
client
  |
  v
HTTP API ── submit/list/get ──> job.Service ──> storage.Store
  |                                |
  |                                v
  └──────── enqueue job id ──> worker.Pool ──> job.Processor
                                      |
                                      v
                           retries, timeouts, state updates
```

### Packages

- `cmd/server`: process entrypoint, configuration, structured logger, graceful shutdown.
- `internal/api`: HTTP routing and JSON request/response handling.
- `internal/job`: domain model, orchestration, retry and timeout behavior.
- `internal/worker`: bounded channel queue and worker pool.
- `internal/storage`: concurrency-safe in-memory storage implementation.
- `internal/observability`: lightweight Prometheus-style counters.

## API

Start the service:

```sh
make run
```

Create a job:

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

Supported demo job types:

- `echo`: returns `payload.message`.
- `sleep`: waits for `payload.duration_ms`, honoring context cancellation.
- `flaky`: fails until `payload.fail_until_attempt`, then succeeds.
- `fail`: always fails, useful for observing retry exhaustion.

Other endpoints:

```sh
curl localhost:8080/v1/jobs
curl localhost:8080/v1/jobs/{id}
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics
```

## Design Tradeoffs

- Storage is in-memory to keep the repository focused on service design. The `job.Store` interface is deliberately small, so replacing it with Postgres, Redis, or a durable queue-backed store would not affect HTTP handlers or workers.
- The queue carries job IDs rather than full payloads. This keeps worker messages small and makes state the source of truth.
- Retries are explicit: failed non-terminal jobs move back to `queued`, and the worker pool re-enqueues them. Timeout failures are terminal because retrying a deterministic timeout usually increases pressure on the system.
- Contexts are used at request boundaries, worker shutdown boundaries, and per-job execution boundaries. State updates use a fresh background context after execution so job outcomes can still be recorded even when a client disconnects.
- The metrics endpoint is intentionally simple Prometheus text. A production service would likely use OpenTelemetry, histograms, and labels with tighter cardinality controls.
- The demo processor avoids executing shell commands. This keeps the sample safe while still demonstrating asynchronous work, retries, and cancellation.

## Production-Minded Details

- Structured JSON logging via `log/slog`.
- Bounded channel queue to provide backpressure.
- Worker pool with graceful shutdown on `SIGINT`/`SIGTERM`.
- Race-testable storage guarded by a mutex.
- Clear package boundaries and small interfaces where they improve testability.
- Docker image built as a static binary and run as a non-root user.

## Development

```sh
make fmt
make vet
make test
make race
make docker-build
```

Configuration is environment based:

| Variable | Default | Description |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `WORKERS` | `4` | Number of worker goroutines |
| `SHUTDOWN_TIMEOUT_SECONDS` | `10` | Graceful shutdown budget |

## Example State Flow

```text
queued -> running -> succeeded
queued -> running -> queued -> running -> succeeded
queued -> running -> failed
queued -> running -> timed_out
```
