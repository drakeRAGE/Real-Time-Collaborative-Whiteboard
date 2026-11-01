---
name: go-microservices-standards
description: >-
  Go coding standards for this repo's microservices (gateway, room, board,
  chat, presence) and shared pkg/ libraries. Use when writing, reviewing, or
  refactoring any Go code: structure, errors, concurrency, NATS/gRPC
  conventions, testing, and the quality gates required before a task is done.
---

# Go Microservices Standards

## Layout

- Services: `services/<name>/cmd/main.go` (wiring only: config → logger → otel → deps → servers) + `internal/{grpc|service|repo|ws|authz}/`. No service imports another service's `internal/`.
- Shared code in `pkg/` only when used by 2+ services. Constructor `New(deps...)` returning an interface. No package-level mutable state, no `init()` side effects.
- Ports convention: gRPC 700X, health HTTP 800X (room 7001/8001, board 7002/8002, chat 7003/8003, presence 7004/8004, gateway REST+WS 8080).

## Code rules

- `context.Context` first param for all I/O; honor cancellation everywhere.
- Errors: wrap with `fmt.Errorf("doing x: %w", err)`; sentinel errors (`ErrNotFound`) at package level; map to gRPC codes only in the transport layer (`NotFound`, `PermissionDenied`, `InvalidArgument`, `FailedPrecondition`).
- No `panic` outside `main`. No `os.Getenv` outside config structs loaded via `pkg/config`.
- Concurrency: every goroutine has a documented owner and shutdown path; channels bounded; gateway hub uses per-connection goroutine ownership (read pump / write pump over buffered chan).
- SQL only via sqlc-generated queries — never string concatenation.
- NATS: publish with `Nats-Msg-Id` (ULID) for idempotency; consumers persist THEN ack — never ack before the DB transaction commits; poison messages go to `wb.dlq.<orig>` after max-deliver.
- Authorization: principal comes ONLY from verified JWT (gateway) or gRPC metadata propagated by `pkg/grpcx` — never from client-supplied payload fields.

## Testing

- Table-driven unit tests next to code. Integration tests behind `-tags=integration` using testcontainers-go (postgres:16, redis:7, nats:2.10).
- Run `go test ./... -race` — race detector always on in CI and locally.
- Coverage floors: `pkg/` ≥ 80%, services ≥ 70%.

## Done means

`golangci-lint run ./...` clean, `go vet ./...` clean, `go test ./... -race` green, gofmt-formatted, and the plan task's Validate block passed with shown output.
