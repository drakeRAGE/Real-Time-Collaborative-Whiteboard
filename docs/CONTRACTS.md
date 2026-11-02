# Realtime Contracts

Single source of truth for wire contracts: `proto/whiteboard/<domain>/v1/`.
Generated code: `gen/go/` (Go) and `frontend/src/realtime/gen/` (TypeScript via ts-proto).
Regenerate with `task proto`. Never hand-edit generated output.

- Protobuf packages: `whiteboard.<domain>.v1`
- NATS subjects: `wb.<domain>.<event>.v1`
- WS transport: binary protobuf frames at the gateway `GET /ws` —
  client→server `whiteboard.realtime.v1.ClientEnvelope`,
  server→client `whiteboard.realtime.v1.ServerEnvelope`

## Idempotency rule

Every durable publish sets the `Nats-Msg-Id` header to a **ULID** so JetStream
dedupes redeliveries/replays inside the stream's duplicate window (2 minutes):

| Publish | `Nats-Msg-Id` |
|---|---|
| `wb.board.stroke.v1` | `StrokeInput.stroke_id` |
| `wb.board.shape.v1` | `ShapeInput.shape_id` |
| `wb.board.clear.v1` / `wb.board.undo.v1` / `wb.board.redo.v1` | `ClientEnvelope.client_msg_id` |
| `wb.chat.message.v1` | `ChatSend.message_id` |
| `*_applied` / `*_sent` events | ULID of the applied op/message |

Consumers are additionally idempotent at the DB layer (`INSERT ... ON CONFLICT (id) DO NOTHING`),
so redelivery outside the dedup window is still safe.

## JetStream streams

| Stream | Subjects | Retention | Max age | Dup window | Storage |
|---|---|---|---|---|---|
| `BOARD` | `wb.board.>` | Limits | 7d | 2m | File |
| `CHAT` | `wb.chat.>` | Limits | 7d | 2m | File |

`wb.presence.>` and `wb.room.>` are **core NATS** (ephemeral, no stream):
presence is throwaway cursor/heartbeat traffic; room lifecycle events are
cache-invalidation + fanout signals whose source of truth is Postgres.

## Durable consumers

| Consumer | Stream | Filter subjects | Semantics |
|---|---|---|---|
| `board-ingest` (board-service) | `BOARD` | `wb.board.stroke.v1`, `wb.board.shape.v1`, `wb.board.clear.v1`, `wb.board.undo.v1`, `wb.board.redo.v1` | explicit ack after DB commit, max_deliver 5, backoff; final failure → republish to `wb.dlq.<orig>` |
| `chat-ingest` (chat-service) | `CHAT` | `wb.chat.message.v1` | same |

`board-ingest` deliberately filters ingest subjects only (not `wb.board.>`) so
it never consumes the `*_applied` events it publishes itself. The gateway's
fanout subscriptions to `*_applied` / `*_sent` events are ephemeral per-instance
consumers (deliver-new), not durables — a gateway restart must not replay history
to live sockets.

## NATS subject table

| Subject | Stream | Publisher | Consumer(s) | Payload (proto) | Idempotency key |
|---|---|---|---|---|---|
| `wb.board.stroke.v1` | BOARD | gateway | board-ingest | `realtime.v1.StrokeInput` + principal headers | stroke_id |
| `wb.board.shape.v1` | BOARD | gateway | board-ingest | `realtime.v1.ShapeInput` + principal headers | shape_id |
| `wb.board.clear.v1` | BOARD | gateway | board-ingest | `realtime.v1.ClearInput` + principal headers | client_msg_id |
| `wb.board.undo.v1` | BOARD | gateway | board-ingest | `realtime.v1.UndoInput` + principal headers | client_msg_id |
| `wb.board.redo.v1` | BOARD | gateway | board-ingest | `realtime.v1.RedoInput` + principal headers | client_msg_id |
| `wb.board.stroke_applied.v1` | BOARD | board-service | gateway fanout | `board.v1.StrokeAdded` | stroke id |
| `wb.board.shape_applied.v1` | BOARD | board-service | gateway fanout | `board.v1.ShapeAdded` | shape id |
| `wb.board.cleared.v1` | BOARD | board-service | gateway fanout | `board.v1.BoardCleared` | op_id |
| `wb.board.undo_applied.v1` | BOARD | board-service | gateway fanout | `board.v1.UndoApplied` | undo client_msg_id |
| `wb.board.redo_applied.v1` | BOARD | board-service | gateway fanout | `board.v1.RedoApplied` | redo client_msg_id |
| `wb.chat.message.v1` | CHAT | gateway | chat-ingest | `realtime.v1.ChatSend` + principal headers | message_id |
| `wb.chat.message_sent.v1` | CHAT | chat-service | gateway fanout | `chat.v1.Message` | message id |
| `wb.chat.rejected.v1` | CHAT | chat-service | gateway fanout | `chat.v1.Message` id + reason header | message id |
| `wb.presence.cursor.v1` | core | gateway | presence-service, gateway fanout | `presence.v1.CursorMoved` | n/a (ephemeral) |
| `wb.presence.user_online.v1` | core | presence-service | gateway fanout | `presence.v1.UserOnline` | n/a |
| `wb.presence.user_offline.v1` | core | presence-service | gateway fanout | `presence.v1.UserOffline` | n/a |
| `wb.room.member_joined.v1` | core | room-service | gateway (fanout + authz cache invalidation) | `room.v1.Member` + room id header | n/a |
| `wb.room.member_left.v1` | core | room-service | gateway (fanout + authz cache invalidation) | `room.v1.Member` + room id header | n/a |
| `wb.room.deleted.v1` | core | room-service | gateway (fanout + authz cache invalidation) | room id | n/a |
| `wb.dlq.<orig-subject>` | — (logged/alerting) | natsx on final delivery failure | ops tooling | original payload + error header | original msg id |

"Principal headers": the gateway stamps `Wb-User-Id` (JWT sub) and `Wb-Username`
NATS headers on everything it publishes — consumer services never trust any
user id found in a payload that a client could have produced.

## WS messages — direction, authz, rate limits

Role gates (enforced by the gateway authz pipeline, Phase 7): `NONE` = any
authenticated principal, `MEMBER`/`ADMIN` = role in the target room from
`RoomService.CheckMembership` (Redis-cached, 30s TTL).

### Client → server (`ClientEnvelope.payload`)

| oneof field | Message | Required role | Rate limit | Published to |
|---|---|---|---|---|
| `join_room` | `JoinRoom` | NONE (valid room/invite) | 5/10s/user | `RoomService.JoinRoom` (gRPC, sync) |
| `leave_room` | `LeaveRoom` | MEMBER | — | `RoomService.LeaveRoom` (gRPC, sync) |
| `stroke_input` | `StrokeInput` | MEMBER | 20/s/user | `wb.board.stroke.v1` |
| `shape_input` | `ShapeInput` | MEMBER | 20/s/user | `wb.board.shape.v1` |
| `clear` | `ClearInput` | ADMIN | 1/s/user | `wb.board.clear.v1` |
| `undo` | `UndoInput` | ADMIN | 5/s/user | `wb.board.undo.v1` |
| `redo` | `RedoInput` | ADMIN | 5/s/user | `wb.board.redo.v1` |
| `chat_send` | `ChatSend` | MEMBER | 5/10s/user (chat-service) | `wb.chat.message.v1` |
| `cursor_move` | `CursorMove` | MEMBER | 60/s/conn | `wb.presence.cursor.v1` |
| `call_join` | `CallJoin` | MEMBER | 2/s/user | gateway call roster (Redis) + broadcast |
| `call_leave` | `CallLeave` | MEMBER | 2/s/user | gateway call roster (Redis) + broadcast |
| `call_signal` | `CallSignal` | MEMBER (sender AND target) | 30/s/user | direct relay to target's sockets |
| `call_toggle` | `CallToggle` | MEMBER | 5/s/user | gateway call roster (Redis) + broadcast |

Every client message is acknowledged with `ServerEnvelope.ack{client_msg_id}`;
failures carry `ack.error{code, message}`. Events not in this table fail closed.

### Server → client (`ServerEnvelope.payload`)

| oneof field | Message | Triggered by |
|---|---|---|
| `ack` | `Ack` | every processed `ClientEnvelope` |
| `error` | `Error` | connection-level failures (auth timeout, slow consumer, …) |
| `room_state` | `RoomState` | after join; on membership/admin change |
| `user_online` | `presence.v1.UserOnline` | `wb.presence.user_online.v1` |
| `user_offline` | `presence.v1.UserOffline` | `wb.presence.user_offline.v1` |
| `stroke_applied` | `board.v1.StrokeAdded` | `wb.board.stroke_applied.v1` |
| `shape_applied` | `board.v1.ShapeAdded` | `wb.board.shape_applied.v1` |
| `board_cleared` | `board.v1.BoardCleared` | `wb.board.cleared.v1` |
| `undo_applied` | `board.v1.UndoApplied` | `wb.board.undo_applied.v1` |
| `redo_applied` | `board.v1.RedoApplied` | `wb.board.redo_applied.v1` |
| `chat_message` | `chat.v1.Message` | `wb.chat.message_sent.v1` |
| `cursor_moved` | `presence.v1.CursorMoved` | `wb.presence.cursor.v1` (excluding originator) |
| `call_state` | `CallState` | to the joining participant |
| `call_user_joined` | `CallUserJoined` | call roster change |
| `call_user_left` | `CallUserLeft` | call roster change |
| `call_ended` | `CallEnded` | last participant left |
| `call_toggled` | `CallToggled` | mic/camera toggle |
| `call_signal` | `CallSignalRelayed` | peer's `CallSignal` |

### Ack/error semantics

- Exactly one `Ack` per `ClientEnvelope`, matched by `client_msg_id`.
- `Ack.error` unset ⇒ accepted (for async ops: accepted into NATS; the
  authoritative apply arrives later as the corresponding `*_applied` event).
- `Ack.error.code` set ⇒ rejected; no state change. `ERROR_CODE_RATE_LIMITED`
  maps legacy "sending messages too fast".
- Unsolicited `Error` (no `client_msg_id`) is connection-scoped, e.g.
  `ERROR_CODE_UNAUTHENTICATED` before close 4401.

## Legacy socket.io → proto mapping

Every capability in `legacy-backend/index.js` and its replacement:

### Client-emitted legacy events

| Legacy event (payload) | New contract |
|---|---|
| `joinRoom(roomId)` | `ClientEnvelope.join_room` → `RoomService.JoinRoom`; history now via REST (`BoardService.GetHistory`/`GetSnapshot`, `ChatService.GetHistory`) |
| `cursorMove({roomId,x,y})` | `ClientEnvelope.cursor_move` → `wb.presence.cursor.v1` |
| `chat:send({roomId,text}, ack)` | `ClientEnvelope.chat_send` → `wb.chat.message.v1`; socket.io ack ⇒ `ServerEnvelope.ack` |
| `draw({x0,y0,x1,y1,color,size})` | `ClientEnvelope.stroke_input` (`Stroke` polyline `points[]`, `width`, `tool`) → `wb.board.stroke.v1` |
| `drawShape({x0,y0,x1,y1,color,size,shape})` | `ClientEnvelope.shape_input` (`ShapeKind` LINE/RECT/CIRCLE) → `wb.board.shape.v1` |
| `clear()` | `ClientEnvelope.clear` → `wb.board.clear.v1` (now ADMIN-only) |
| `undo(roomId)` | `ClientEnvelope.undo` → `wb.board.undo.v1` (ADMIN-only, server picks target op) |
| `redo(roomId)` | `ClientEnvelope.redo` → `wb.board.redo.v1` (ADMIN-only) |
| `call:join({roomId})` | `ClientEnvelope.call_join` |
| `call:leave({roomId})` | `ClientEnvelope.call_leave` |
| `call:toggleMic({roomId,enabled})` | `ClientEnvelope.call_toggle` (`MEDIA_KIND_MIC`) |
| `call:toggleCamera({roomId,enabled})` | `ClientEnvelope.call_toggle` (`MEDIA_KIND_CAMERA`) |
| `call:signal({roomId,to,signal})` | `ClientEnvelope.call_signal` (`target_user_id` required; no room-wide fallback broadcast — member-to-member only) |
| *(implicit)* socket `disconnect` | WS close → gateway emits `wb.presence.user_offline.v1`; admin transfer handled by room-service (`LeaveRoom`/heartbeat expiry) |

### Server-emitted legacy events

| Legacy event | New contract |
|---|---|
| `initialData{drawings, adminId}` | `ServerEnvelope.room_state` (adminId) + REST history fetch (drawings) |
| `chat:history` (last 50) | `ChatService.GetHistory` via gateway REST |
| `userJoined{userId, users, username, adminId}` | `ServerEnvelope.user_online` + `room_state` |
| `userLeft{userId, users, username}` | `ServerEnvelope.user_offline` + `room_state` |
| `cursorMove{userId,x,y,username,color,socketId}` | `ServerEnvelope.cursor_moved` (originator excluded server-side; color from ported deterministic hash) |
| `chat:new{...}` | `ServerEnvelope.chat_message` |
| `draw(doc)` | `ServerEnvelope.stroke_applied` |
| `drawShape(doc)` | `ServerEnvelope.shape_applied` |
| `clear` | `ServerEnvelope.board_cleared` |
| `undo{removed, drawings}` | `ServerEnvelope.undo_applied{op_id}` (clients remove op locally; no full-board resend) |
| `redo{added, drawings}` | `ServerEnvelope.redo_applied{op_id}` |
| `call:state(session)` | `ServerEnvelope.call_state` |
| `call:userJoined{...}` | `ServerEnvelope.call_user_joined` |
| `call:userLeft{...}` | `ServerEnvelope.call_user_left` |
| `call:ended` | `ServerEnvelope.call_ended` |
| `call:micToggled{userId,enabled}` / `call:cameraToggled{...}` | `ServerEnvelope.call_toggled` |
| `call:signal{from, signal}` | `ServerEnvelope.call_signal` (`CallSignalRelayed`) |
| `errorMsg(string)` | `ServerEnvelope.ack.error` (request-scoped) or `ServerEnvelope.error` (connection-scoped) |

Deliberate behavior changes vs legacy (locked by the plan):
`clear`/`undo`/`redo` are ADMIN-gated server-side (legacy only gated undo/redo
in the UI); `undo`/`redo` broadcast op ids instead of the full drawings array;
signaling requires an explicit target and membership on both ends; rooms,
users, strokes and chat persist in Postgres (RLS) instead of MongoDB.

## Breaking-change policy

`buf breaking` with `FILE` rules runs against `.git#branch=main`.
**The breaking baseline starts at the Phase 1 commit** — there is no proto
history before it (Phase 0 scaffolded empty directories). Breaking changes to
any `v1` package require a new `v2` package, never edits to `v1`.
