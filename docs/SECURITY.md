# Security

This file records the gateway authorization matrix. It is kept in lockstep with
`services/gateway/internal/authz` (`Rules`) and `docs/CONTRACTS.md`. The full
threat model is Phase 10.

Identity comes only from a verified Supabase JWT (`Authorization: Bearer` on
REST, or `Sec-WebSocket-Protocol: bearer, <jwt>` / the first WebSocket frame).
gRPC calls carry `wb-user-id`. NATS publishes carry `Wb-User-Id` and
`Wb-Username`. Client payloads are not a source of identity.

`PermissionDenied` from room-service is HTTP 403. `Unauthenticated` is HTTP 401.

## REST

| Route | Requirement |
|---|---|
| GET /healthz | public |
| GET /readyz | public; fails when NATS, Redis, or room-service is down, and during shutdown |
| GET /ws | JWT, then the event matrix below |
| POST /api/v1/rooms | JWT; room-service CreateRoom (caller becomes admin) |
| GET /api/v1/rooms/{roomID} | JWT; member via room-service |
| DELETE /api/v1/rooms/{roomID} | JWT; admin via room-service |
| POST /api/v1/rooms/{roomID}/join | JWT; room-service validates the room or invite |
| POST /api/v1/rooms/{roomID}/leave | JWT; member via room-service |
| GET /api/v1/rooms/{roomID}/members | JWT; member via room-service |
| POST /api/v1/rooms/{roomID}/invites | JWT; admin via room-service |
| POST /api/v1/rooms/{roomID}/transfer | JWT; admin via room-service |
| POST /api/v1/invites/validate | JWT |

## WebSocket events

| Event | Role | Limit | Effect |
|---|---|---|---|
| join_room | NONE | 5/10s/user | RoomService.JoinRoom |
| leave_room | MEMBER | none | RoomService.LeaveRoom |
| stroke_input | MEMBER | 20/s/user | wb.board.stroke.v1 |
| shape_input | MEMBER | 20/s/user | wb.board.shape.v1 |
| clear | ADMIN | 1/s/user | wb.board.clear.v1 |
| undo | ADMIN | 5/s/user | wb.board.undo.v1 |
| redo | ADMIN | 5/s/user | wb.board.redo.v1 |
| chat_send | MEMBER | delegated to chat-service | wb.chat.message.v1 |
| cursor_move | MEMBER | 60/s/conn | wb.presence.cursor.v1 |
| call_join | MEMBER | 2/s/user | Redis call:{roomID} |
| call_leave | MEMBER | 2/s/user | Redis call:{roomID} |
| call_signal | MEMBER | 30/s/user | relay only if the target is a member of the same room |
| call_toggle | MEMBER | 5/s/user | Redis call:{roomID} |

Events with no registry row are rejected and are not published.
`call_signal` to a non-member returns `ERROR_CODE_PERMISSION_DENIED`.
Membership is cached at `authz:{userID}:{roomID}` for 30s and dropped on
`wb.room.member_joined.v1`, `wb.room.member_left.v1`, and `wb.room.deleted.v1`.
