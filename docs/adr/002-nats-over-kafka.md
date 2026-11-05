# ADR 002: NATS JetStream instead of Kafka

## Status

Accepted.

## Context

Board and chat events must be redelivered after a consumer crash, and a redelivered op must not create a second row. Cursors and presence are ephemeral: a missed cursor is stale on arrival. Kafka would give a large retained log, a mature connector ecosystem, and consumer groups that many operators already know. It would also add a JVM cluster, partition planning, and a heavier local Compose file for a product whose subjects are a handful of protobuf events.

## Decision

Use NATS. `pkg/natsx.EnsureStreams` creates two JetStream streams, `BOARD` (`wb.board.>`) and `CHAT` (`wb.chat.>`), with file storage, a 7-day max age, and a 2-minute duplicate window (`Duplicates` in `pkg/natsx/natsx.go`). `Publish` sets `Nats-Msg-Id`. Presence uses core NATS: `services/presence/cmd/main.go` calls `nats.Connect`, and `services/presence/internal/service/presence.go` publishes `wb.presence.user_offline.v1` with `nc.Publish`. Cursor ingress is marked core in `services/gateway/internal/authz/registry.go` (`cursor_move`, `Core: true`).

`pkg/natsx.Consume` cannot set filter subjects. Board and chat therefore create their own durables with the same explicit ack, five deliveries, and backoff: `board-ingest` in `services/board/internal/ingest/consumer.go` (ingest subjects only) and `chat-ingest` in `services/chat/internal/ingest/consumer.go` (`wb.chat.message.v1`). Gateway fan-out subscribes to the applied subjects in `services/gateway/internal/fanout/fanout.go`.

## Consequences

JetStream delivery is at least once. Board inserts use `ON CONFLICT (id) DO NOTHING` in `services/board/internal/repo/queries/board.sql`. Dedup is per stream, so an applied event must carry a different `Nats-Msg-Id` than the ingest op. `appliedMessageID` in `services/board/internal/service/board.go` does that. Reusing the op ULID would drop the applied record inside the 2-minute window and leave fan-out with nothing to send.

Kafka remains the better tool for a huge retained log or a team that already runs it. This product does not. The local broker is the `nats:2.10` service in `deploy/compose/docker-compose.yml`.
