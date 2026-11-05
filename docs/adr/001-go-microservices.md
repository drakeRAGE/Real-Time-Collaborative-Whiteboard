# ADR 001: Go microservices instead of a modular monolith

## Status

Accepted.

## Context

The whiteboard has five runtime shapes that do not share a lifetime. Browsers hold long-lived sockets. Room membership and invites are request/response. Board and chat must survive restarts and redelivery. Cursors expire. A modular monolith would keep one deployable, one database transaction, and simpler local debugging. That is a sound default when every module restarts together. It is a weaker fit once the socket process, the durable board consumer, and cursor expiry need to restart and scale apart.

The processes in this tree are `services/gateway/cmd/main.go`, `services/room/cmd/main.go`, `services/board/cmd/main.go`, `services/chat/cmd/main.go`, and `services/presence/cmd/main.go`. Shared libraries are `pkg/grpcx`, `pkg/natsx`, `pkg/authn`, `pkg/otelx`, `pkg/redisx`, `pkg/config`, and `pkg/logger`, wired in `go.work`.

## Decision

Ship five Go services. Synchronous calls are gRPC. `pkg/grpcx.NewServer` chains recovery, OpenTelemetry, logging, and protovalidate. Board and chat durability is NATS JetStream through `pkg/natsx`. The gateway terminates native WebSockets in `services/gateway/internal/ws/hub.go`. Local orchestration is `deploy/compose/docker-compose.yml`, started with `task up`. Kubernetes packaging is the Helm charts under `deploy/k8s/charts/`.

Go is the language because the hub is goroutine-per-connection code in `services/gateway/internal/ws/hub.go`, and each service builds to a static image. The HTTP API is chi in `services/gateway/internal/httpapi/server.go`. Room, board, and chat persist in Postgres. Presence state and the board hot list are Redis (`pkg/redisx`, `services/presence/internal/store`, `services/board/internal/cache/cache.go`).

## Consequences

A membership check is an RPC. `services/board/internal/members/client.go` calls room-service. A board write and that check are separate failures. Traces cross processes through `pkg/otelx`. Running the system means Compose or Helm.

The split matches those durability and scale profiles. At a smaller product, the same package boundaries inside one process would be less to operate. This repository pays the orchestration cost because the socket hub and the board consumer already have different jobs.
