# Local development stack

Compose runs Postgres, Redis, NATS JetStream, the five Go services, and
Jaeger, Prometheus, and Grafana. The React app is a separate Vite process.
It speaks protobuf WebSockets to the gateway.

Auth for real sign-in stays on Supabase cloud. `deploy/compose/seed.ps1`
does not call Supabase. The gateway process in compose uses local HS256
(`SUPABASE_JWT_ALLOW_HS256=true`) so it can boot without fetching a JWKS
document. That secret is a local placeholder, not a cloud JWT secret.

## Commands

From the repo root, with Docker Desktop running:

```powershell
docker compose -f deploy/compose/docker-compose.yml down -v
task up
task logs
task down
docker compose -f deploy/compose/docker-compose.yml down -v
powershell -NoProfile -File deploy/compose/seed.ps1
```

`task down` stops the stack and keeps volumes. Add `-- -v` to drop them:
`task down -- -v`. `task logs` follows logs until you stop it.

`down -v` is required before the first boot on a machine that already has a
Postgres volume from an earlier attempt. Init SQL creates `anon`,
`authenticated`, and `service_role` (`BYPASSRLS`) only on an empty data
directory. golang-migrate then applies `migrations/`. Those files are not
edited. `service_role` is `NOLOGIN`, matching `docs/SCHEMA.md`. The services
connect as the `postgres` superuser, the same way the integration tests do.
The superuser bypasses `FORCE ROW LEVEL SECURITY`.

There is no frontend image. Run the UI with `task dev-frontend` (Vite on port 5173).

## URLs

| What | URL |
|---|---|
| Gateway liveness | http://localhost:8080/healthz |
| Gateway readiness | http://localhost:8080/readyz |
| Room gRPC | localhost:7001 |
| Room health | http://localhost:8001/healthz |
| Board gRPC | localhost:7002 |
| Board health | http://localhost:8002/healthz |
| Chat gRPC | localhost:7003 |
| Chat health | http://localhost:8003/healthz |
| Presence health | http://localhost:8004/healthz |
| Jaeger UI | http://localhost:16686 |
| Grafana | http://localhost:3000 (admin / admin) |
| Grafana dashboard | http://localhost:3000/d/whiteboard-dev/whiteboard-local |
| Prometheus | http://localhost:9090 |
| Postgres | localhost:5432, database `whiteboard`, user `postgres`, password `postgres` |
| Redis | localhost:6379 |
| NATS | nats://localhost:4222 (JetStream on, no auth) |
| Frontend dev server | http://localhost:5173 (not started by `task up`) |

NATS in this file is `nats:2.10-alpine` so the healthcheck can call the
monitoring endpoint. The server flag is still `-js`. The default `nats:2.10`
image has no shell.

The gateway dials `room-service:7001`. Board dials the same address for
`CheckMembership`. `FRONTEND_URL` is `http://localhost:5173`.

## Metrics gap

The Go services do not export Prometheus metrics. Grafana is provisioned
with the Prometheus datasource and one dashboard. The dashboard shows
Prometheus' own `up` series and states that WebSocket connections, NATS
consumer lag, and request p99 are not emitted. Traces are in Jaeger.

## Smoke results

Checked on this machine after `docker compose -f deploy/compose/docker-compose.yml down -v` and `task up`. The first request to readiness, immediately after `task up` returned, was HTTP 200.

| URL | Status |
|---|---|
| http://localhost:8080/healthz | 200 |
| http://localhost:8080/readyz | 200 `{"status":"ok"}` |
| http://localhost:8001/healthz | 200 |
| http://localhost:8002/healthz | 200 |
| http://localhost:8003/healthz | 200 |
| http://localhost:8004/healthz | 200 |
| http://localhost:16686/ | 200 |
| http://localhost:9090/-/healthy | 200 |
| http://localhost:3000/api/health | 200 |
| http://localhost:3000/d/whiteboard-dev/whiteboard-local | 200 (admin / admin) |

Jaeger UI: http://localhost:16686. After `POST /api/v1/rooms` with a local HS256 token, Jaeger listed services `gateway` and `room`. The trace has two spans: `gateway whiteboard.room.v1.RoomService/CreateRoom` and `room /whiteboard.room.v1.RoomService/CreateRoom`. That is the gateway-to-room call. A later board-history trace, with gateway, board, and room, is in `docs/assets/jaeger-trace.png`.

Grafana: http://localhost:3000. Dashboard: http://localhost:3000/d/whiteboard-dev/whiteboard-local. Datasource uid `prometheus`. The Go services still have no Prometheus exporter.

`schema_migrations` version is 6, not dirty. Roles `anon` and `authenticated` are `NOLOGIN`. `service_role` is `NOLOGIN` and `BYPASSRLS`.

Image sizes from `docker images` (uncompressed, including the distroless and busybox layers):

| Image | Size | Binary |
|---|---|---|
| whiteboard-gateway:dev | 53.4MB | 33,755,298 bytes |
| whiteboard-room:dev | 45.6MB | 27,762,850 bytes |
| whiteboard-board:dev | 57MB | 36,704,418 bytes |
| whiteboard-chat:dev | 56.8MB | 36,565,154 bytes |
| whiteboard-presence:dev | 41.6MB | 24,862,882 bytes |

The final stage is `gcr.io/distroless/static-debian12:nonroot` pinned to `sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab`. The process runs as uid 65532 (`nonroot`). Build used `golang:1.26-alpine` because `go.mod` says `go 1.26.0`. `CGO_ENABLED=0` and `-ldflags "-s -w"`. Several images are larger than 30MB because the stripped binaries themselves are that large.

The frontend is the TypeScript app under `frontend/`. Compose still has no frontend image.
