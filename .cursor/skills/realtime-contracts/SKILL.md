---
name: realtime-contracts
description: >-
  Contract-first protobuf and NATS rules for this repo. Use when adding or
  changing anything in proto/, generated code, WS envelope messages, NATS
  subjects, or when wiring a new realtime event end to end (client -> gateway
  -> NATS -> service -> fanout).
---

# Realtime Contract Rules

`proto/whiteboard/<domain>/v1/` is the single source of truth. Go types AND frontend TS types are generated from it (`task proto`). Hand-editing `gen/` or `frontend/src/realtime/gen/` is forbidden — fix the `.proto` and regenerate.

## Changing a contract

1. Edit the `.proto`. Add protovalidate rules for every new field (lengths, UUID/ULID format, numeric bounds). Unvalidated fields are a review blocker.
2. `buf lint` must pass. `buf breaking --against '.git#branch=main'` must pass — breaking changes require a new `v2` package, never edits to `v1`.
3. `task proto`, commit source + generated output together. CI checks drift via `task proto && git diff --exit-code`.

## Adding a new realtime event (full checklist)

A new WS client event is complete ONLY when all of these exist in the same PR:

1. `ClientEnvelope` oneof entry (and `ServerEnvelope` response/broadcast entry) in `proto/whiteboard/realtime/v1/envelope.proto`.
2. Row in the gateway authz registry (`services/gateway/internal/authz/`): event → required role (NONE/MEMBER/ADMIN) → rate limit → NATS subject. Events without a registry row must fail closed.
3. NATS subject following `wb.<domain>.<event>.v1`, documented in `docs/CONTRACTS.md` with: owning JetStream stream, consumer, idempotency key.
4. Consumer handling in the owning service (persist → ack → publish applied-event).
5. Fanout translation in gateway (NATS event → `ServerEnvelope`).
6. Row in the `docs/SECURITY.md` authz matrix.
7. Table-test entries in the gateway authz pipeline test (non-member / member / admin expectations).

## Subjects quick reference

- Durable (JetStream): `wb.board.>` → BOARD stream; `wb.chat.>` → CHAT stream.
- Ephemeral (core NATS): `wb.presence.>`.
- Dead letters: `wb.dlq.<original-subject>`.
- Room lifecycle: `wb.room.member_joined.v1`, `wb.room.member_left.v1`, `wb.room.deleted.v1` (these also invalidate gateway authz caches).
