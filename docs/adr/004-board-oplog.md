# ADR 004: Board op-log, undo/redo, and snapshots

## Status

Accepted (Phase 5).

## Context

Drawing is asynchronous. The gateway publishes ingest events on the BOARD
JetStream stream; board-service is the only writer of `strokes` and
`board_snapshots`. Clients need a gapless per-room order, restart-safe
delivery, and standard whiteboard undo/redo: undo targets the last undoable
op, redo restores it, and a new drawing or clear clears the redo stack.

`docs/SCHEMA.md` is the schema contract. `strokes` has no `active` column.
`seq` is assigned in the insert transaction with a per-room advisory lock.
`authenticated` cannot write the op-log.

## Decision

`strokes` is the op-log. `kind` is `stroke`, `shape`, `clear`, `undo`, or
`redo`. Payload shapes are the ones in `docs/SCHEMA.md`. Rows are not deleted
by compaction.

An op is on the effective board only after replay in `seq` order:

- `stroke` and `shape` start active and become the undo-stack top. They clear
  the redo stack.
- `clear` starts active, hides drawing ops with a lower `seq`, becomes the
  undo-stack top, and clears the redo stack.
- `undo` stores `{"op_id"}` of the undo-stack top and marks that op inactive.
  The target moves to the redo stack. Undo of a clear marks the clear
  inactive, so drawings it hid are visible again when they are still active.
- `redo` re-activates `op_id` and moves it back to the undo stack. Redo is
  rejected when the redo stack is empty. A stroke, shape, or clear empties
  that stack, so redo after a new op does not insert a row.
- The effective board is active `stroke` and `shape` ops, in `seq` order,
  with `seq` greater than the latest active clear.

The server picks the undo and redo targets. Clients send only the room.

`seq` is `COALESCE(MAX(seq), 0) + 1` in the same statement as the insert,
inside a transaction that holds `pg_advisory_xact_lock(hashtext(room_id))`.
`ON CONFLICT (id) DO NOTHING` keeps the original row on JetStream
redelivery. A unique violation of `strokes_room_seq_idx` retries the
transaction. The lock is held only around Postgres work, not around the
room-service RPC or Redis.

The principal is `wb-user-id` gRPC metadata or the `Wb-User-Id` NATS header.
Payload fields are never the principal. Stroke and shape require membership.
Clear, undo, and redo require admin. Both are re-checked here with
room-service `CheckMembership` before the transaction.

Redis key `board:{roomID}` is a LIST of the last 500 protobuf `BoardOp`
values, appended in `seq` order and `LTRIM`med. The ingest consumer allows
one in-flight message so a single process appends in order. A missing key is
rebuilt from the latest 500 Postgres rows. `GetHistory` and `GetSnapshot`
read Postgres. After a Redis flush they still match the database, and the
read rebuilds the list.

When `seq % 1000 == 0`, the same transaction writes `board_snapshots` for
that `upto_seq`. The payload is the effective ops with `seq <= upto_seq`.
`GetHistory` pages that snapshot, then `strokes` with `seq` greater than
`upto_seq`. `GetSnapshot` replays the strokes log so a later undo still
matches the effective board. Old stroke rows stay; the snapshot is a
checkpoint, not a delete.

The durable consumer is `board-ingest` on stream `BOARD`. Its filter
subjects are the five ingest subjects in `docs/CONTRACTS.md`
(`wb.board.stroke.v1`, `shape`, `clear`, `undo`, `redo`). It does not
subscribe to `wb.board.>`, so the `*_applied` events this service publishes
back onto BOARD are not delivered to it. `pkg/natsx.Consume` has no filter
argument (it would bind every subject on the stream), so board-service
creates that consumer with the same ack policy as `natsx`: explicit ack,
max deliver 5, the same backoff, and `wb.dlq.<subject>` on the final
failure. Publish and stream setup still go through `pkg/natsx`.

The handler acks only after `Ingest` returns nil. `Ingest` commits the
Postgres transaction, then updates Redis, then publishes the applied event.
A conflict on `id` still returns nil so the redelivery is acked, and the
applied event is published again (JetStream dedupes on the op ULID inside
the duplicate window). A permission, validation, or empty undo/redo stack
error is acked and not retried. Database, Redis, and publish errors are
naked so the transaction can be retried; the row is not acked first.

## Consequences

- Two board-service instances can ingest the same room. The advisory lock
  serializes `seq`. Redis append order across instances can lag; a miss
  rebuilds from Postgres.
- `GetHistory` after compaction does not resend strokes hidden by a clear
  that landed on a snapshot boundary. `GetSnapshot` and undo/redo replay
  `strokes`, which still hold those rows.
- Gateway fanout is a separate consumer. Filtering `board-ingest` does not
  remove `*_applied` messages from the stream.
