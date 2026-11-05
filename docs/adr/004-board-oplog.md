# ADR 004: Op-log and snapshots instead of a CRDT

## Status

Accepted.

## Context

A collaborative surface can merge peers with a CRDT such as Yjs, or it can append operations to a server log and give them one order. Yjs is the right tool when several writers edit the same structure while offline and must converge without a central sequencer. Text and shared rich-text are the usual case. A whiteboard stroke here is a new object, not a character-level merge, and undo is a server-chosen stack (ADR 006). Clients do not edit the same stroke concurrently and reconcile.

## Decision

`strokes` is the op-log. `docs/SCHEMA.md` allows `kind` of `stroke`, `shape`, `clear`, `undo`, or `redo`. `seq` is `COALESCE(MAX(seq), 0) + 1` in the insert, inside `pg_advisory_xact_lock(hashtext(room_id))`, with `ON CONFLICT (id) DO NOTHING` (`services/board/internal/repo/queries/board.sql`). The lock and insert run in `services/board/internal/service/board.go` `commit`. Rows are not deleted to compact history.

Every 1000th `seq` (`snapshotEvery`) writes `board_snapshots` for that `upto_seq`. The payload is the effective ops from `effective` in `services/board/internal/service/replay.go`. `GetHistory` returns that snapshot plus rows with a greater `seq`. `GetSnapshot` replays the log so a later undo still matches the visible board. Redis key `board:{roomID}` holds the last 500 encoded ops (`services/board/internal/cache/cache.go`). A missing key is rebuilt from Postgres in `rebuild`.

The durable consumer is `board-ingest` (`services/board/internal/ingest/consumer.go`). Its filters are the five ingest subjects. It does not subscribe to `wb.board.>`, so the `*_applied` events this service publishes back onto `BOARD` are not delivered to itself.

Use Yjs, or another CRDT, when peers must merge offline without a total server order. This board wants one `seq` per room, Postgres as the history, and admin-gated undo. A client-side CRDT would move that authority off the service that checks membership.

## Consequences

Clients send inputs and render applied events. They do not merge. Two board-service processes can ingest one room; the advisory lock serializes `seq`. Redis append order across processes can lag, and a missing list is rebuilt from Postgres. `GetHistory` after a snapshot boundary does not resend strokes a clear has already hidden inside that snapshot. `GetSnapshot` and undo replay `strokes`, which still hold those rows. `authenticated` may `SELECT` strokes as a member and has no `INSERT`, `UPDATE`, or `DELETE` grant (`docs/SCHEMA.md`).
