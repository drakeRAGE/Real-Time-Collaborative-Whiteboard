# ADR 006: Server-side undo and redo

## Status

Accepted.

## Context

A whiteboard undo stack kept in the browser is easy to draw and wrong as soon as two tabs, a reconnect, or a second user are involved. A client can also name an arbitrary stroke and mark it undone. The board already has one ordered log (ADR 004). Undo and redo belong in that log, chosen by the service that can see every op, so every subscriber renders the same board after a restart.

## Decision

The client does not name the target. `UndoInput` and `RedoInput` carry `room_id` only (`proto/whiteboard/realtime/v1/envelope.proto`). The new row's id is the envelope `client_msg_id`, which `messageID` in `services/gateway/internal/authz/pipeline.go` uses for undo, redo, and clear.

`services/board/internal/service/board.go` `ApplyUndo` and `ApplyRedo` require an admin, then `commit` replays the room. `replay` in `services/board/internal/service/replay.go` walks `seq` order. A stroke, shape, or clear becomes the undo-stack top and clears the redo stack. Undo stores `{"op_id"}` of that top and marks it inactive. Redo re-activates the redo-stack top. An empty undo stack returns nothing to undo. An empty redo stack rejects the redo, which is what happens after a newer stroke, shape, or clear. The visible board is active strokes and shapes with `seq` greater than the latest active clear.

The gateway enforces the same admin rule before publish: `undo`, `redo`, and `clear` are `RoleAdmin` in `services/gateway/internal/authz/registry.go`. Membership is checked again in the board service via room-service. The principal is `wb-user-id` or the `Wb-User-Id` header, never a field in the payload.

## Consequences

Two tabs and a restarted client see one stack, because the stack is the log. Undo costs a round trip and a replay of the room's ops inside the advisory-lock transaction. A member who is not the admin cannot undo, even if their own stroke is the top of the stack. That is the rule the registry and `requireAdmin` both implement. A client-side stack would feel faster and would diverge the moment another admin, or another tab, committed an op the first client had not applied yet.
