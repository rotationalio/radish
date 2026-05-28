# Radish

[![GoDoc](https://godoc.org/go.rtnl.ai/radish?status.svg)](https://godoc.org/go.rtnl.ai/radish)
[![Go Report Card](https://goreportcard.com/badge/github.com/rotationalio/radish)](https://goreportcard.com/report/github.com/rotationalio/radish)
[![CI Tests](https://github.com/rotationalio/radish/actions/workflows/tests.yaml/badge.svg)](https://github.com/rotationalio/radish/actions/workflows/tests.yaml)

Radish is a lightweight, type-safe, persistent background task queue for Go. Tasks
are durably stored in a relational database (PostgreSQL or SQLite) so that they
survive process restarts, are processed exactly-once with at-least-once delivery
semantics, and can be retried with configurable backoff. Radish is designed to be
embedded directly into your Go application - there is no separate broker process
to deploy or manage.

- **Persistent** - tasks are stored in your database and survive crashes.
- **Type-safe** - workers and tasks are strongly typed via Go generics.
- **Embedded** - runs in-process with your application; no separate broker.
- **Retryable** - configurable backoff (zero, constant, linear, exponential) with
  optional jitter, per-task and per-worker retry overrides.
- **Schedulable** - enqueue tasks for immediate execution or schedule them for
  later.
- **Concurrent** - configurable pool of executors poll for and run tasks in
  parallel.
- **Pluggable storage** - PostgreSQL for production, SQLite for testing or single-node
  deployments, mock for unit tests.

## Quick Start

Install Radish:

```bash
go get go.rtnl.ai/radish
```

A complete working example with a single task type and worker:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "go.rtnl.ai/radish"
)

// 1. Define a task. The struct is JSON-serialized into the queue.
type GreetTask struct {
    Name string `json:"name"`
}

func (t *GreetTask) Kind() string { return "greet" }

// 2. Define a worker. Embed WorkerDefaults to inherit default Retry/Timeout.
type GreetWorker struct {
    radish.WorkerDefaults[*GreetTask]
}

func (w *GreetWorker) Do(ctx context.Context, task *radish.TaskInfo[*GreetTask]) error {
    fmt.Printf("hello, %s!\n", task.Task.Name)
    return nil
}

func main() {
    // 3. Configure Radish (env vars also work; see Configuration below).
    tasks, err := radish.New(&radish.Config{
        DatabaseURL: "postgres://radish@localhost:5432/radish?sslmode=disable",
        NumWorkers:  4,
        TaskTimeout: 30 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }

    // 4. Register your worker(s).
    if err := radish.Register(tasks, new(GreetWorker)); err != nil {
        log.Fatal(err)
    }

    // 5. Start the executor pool.
    if err := tasks.Run(); err != nil {
        log.Fatal(err)
    }
    defer tasks.Shutdown()

    // 6. Enqueue tasks from anywhere in your application.
    if _, err := tasks.Enqueue(context.Background(), &GreetTask{Name: "world"}); err != nil {
        log.Fatal(err)
    }

    // Block until shutdown.
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit
}
```

That's it. Run the program, and the executor pool will pick up the `greet` task
and execute it. If the worker returns an error, Radish will retry the task
according to the configured backoff policy.

---

## Table of Contents

- [Concepts](#concepts)
- [Installation](#installation)
- [Configuration](#configuration)
- [Designing Tasks](#designing-tasks)
- [Designing Workers](#designing-workers)
- [Embedding Radish](#embedding-radish)
- [Enqueueing and Scheduling Tasks](#enqueueing-and-scheduling-tasks)
- [Retries and Backoff](#retries-and-backoff)
- [Brokers and Storage](#brokers-and-storage)
- [Operations](#operations)
- [Developer Guide](#developer-guide)

---

## Concepts

A few terms used throughout the documentation:

- **Task** - a Go struct (implementing `radish.Task`) that describes a unit of
  work. It is JSON-serialized and persisted to the database. The task `Kind()`
  routes the payload to the correct worker.
- **Worker** - a Go type (implementing `radish.Worker[T]`) that processes tasks
  of a specific type `T`. Each task `Kind` is handled by exactly one worker.
- **TaskInfo** - the runtime view a worker sees: the deserialized task `T` plus
  metadata (id, attempts, errors, timestamps, etc).
- **Broker** - the persistent storage backend that holds tasks. PostgreSQL,
  SQLite, and an in-memory mock are provided.
- **Executor** - a goroutine that polls the broker for ready tasks and dispatches
  them to workers. The executor pool size is configured by `NumWorkers`.
- **Radish** - the top-level controller: holds configuration, the worker
  registry, and the executor pool.

## Installation

```bash
go get go.rtnl.ai/radish
```

Radish requires Go 1.26 or later and a PostgreSQL database for production use.
SQLite is supported for development and single-node deployments. The schema is
created automatically the first time Radish connects.

NOTE: other brokers such as redis and queues like Kafka and Pulsar may be available in the future.

## Configuration

`radish.Config` controls all of Radish's behavior. It can be constructed
explicitly or loaded from the environment using
[`confire`](https://pkg.go.dev/go.rtnl.ai/confire).

### Configuration fields

| Field            | Env Var                  | Default  | Description                                                                                |
|------------------|--------------------------|----------|--------------------------------------------------------------------------------------------|
| `DatabaseURL`    | `DATABASE_URL`           |          | DSN to the database (e.g. `postgres://...`, `sqlite3:///path/to/db.sqlite`).               |
| `ManagedDB`      | `RADISH_MANAGED_DB`      | `false`  | If true, supply your own `*sql.DB` via `Conn` instead of letting Radish connect.           |
| `NumWorkers`     | `RADISH_NUM_WORKERS`     | `8`      | Number of executor goroutines polling for tasks.                                           |
| `TaskRetries`    | `RADISH_TASK_RETRIES`    | `3`      | Maximum default retry attempts per task.                                                   |
| `TaskTimeout`    | `RADISH_TASK_TIMEOUT`    | `60s`    | Hard timeout per task; if exceeded, the task is reclaimed and retried.                     |
| `PollInterval`   | `RADISH_POLL_INTERVAL`   | `5s`     | How often each executor polls for new work.                                                |
| `PollJitter`     | `RADISH_POLL_JITTER`     | `125ms`  | Standard deviation of jitter applied to the poll interval to avoid thundering herds.       |
| `Retention`      | `RADISH_RETENTION`       | `24h`    | How long completed/failed tasks are retained when calling `Vacuum`.                        |
| `VacuumInterval` | `RADISH_VACUUM_INTERVAL` | `3h`     | How often the `Vacuum` background task is executed to clean up the database.               |
| `Backoff`        | `RADISH_BACKOFF_*`       | (linear) | Backoff policy; see [Retries and Backoff](#retries-and-backoff).                           |
| `Conn`           | (not env-loadable)       | `nil`    | An existing `*sql.DB` to use instead of opening a new one. Required when `ManagedDB=true`. |

### Loading from the environment

```go
tasks, err := radish.New(nil) // nil triggers env-based config loading
```

When `nil` is passed to `radish.New`, the config is loaded from environment
variables using the `RADISH_` prefix (with `DATABASE_URL` as the one exception).
Set them in your shell, `.env`, or process manager:

```bash
export DATABASE_URL=postgres://radish@localhost:5432/radish?sslmode=disable
export RADISH_NUM_WORKERS=8
export RADISH_TASK_RETRIES=3
export RADISH_TASK_TIMEOUT=120s
export RADISH_POLL_INTERVAL=15s
export RADISH_POLL_JITTER=1250ms
export RADISH_RETENTION=72h
export RADISH_BACKOFF_POLICY=exponential
export RADISH_BACKOFF_DELAY=16s
export RADISH_BACKOFF_FACTOR=2.0
export RADISH_BACKOFF_JITTER=true
export RADISH_BACKOFF_SIGMA=1500ms
```

### Constructing a config in code

```go
cfg := &radish.Config{
    DatabaseURL:  "postgres://radish@localhost:5432/radish?sslmode=disable",
    NumWorkers:   8,
    TaskRetries:  5,
    TaskTimeout:  2 * time.Minute,
    PollInterval: 10 * time.Second,
    PollJitter:   500 * time.Millisecond,
    Retention:    72 * time.Hour,
    Backoff: backoff.Config{
        Policy: backoff.PolicyExponential,
        Delay:  10 * time.Second,
        Factor: 2.0,
        Jitter: true,
        Sigma:  750 * time.Millisecond,
    },
}
tasks, err := radish.New(cfg)
```

### Sharing an existing database connection

If your application already manages a `*sql.DB`, you can let Radish use it
instead of opening its own connection by setting `ManagedDB: true` and providing
`Conn`:

```go
db, _ := sql.Open("postgres", dsn)
tasks, err := radish.New(&radish.Config{
    ManagedDB: true,
    Conn:      db,
})
```

## Designing Tasks

A task is any Go struct that implements `radish.Task`:

```go
type Task interface {
    Kind() string
}
```

Some guidance for designing tasks:

### Use a unique, stable Kind

The `Kind()` is the routing key that maps a serialized payload to a worker.
Kinds should be lowercase, alphanumeric, with no spaces or special characters
(dashes are fine). For example: `send-email`, `index-document`, `report.daily`.

> **Important:** Kind values must be **static**, not dynamic. The worker registry
> instantiates a zero-valued task and calls `Kind()` to learn which kind a worker
> is responsible for. If `Kind()` reads fields from the struct, it will return
> the wrong value at registration time.

```go
// GOOD - constant, returned for any zero value of the type.
func (t *SendEmail) Kind() string { return "send-email" }

// BAD - kind is derived from struct fields and won't match at registration.
func (t *SendEmail) Kind() string { return "email-" + t.Provider }
```

### Renaming tasks safely

After tasks have been deployed and persisted, you cannot simply rename a kind
without orphaning queued tasks. To rename safely, implement
`radish.TaskWithAliases` so the worker continues to handle previous kinds:

```go
type SortTask struct {
    Numbers []int `json:"numbers"`
}

func (t *SortTask) Kind() string { return "sort" }

func (t *SortTask) KindAliases() []string {
    return []string{"sort-numbers", "sort-integers"}
}
```

The worker registered for `SortTask` will receive any task with kind `sort`,
`sort-numbers`, or `sort-integers`.

### Payloads are JSON

Tasks are serialized with `encoding/json`, so:

- Exported fields are marshaled; unexported fields are not.
- `json:"..."` struct tags are honored.
- Avoid putting non-serializable values (channels, funcs, `*sql.DB`, etc.) in
  the task struct - only put what the worker needs to recreate context, and let
  the worker fetch the rest from your application's services.

A complete example:

```go
type SendEmailTask struct {
    To       string `json:"to"`
    Subject  string `json:"subject"`
    Template string `json:"template"`
    Data     map[string]any `json:"data,omitempty"`
}

func (t *SendEmailTask) Kind() string { return "send-email" }
```

## Designing Workers

A worker implements `radish.Worker[T]` for a specific task type `T`:

```go
type Worker[T Task] interface {
    // Determines whether the failed task should be retried, and if so, how
    // long to wait. Return nil to fall back to the default retry policy.
    Retry(*TaskInfo[T]) *Retry

    // Per-task timeout. Return 0 to use the configured TaskTimeout. Values
    // greater than the configured TaskTimeout are clamped down.
    Timeout(*TaskInfo[T]) time.Duration

    // Performs the work. Returns nil for success, error to mark failure.
    Do(context.Context, *TaskInfo[T]) error
}
```

### Embedding `WorkerDefaults`

Most workers only need to implement `Do`. Embed `radish.WorkerDefaults[T]` to get
sensible no-op implementations of `Retry` and `Timeout`:

```go
type SendEmailWorker struct {
    radish.WorkerDefaults[*SendEmailTask]
    smtp *smtp.Client
}

func (w *SendEmailWorker) Do(ctx context.Context, task *radish.TaskInfo[*SendEmailTask]) error {
    return w.smtp.Send(ctx, task.Task.To, task.Task.Subject, task.Task.Template, task.Task.Data)
}
```

### Quick workers from functions

For trivial workers, `radish.WorkFunc` wraps a function as a worker:

```go
worker := radish.WorkFunc(func(ctx context.Context, task *radish.TaskInfo[*GreetTask]) error {
    fmt.Println("hello,", task.Task.Name)
    return nil
})

radish.Register(tasks, worker)
```

### Custom retry behavior

Override `Retry` to react to specific errors or to the task's history:

```go
func (w *SendEmailWorker) Retry(info *radish.TaskInfo[*SendEmailTask]) *radish.Retry {
    // Retry up to 10 times for this worker.
    if info.Attempts >= 10 {
        return &radish.Retry{Retry: false}
    }
    return &radish.Retry{Retry: true, Delay: time.Duration(info.Attempts) * 30 * time.Second}
}
```

Returning `nil` (the default) means "use the global retry policy from `Config.Backoff`
and `Config.TaskRetries`". A returned `Retry.Delay < 0` also falls back to the
default backoff delay.

### Custom timeouts

Override `Timeout` to give certain tasks more or less time to run:

```go
func (w *RenderReportWorker) Timeout(info *radish.TaskInfo[*RenderReportTask]) time.Duration {
    if info.Task.Large {
        return 10 * time.Minute
    }
    return 30 * time.Second
}
```

If the returned duration exceeds the global `TaskTimeout`, it is clamped to
`TaskTimeout` (which is also the broker visibility lease - exceeding it would
let another executor double-process the task).

### Worker concurrency

Workers' `Do` methods are called from multiple goroutines concurrently (one per
executor). Workers must be safe for concurrent use - guard mutable state with
mutexes or design workers as stateless processors that pull dependencies from
injected services.

## Embedding Radish

The Radish lifecycle has three phases:

1. **Construct** with `radish.New`.
2. **Register** workers (must happen before `Run`).
3. **Run** the executor pool, then `Shutdown` cleanly when finished.

```go
tasks, err := radish.New(cfg)
if err != nil { return err }

if err := radish.Register(tasks, new(SendEmailWorker)); err != nil { return err }
if err := radish.Register(tasks, new(IndexDocumentWorker)); err != nil { return err }

if err := tasks.Run(); err != nil { return err }
defer tasks.Shutdown()

// ... your application runs ...
```

Useful methods to know:

- `radish.Register(tasks, worker)` - typed registration. Returns
  `ErrRunning` if called after `Run`.
- `radish.MustRegister(tasks, worker)` - same, but panics on error.
- `tasks.Run()` - starts `NumWorkers` executor goroutines. Returns
  `ErrRunning` if already running.
- `tasks.Shutdown()` - signals all executors to stop, waits for in-flight
  tasks to finish (subject to their timeouts), and closes the broker.
- `tasks.IsRunning()` - reports whether the executor pool is active.

`Shutdown` is **graceful**: it waits for in-flight tasks to either complete or
hit their timeout before returning. Tasks still in the queue remain there and
will be picked up by the next process that runs.

## Enqueueing and Scheduling Tasks

Once Radish is running, your application enqueues tasks through it:

```go
// Run as soon as an executor is available.
id, err := tasks.Enqueue(ctx, &SendEmailTask{To: "ada@example.com", Subject: "hi"})

// Run no earlier than a specific time.
id, err := tasks.Schedule(ctx, &ReportTask{Date: today}, time.Now().Add(1*time.Hour))
```

Both methods return the broker-assigned `int64` task id, which can be used to
inspect or cancel the task later.

### Inspecting tasks

```go
meta, err := tasks.Info(ctx, id)
fmt.Println(meta.Status, meta.Attempts, meta.Errors)
```

`meta` is a `*models.TaskMeta` containing the kind, status, payload, attempt
count, accumulated errors, and timestamps.

### Cancelling tasks

```go
if err := tasks.Cancel(ctx, id); err != nil { /* ... */ }
```

Cancellation marks the task as cancelled in the broker so it will not be picked
up by an executor. Cancelling a task that is already running has no effect on
the in-flight execution.

### Vacuuming completed tasks

Successfully completed and permanently failed tasks are kept in the database for
auditing. Periodically call `Vacuum` to reclaim space:

```go
if err := tasks.Vacuum(ctx, 72*time.Hour); err != nil { /* ... */ }
```

This deletes tasks in a terminal state (`succeeded`, `failed`, `cancelled`) that
finished more than `retention` ago. A common pattern is to run a daily Radish
task that vacuums the queue.

## Retries and Backoff

When a worker's `Do` returns an error, Radish:

1. Adds the error to the task's `Errors` history (with attempt number and
   timestamp).
2. Asks the worker for a `Retry` decision via `Worker.Retry(info)`.
3. If the worker returns `nil`, it consults the configured retry policy:
   - if `Attempts < TaskRetries`, the task is retried;
   - otherwise it is marked `failed`.
4. The retry delay comes from the worker's `Retry.Delay` if positive; otherwise
   from the configured `Backoff` policy.

### Backoff policies

Configure the backoff via `radish.Config.Backoff` (or `RADISH_BACKOFF_*` env
vars):

| Policy        | Delay formula                              | Required fields              |
| ------------- | ------------------------------------------ | ---------------------------- |
| `zero`        | `0` (retry immediately)                    | (none)                       |
| `constant`    | `Delay`                                    | `Delay`                      |
| `linear`      | `Delay * Attempts`                         | `Delay`                      |
| `exponential` | `Delay * Factor^Attempts`                  | `Delay`, `Factor`            |

Setting `Jitter: true` wraps any policy in a normal distribution centered on
the computed delay with standard deviation `Sigma`, which helps avoid coordinated
thundering herds across executors.

```go
backoff.Config{
    Policy: backoff.PolicyExponential,
    Delay:  10 * time.Second,
    Factor: 2.0,
    Jitter: true,
    Sigma:  750 * time.Millisecond,
}
```

## Brokers and Storage

Radish ships with three broker implementations selected automatically from the
DSN scheme:

- **PostgreSQL** (`postgres://...`) - the recommended production backend. Uses
  row-level locking and `SKIP LOCKED` for safe multi-process dequeuing.
- **SQLite** (`sqlite3://...`) - good for development, tests, and single-node
  deployments.
- **Mock** (`mock://...`) - in-memory, for unit tests of code that uses Radish.

The schema (`radish_tasks` table and `radish_status` enum) is created
automatically the first time the broker connects, guarded by an advisory lock so
multiple processes can safely race.

### Task statuses

A task moves through these statuses (`go.rtnl.ai/radish/status`):

- `pending` - waiting in the queue, available for dequeue.
- `scheduled` - waiting for `VisibleAt` to elapse.
- `running` - leased by an executor and being worked.
- `retry` - failed and waiting for the retry delay to elapse.
- `succeeded` - terminal: worker returned `nil`.
- `failed` - terminal: worker exhausted retries or returned a non-retryable
  error.
- `cancelled` - terminal: cancelled via `Cancel(ctx, id)`.

## Operations

### Logging

Radish logs through [`go.rtnl.ai/x/rlog`](https://pkg.go.dev/go.rtnl.ai/x/rlog),
which wraps the standard `slog` package. Configure your application's `slog`
handler to control output format and level - Radish's logs will follow.

### Multiple Radish instances

You can run multiple Radish processes against the same database; the dequeue
query uses `FOR UPDATE SKIP LOCKED` (PostgreSQL) to ensure each task is leased
to exactly one executor at a time. This is the recommended way to scale
horizontally.

### Worker isolation

Each Radish process must register the same set of workers (or at least know
about the kinds it might dequeue). If an executor dequeues a task with an
unknown kind, it logs an error and leaves the task in the queue for another
process to handle.

### Migration / schema

The schema is initialized on first connect. There is currently no separate
migration tool - if you need to alter the schema, do so via your application's
own migration framework against the same database.

---

## Developer Guide

This section is for contributors working on Radish itself.

### Project layout

```
radish/
├── radish.go            # Top-level controller: New, Run, Shutdown, Enqueue, etc.
├── worker.go            # Worker[T] interface, WorkerDefaults, Workers registry.
├── task.go              # Task / TaskWithAliases interfaces and TaskInfo[T].
├── wrapper.go           # Generic ↔ untyped worker bridge (Factory pattern).
├── config.go            # radish.Config and env loading.
├── backoff/             # Retry-delay policies.
├── jitter/              # Normally-distributed jitter ticker.
├── status/              # Task status enum + JSON/SQL marshaling.
├── models/              # TaskMeta and AttemptError persistence types.
├── broker/
│   ├── broker.go        # Broker interface + DSN-based Connect dispatch.
│   ├── postgres/        # PostgreSQL broker implementation.
│   ├── sqlite/          # SQLite broker implementation.
│   ├── mock/            # In-memory broker for testing.
│   ├── tests/           # Cross-broker conformance suite.
│   └── errors/          # Shared broker errors (ErrNotFound, etc).
├── internal/worker/     # Untyped worker.Worker / Factory used by the executor.
└── cmd/turnip/          # Integration testing harness (see below).
```

### Running tests

```bash
# All tests, with race detection.
go test -race ./...

# Unit tests only (skip the postgres-backed broker tests).
go test -short ./...
```

CI runs against PostgreSQL 18 with `DATABASE_URL` configured to a test database.
To run the full suite locally, start a Postgres container and export the same
variable:

```bash
docker run --rm -d -p 5432:5432 \
  -e POSTGRES_USER=radish \
  -e POSTGRES_PASSWORD=turnip42 \
  -e POSTGRES_DB=radish_test \
  postgres:18

export DATABASE_URL=postgres://radish:turnip42@localhost:5432/radish_test?sslmode=disable
go test -race ./...
```

### Linting

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...
```

### Integration testing with `turnip`

`cmd/turnip` is a synthetic load generator and integration harness that:

- registers a generic `Basic` worker that sleeps for a configurable duration and
  fails with a configurable probability;
- enqueues / schedules tasks at a configurable rate using a JSON simulator
  configuration.

Two convenience scripts live in `fixtures/`:

- `fixtures/simulate.sh` exports a sample environment and runs `turnip`.
- `fixtures/simulators.json` describes the workload simulators.

Quick run (Radish only, no synthetic load):

```bash
cd fixtures
./simulate.sh
```

Run with simulated load:

```bash
cd fixtures
go run ../cmd/turnip/ -path simulators.json -log turnip.log
```

### Adding a new broker

1. Create a new package under `broker/<name>/` exposing a struct that implements
   the `broker.Broker` interface (see `broker/broker.go`).
2. Add a `Connect(uri *dsn.DSN)` constructor.
3. Register the new provider in `broker.Connect` (in `broker/broker.go`).
4. Implement the cross-broker conformance tests in `broker/tests/` against your
   new broker.

### Pull request guidelines

- Open a PR using the template in `.github/pull_request_template.md`.
- Add or update tests for any change to behavior.
- Run `go test -race ./...` and `staticcheck ./...` before pushing.
- Update the documentation in this README if you change public API or add a new
  configuration knob.
- Keep PRs focused and small; prefer multiple reviewable PRs over one large one.

### Versioning

Radish follows [Semantic Versioning](https://semver.org). The current version
constants live in `version.go` and are exposed via `radish.Version()`. Build
metadata (git sha, build date) can be injected via `-ldflags` at release time.

### License

See [LICENSE.txt](LICENSE.txt).
