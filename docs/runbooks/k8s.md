# Kubernetes runbook

Commands assume the repo root and the tools from Phase 0: Docker, `kind`, `kubectl`, Helm, `task`, and Node.js.

The chart lives at `deploy/k8s/charts/whiteboard`. It installs gateway, room, board, chat, presence, the official NATS chart (JetStream on), and the official Bitnami Redis chart. Pods run as uid 65532 with a read-only root filesystem. Supabase settings are read from the Secret `whiteboard-supabase`, not from the image.

## Bootstrap

Build the five service images once:

```
docker compose -f deploy/compose/docker-compose.yml build
```

In the same shell, set the Supabase cloud values the Secret is filled from:

```
$env:DATABASE_URL = "postgres://..."
$env:SUPABASE_JWT_ISSUER = "https://<project-ref>.supabase.co/auth/v1"
$env:SUPABASE_JWKS_URL = "https://<project-ref>.supabase.co/auth/v1/.well-known/jwks.json"
```

`DATABASE_URL` must be reachable from the cluster (the Supabase pooler, or a Postgres the nodes can route to). `SUPABASE_JWT_ALLOW_HS256` stays false; the gateway fetches the JWKS URL at startup.

Create the cluster and install the stack:

```
task kind-down
task kind-up
```

`task kind-up` runs `deploy/k8s/kind-up.ps1`. That script:

1. Creates the cluster from `deploy/k8s/kind.yaml` (1 control-plane, 2 workers, host ports 8088 and 8443).
2. Installs Calico v3.30.3 so NetworkPolicy is enforced, then waits until every node is Ready.
3. Installs ingress-nginx (kind provider manifest, controller v1.13.2) on the control-plane.
4. Loads `whiteboard-gateway:dev`, `whiteboard-room:dev`, `whiteboard-board:dev`, `whiteboard-chat:dev`, and `whiteboard-presence:dev` into the cluster.
5. Creates Secret `whiteboard-supabase` in namespace `whiteboard` from the three environment variables (`issuer`, `jwksUrl`, `databaseUrl`).
6. Installs the chart with `values.yaml` and `values-dev.yaml` (ingress enabled).
7. Applies `deploy/k8s/netpol`.
8. Runs `deploy/k8s/smoke.ps1`.

Smoke checks `http://127.0.0.1:8088/readyz`, opens a WebSocket to `/ws` and expects the unauthenticated error frame, then runs a debug pod whose `curl` to `room:8001` must fail.

Re-run the checks alone with:

```
powershell -NoProfile -File deploy/k8s/smoke.ps1
```

## Upgrade

```
Remove-Item deploy/k8s/charts/whiteboard/charts/*-0.1.0.tgz -ErrorAction SilentlyContinue
helm dependency build deploy/k8s/charts/whiteboard
helm upgrade whiteboard deploy/k8s/charts/whiteboard -n whiteboard -f deploy/k8s/charts/whiteboard/values.yaml -f deploy/k8s/charts/whiteboard/values-dev.yaml
```

Helm will not repackage a local chart while its `0.1.0` archive is already in `charts/`. Deleting those archives first picks up chart edits. The NATS and Redis archives stay.

NetworkPolicy manifests are not part of the chart. After an upgrade that should keep them, apply them again:

```
kubectl apply -f deploy/k8s/netpol
```

## Rollback

```
helm history whiteboard -n whiteboard
helm rollback whiteboard <revision> -n whiteboard
```

Replace `<revision>` with the number from `helm history` that you want to restore.

## Consumer lag

Board uses durable consumer `board-ingest` on stream `BOARD`. Chat uses `chat-ingest` on stream `CHAT`. Monitoring port 8222 is not allowed by NetworkPolicy, so read it from inside the NATS pod's network namespace with a port-forward:

```
kubectl -n whiteboard port-forward pod/nats-0 8222:8222
```

In a second shell:

```
curl -s "http://127.0.0.1:8222/jsz?consumers=1"
```

`num_pending` on `board-ingest` and `chat-ingest` is the lag. Do this before changing board replicas. The board Deployment uses `maxUnavailable: 0` so a rollout keeps the consumer on a running pod.

## Scale the gateway

The gateway HorizontalPodAutoscaler owns the replica count (CPU 70%, minimum equal to `gateway.replicaCount`, maximum 10). Raise the floor so the autoscaler does not scale back down:

```
kubectl -n whiteboard patch hpa gateway --type merge -p "{\"spec\":{\"minReplicas\":3}}"
```

A custom WebSocket-session metric is not exported yet. The chart comment on the HPA is the place to add it beside the CPU target.

## Rotate secrets

Put the new issuer, JWKS URL, and database URL in the environment, then replace the Secret and restart the workloads that mount it. Presence does not use this Secret.

```
kubectl -n whiteboard create secret generic whiteboard-supabase --from-literal=issuer="$env:SUPABASE_JWT_ISSUER" --from-literal=jwksUrl="$env:SUPABASE_JWKS_URL" --from-literal=databaseUrl="$env:DATABASE_URL" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n whiteboard rollout restart deploy/gateway deploy/room deploy/board deploy/chat
kubectl -n whiteboard rollout status deploy/gateway
```

`http://127.0.0.1:8088/readyz` should return 200 before you restart the next environment.

## Tear down

```
task kind-down
```
