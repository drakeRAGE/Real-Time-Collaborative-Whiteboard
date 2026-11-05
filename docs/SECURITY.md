# Security

This file records the gateway authorization matrix. It is kept in lockstep with
`services/gateway/internal/authz` (`Rules`) and `docs/CONTRACTS.md`. Threats
and residual risk are in `docs/THREAT_MODEL.md`.

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
| GET /api/v1/rooms/{roomID}/board/history | JWT; member via board-service GetHistory |
| GET /api/v1/rooms/{roomID}/chat/history | JWT; member via chat-service GetHistory |

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

The WebSocket table is the `Rules()` map in
`services/gateway/internal/authz/registry.go`. `Pipeline.New` in
`services/gateway/internal/authz/pipeline.go` refuses to start when a
`ClientEnvelope` oneof field has no row. REST routes are the chi routes in
`services/gateway/internal/httpapi/server.go`.

## Server frames

These `ServerEnvelope` oneof fields are not client events and have no
registry row. The gateway writes them after the authz pipeline or from
fanout (`services/gateway/internal/fanout/fanout.go`).

| Field | Produced by |
|---|---|
| ack | pipeline result for every accepted client event |
| error | connection-level failure (auth timeout, slow consumer) |
| room_state | after join, and on membership or admin change |
| user_online | `wb.presence.user_online.v1` |
| user_offline | `wb.presence.user_offline.v1` |
| stroke_applied | `wb.board.stroke_applied.v1` |
| shape_applied | `wb.board.shape_applied.v1` |
| board_cleared | `wb.board.cleared.v1` |
| undo_applied | `wb.board.undo_applied.v1` |
| redo_applied | `wb.board.redo_applied.v1` |
| chat_message | `wb.chat.message_sent.v1` |
| cursor_moved | `wb.presence.cursor.v1` |
| call_state | call roster, to the joining socket |
| call_user_joined | call roster broadcast |
| call_user_left | call roster broadcast |
| call_ended | call roster broadcast |
| call_toggled | call roster broadcast |
| call_signal | relay of `call_signal` to the target member |

## Token lifecycle

Supabase issues the access token. This service does not mint or refresh it.
The gateway only verifies it.

1. The client sends `Authorization: Bearer <jwt>` on `/api/v1/*`, or
   `Sec-WebSocket-Protocol: bearer, <jwt>` on `/ws`. If the subprotocol has
   no token, the first WebSocket frame must be the raw jwt within 5 seconds
   (`services/gateway/internal/ws/hub.go`).
2. `pkg/authn` allows RS256 and ES256, checked against the JWKS document.
   The cache refreshes about every 15 minutes. `iss`, `aud` (default
   `authenticated`), `exp`, and `sub` are required. `alg=none` and any
   algorithm outside that set are rejected. HS256 is rejected unless
   `SUPABASE_JWT_ALLOW_HS256` is set (`pkg/authn/authn.go`,
   `services/gateway/internal/config/config.go`). Startup fetches JWKS
   before serving when HS256 mode is off.
3. `sub` must be a UUID. The principal is stored on the request context.
   gRPC calls get `wb-user-id` and `wb-username`
   (`services/gateway/internal/principal/principal.go`). NATS publishes get
   `Wb-User-Id` and `Wb-Username` (`services/gateway/internal/publish/publish.go`).
   Client payload fields are not identity.
4. An expired token is HTTP 401 `token expired`. A bad token is HTTP 401
   `invalid token`. There is no server-side revocation list: a stolen token
   works until `exp`. The client refreshes with Supabase and reconnects.

Local compose sets `SUPABASE_JWT_ALLOW_HS256=true` so the stack boots without
JWKS (`deploy/compose/docker-compose.yml`). Production leaves that flag false.

## Rate limits

Gateway limits come from `Rules()` and the Redis token bucket in
`pkg/redisx/redis.go`. A Redis error on a limited event returns
`ERROR_CODE_INTERNAL` and does not publish
(`services/gateway/internal/authz/pipeline.go`). Chat is not limited in the
gateway; chat-service applies its own bucket and publishes
`wb.chat.rejected.v1` with `Wb-Reason: rate_limited`
(`services/chat/internal/ratelimit/ratelimit.go`).

| Event or route | Limit | Key | Enforced in |
|---|---|---|---|
| GET /healthz, GET /readyz | none | | public |
| `/api/v1/*` | none | | JWT only |
| join_room | 5 / 10s | user | gateway |
| leave_room | none | | gateway role gate |
| stroke_input | 20 / 1s | user | gateway |
| shape_input | 20 / 1s | user | gateway |
| clear | 1 / 1s | user | gateway |
| undo | 5 / 1s | user | gateway |
| redo | 5 / 1s | user | gateway |
| chat_send | 5 / 10s | user | chat-service (`rl:chat:{userID}`) |
| cursor_move | 60 / 1s | connection | gateway |
| call_join | 2 / 1s | user | gateway |
| call_leave | 2 / 1s | user | gateway |
| call_signal | 30 / 1s | user | gateway |
| call_toggle | 5 / 1s | user | gateway |

Stroke and shape bodies are also capped at 500 points
(`proto/whiteboard/realtime/v1/envelope.proto`). Each socket reads at most
64 KiB per frame and a user keeps at most 3 sockets
(`services/gateway/internal/ws/hub.go`).

## Pod securityContext

Phase 13 Helm charts are not in this change. The charts should set:

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
```

The service images already drop to `nonroot`
(`services/gateway/Dockerfile`, `services/room/Dockerfile`,
`services/board/Dockerfile`, `services/chat/Dockerfile`,
`services/presence/Dockerfile`: `USER nonroot:nonroot` on
`gcr.io/distroless/static-debian12:nonroot`). `readOnlyRootFilesystem` is a
chart setting, not a Dockerfile flag. If a process needs a writable
directory, mount an emptyDir. Do not put secrets in image ENV; charts should
reference a Kubernetes Secret. Compose still injects env for local dev
(`deploy/compose/docker-compose.yml`).

## NetworkPolicy

Manifests: `deploy/k8s/netpol/`. Namespace `whiteboard`. Pods the policies
select use `app.kubernetes.io/name` of `gateway`, `room`, `board`, `chat`,
`presence`, or `nats`.

Ingress from outside the cluster is only to the gateway, and only from the
`ingress-nginx` namespace on TCP 8080. room, board, chat, and presence
accept traffic from the gateway and from NATS (same namespace, or namespace
`nats`). room also accepts board, because board dials room for membership.
NATS port 4222 accepts the five app workloads. NATS monitoring port 8222
has no allow rule. Egress is not restricted, so JWKS, Postgres, and Redis
stay reachable. A kind test that a debug pod cannot reach room-service is
Phase 13; these files are not that proof.

Threats and residual risk: `docs/THREAT_MODEL.md`.
