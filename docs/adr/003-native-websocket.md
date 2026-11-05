# ADR 003: Native WebSockets instead of Socket.IO

## Status

Accepted.

## Context

The previous app spoke Socket.IO. Socket.IO would still supply rooms, acknowledgements, and reconnection, at the cost of a second event vocabulary beside the protobuf contracts in `proto/`. Engine.IO polling is a second transport this gateway does not implement. The contracts already define one client envelope and one server envelope (`proto/whiteboard/realtime/v1/envelope.proto`). A parallel Socket.IO event map would have to stay aligned with those messages by hand.

## Decision

The browser speaks binary WebSockets. `frontend/src/realtime/client.ts` sets `binaryType` to `arraybuffer`, sends protobuf `ClientEnvelope` bytes, and reconnects with a fresh access token from `frontend/src/context/SocketProvider.tsx`. The gateway accepts the socket with `github.com/coder/websocket` (`services/gateway/go.mod`, `services/gateway/internal/ws/hub.go`). The path is `/ws`. The hub allows one `Origin` (`FRONTEND_URL`), waits up to 5 seconds for a bearer token (`DefaultAuthTimeout`), and caps a user at 3 connections.

TypeScript messages are generated into `frontend/src/realtime/gen` from the same `proto/` tree (`task proto`, ts-proto). `client.ts` queues strokes and chat until the socket is open. Every accepted envelope goes through `services/gateway/internal/authz/pipeline.go` before anything is published. The pipeline comment states the order: decode, protovalidate, membership, role, rate limit. Rules live in `services/gateway/internal/authz/registry.go`. Applied events return as `ServerEnvelope` frames from `services/gateway/internal/fanout/fanout.go`.

## Consequences

Rooms, acks, backpressure, and reconnect are this repository's code. The hub's write queue is 256 frames (`sendCap`) and a frame is capped at 64 KiB (`maxFrame`). There is no Engine.IO long-poll fallback. A client that cannot open a WebSocket cannot use the app.

That is more code than adopting Socket.IO. The gain is one schema: the same protobuf messages are validated on the gateway and on the board consumer, and authorization is a table in `registry.go` rather than a Socket.IO middleware that trusts event names.
