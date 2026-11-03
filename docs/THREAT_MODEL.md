# Threat model

STRIDE-lite for the realtime edge. Each mitigation names a file that is in
the tree. Residual risk is what that control does not cover.
`docs/SECURITY.md` holds the authz matrix these threats rely on.

## WebSocket hijack

Spoofing. An attacker who does not hold the user's Supabase access token
opens `/ws` and sends events as that user, or a page on another origin
rides an existing browser session.

Mitigation. The hub accepts one browser origin, `FRONTEND_URL`, and rejects
a wildcard (`services/gateway/internal/ws/hub.go`,
`services/gateway/internal/config/config.go`). The socket is not served
until `pkg/authn` verifies a token from the `bearer` subprotocol or from
the first frame, which must arrive within 5 seconds
(`services/gateway/internal/ws/hub.go`). Verification checks the algorithm,
the JWKS signature, `iss`, `aud`, `exp`, and `sub`
(`pkg/authn/authn.go`). CORS on the HTTP router allows only that same
origin (`services/gateway/internal/httpapi/server.go`).

Residual risk. There is no revocation list. A stolen access token is
accepted until `exp`. The first-frame token is visible to anything that
can read the bytes, so the deployment has to terminate TLS in front of
the gateway. The process sets HSTS and frame denial
(`services/gateway/internal/httpapi/server.go`) and does not itself
redirect HTTP to HTTPS.

## Room-id enumeration

Information disclosure. An authenticated caller probes room UUIDs to learn
which rooms exist, then joins one.

Mitigation. Every `/api/v1` route requires a verified JWT
(`services/gateway/internal/httpapi/server.go`). `GetRoom` returns the
room only after `requireMember`
(`services/room/internal/service/room.go`). Missing and deleted rooms are
`NotFound`; a live room the caller is not in is `PermissionDenied`
(`services/room/internal/grpc/server.go`). Room ids are UUIDs. Direct
table reads by `anon` and `authenticated` are denied by RLS
(`migrations/0006_rls.up.sql`). `join_room` is limited to 5 per 10 seconds
per user (`services/gateway/internal/authz/registry.go`).

Residual risk. The 404 versus 403 split tells a caller that a guessed id
exists. Guessing a UUID is not practical; a leaked id is.
`JoinRoom` adds the caller as a member when the room exists and the
invite token is empty (`services/room/internal/service/room.go`). There
is no REST rate limit on `GET /api/v1/rooms/{roomID}`.

## Stroke flooding

Denial of service. A member sends strokes faster than the board or the
other clients can absorb.

Mitigation. `stroke_input` and `shape_input` allow 20 per second per user.
The limiter is the Redis token bucket (`pkg/redisx/redis.go`) called from
the authz pipeline before publish
(`services/gateway/internal/authz/pipeline.go`). A Redis error fails
closed with `ERROR_CODE_INTERNAL`. Protovalidate caps a stroke at 500
points (`proto/whiteboard/realtime/v1/envelope.proto`). The hub reads at
most 64 KiB per frame and keeps at most 3 sockets per user
(`services/gateway/internal/ws/hub.go`). The board consumer checks
membership again before insert
(`services/board/internal/service/board.go`).

Residual risk. The cap is per user, not per room. Many members can each
sustain 20 strokes per second. A Redis outage rejects new strokes until
the bucket is reachable again.

## Signaling abuse

Elevation of privilege and information disclosure. A client sends
`call_signal` so the gateway relays SDP or ICE to someone outside the room,
or forges the sender id.

Mitigation. The sender must be a member. The pipeline then loads the
target's role in that same room and returns
`ERROR_CODE_PERMISSION_DENIED` unless the target is a member or admin
(`services/gateway/internal/authz/pipeline.go`). The relayed frame's
`from_user_id` is the authenticated principal, not a payload field
(`services/gateway/internal/authz/pipeline.go`). The event is limited to
30 per second per user (`services/gateway/internal/authz/registry.go`).
The payload is opaque bytes with a proto max of 65536
(`proto/whiteboard/realtime/v1/envelope.proto`), and the hub's 64 KiB
frame cap applies first (`services/gateway/internal/ws/hub.go`).

Residual risk. Any member can still deliver 30 opaque payloads per second
to another member of the same room. The gateway does not parse SDP.

## Envelope replay

Repudiation and tampering. A client or a bus peer resubmits a
`ClientEnvelope` or a NATS message and the board or the fanout applies it
again.

Mitigation. Publish sets `Nats-Msg-Id` to the stroke id, shape id, chat
message id, or `client_msg_id`, all of which are ULIDs
(`services/gateway/internal/publish/publish.go`,
`services/gateway/internal/authz/pipeline.go`). BOARD and CHAT streams
drop duplicates for 2 minutes (`pkg/natsx/natsx.go`). Stroke rows insert
with `ON CONFLICT (id) DO NOTHING`
(`services/board/internal/repo/queries/board.sql`). `commit` treats an
existing id as the stored op and does not assign a new seq
(`services/board/internal/service/board.go`). Chat inserts use the same
conflict clause (`services/chat/internal/repo/queries/chat.sql`). Fanout
drops a `Nats-Msg-Id` it has already delivered
(`services/gateway/internal/fanout/fanout.go`).

Residual risk. After the 2 minute JetStream window, `commit` still calls
`publish` for an id that was already stored
(`services/board/internal/service/board.go`), and chat ingest still
publishes `message_sent` when the insert conflicts
(`services/chat/internal/service/chat.go`). Fanout's dedupe map is
process-local and keeps 4096 ids
(`services/gateway/internal/fanout/fanout.go`), so a late replay can be
broadcast again. Cursor and presence use core NATS
(`services/gateway/internal/publish/publish.go`) and have no duplicate
window.

## NATS access from a compromised service

Elevation of privilege and tampering. A workload that can open port 4222
publishes on `wb.board.>` or `wb.chat.>` with a forged `Wb-User-Id`.

Mitigation. The gateway stamps `Wb-User-Id` from the verified principal
(`services/gateway/internal/publish/publish.go`). Board ingest rejects a
message whose header is not a UUID and checks that user is still a member
before writing (`services/board/internal/ingest/consumer.go`,
`services/board/internal/service/board.go`). On Kubernetes, NATS port 4222
accepts only the gateway, room, board, chat, and presence pods, and the
other workloads do not accept cluster ingress except from the gateway and
NATS (`deploy/k8s/netpol/`). Postgres RLS blocks the `authenticated` role
from writing those tables (`migrations/0006_rls.up.sql`).

Residual risk. The NATS process has no accounts or credentials. Compose
starts it as `nats -js -sd /data -m 8222` and publishes host port 4222
(`deploy/compose/docker-compose.yml`). Any client that can reach that
port can subscribe and publish. Board and chat trust `Wb-User-Id` on the
message. Chat ingest does not re-check room membership
(`services/chat/internal/service/chat.go`). NetworkPolicy does not apply
to the compose network, and a compromised pod that is already on the
allow-list can still forge headers. The kind test that a non-allow-listed
pod is blocked is Phase 13.

## JWKS outage

Denial of service and spoofing. The Supabase JWKS endpoint is down, so
the gateway cannot learn the current signing keys. Either nobody can
connect, or a retired key is still trusted.

Mitigation. With HS256 mode off, `pkg/authn.New` fetches the JWKS document
before the process serves, and a failed fetch aborts startup
(`pkg/authn/authn.go`, `services/gateway/internal/config/config.go`).
Verification uses the cached set. If that lookup fails, the token is
`ErrInvalid` and the request is 401 (`pkg/authn/authn.go`,
`services/gateway/internal/httpapi/server.go`). The cache refreshes about
every 15 minutes. `Refresh` is available for an operator-triggered pull
after a rotation (`pkg/authn/authn.go`).

Residual risk. A gateway that is already running keeps the last good
document when a later refresh fails, so a key that Supabase has retired
during the outage still verifies until a refresh succeeds. A restart
during the outage refuses to boot, which is an availability loss. Local
compose sets `SUPABASE_JWT_ALLOW_HS256` and does not consult JWKS at all
(`deploy/compose/docker-compose.yml`).
