# WebSocket load report

Local Docker Compose (`deploy/compose/docker-compose.yml`). Gateway `http://localhost:8080/readyz` was 200 before the runs. k6 2.3.0 at `C:\Program Files\k6\k6.exe`. Each VU has its own user because the hub allows 3 sockets per user and `stroke_input` is 20/s/user. The plan JSON is produced by `load/harness` (`-mode prep`) and is not committed (it contains JWTs). `load/ws_load.js` refuses to start when `K6_TOKEN` or `K6_PLAN` is unset.

Auth matches the gateway: `Sec-WebSocket-Protocol: bearer, <jwt>`, `Origin: http://localhost:5173`. The plan's `ws` field must be `ws://localhost:8080/ws` when k6 runs on the host. A prep run inside the compose network writes `ws://gateway:8080/ws`, which the host cannot dial. Write that file as UTF-8 without a BOM; k6's `JSON.parse` rejects a BOM.

The 500 VU / 50 room ramp was not run. After hub/fanout tuning on 2026-11-08, smoke p99 is **173 ms** (still over the 150 ms threshold by 23 ms; median **37 ms**). Pre-tuning smoke p99 was **471 ms**. A larger ramp is still unlikely to meet the SLO until p99 clears 150 ms at smoke load.

## Smoke: 20 VUs (post hub/fanout tuning, 2026-11-08)

`Hub.Broadcast` of `CursorMoved` now uses a one-slot channel, and the write pump drains `send` (strokes, acks, room frames) before that slot. A newer cursor replaces the queued one. Stroke fanout is otherwise unchanged: one protobuf marshal, shared bytes, no Redis on `stroke_applied`. Gateway image rebuilt with `docker compose -f deploy/compose/docker-compose.yml up -d --build gateway`. `/readyz` was 200. Exit **99**. Wall clock 41.5s. Summary file: `%TEMP%\k6-smoke-cursor.json`. The threshold line in that run is `p(99)=173ms`.

| Metric | Value |
| --- | --- |
| errors | 0.00% (0 / 13899 acks) |
| ws upgrade | 51/51 status 101 |
| fanout_samples | 21785 (524/s) |
| fanout_latency min | 10 ms |
| fanout_latency avg | 44.0 ms |
| fanout_latency med | 37 ms |
| fanout_latency p90 | 67 ms |
| fanout_latency p95 | 89 ms |
| fanout_latency p99 | **173 ms** (threshold `p(99)<150` failed) |
| fanout_latency max | 607 ms |
| ws_sessions | 51 |
| ws_connecting | avg 8.47ms, med 5.89ms, p90 15.04ms, p95 24.43ms, max 32.13ms |
| ws_msgs_sent | 13922 |
| ws_msgs_received | 121132 |
| data received / sent | 15 MB / 1.5 MB |

p90 and p95 are under 150ms. p99 is 23ms over the SLO. The max stayed near 600ms, so the cursor queue was the broad tail and not every slow sample. Board ingest still applies strokes one at a time (`MaxAckPending: 1`), with CheckMembership, a room advisory lock, a Redis append, and a JetStream publish before this gateway fanout runs. That path was not part of this attempt.

## Smoke: 20 VUs, 2 rooms (pre-tuning)

```
$env:K6_PROFILE = 'smoke'
$env:K6_ORIGIN = 'http://localhost:5173'
$env:K6_PLAN = "$env:TEMP\k6-plan-smoke.json"   # ws rewritten to ws://localhost:8080/ws, no BOM
$env:K6_TOKEN = <first plan token>
& 'C:\Program Files\k6\k6.exe' run --summary-export $env:TEMP\k6-smoke-clean.json load\ws_load.js
```

Exit 99. Stages: 10s to 20 VUs, 20s hold, then graceful ramp-down. Wall clock about 42s. `fanout_samples` is a real count. `fanout_latency` is the delay from the client timestamp embedded in the first stroke point to receipt on another user's socket. Same-user echoes are not counted.

| Metric | Value |
| --- | --- |
| errors | 0.00% (0 / 13702 acks) |
| ws upgrade | 51/51 status 101 |
| fanout_samples | 21883 (527/s) |
| fanout_latency min | 11 ms |
| fanout_latency avg | 90.4 ms |
| fanout_latency med | 48 ms |
| fanout_latency p90 | 225 ms |
| fanout_latency p95 | 311 ms |
| fanout_latency p99 | 471 ms (threshold `p(99)<150` failed) |
| fanout_latency max | 655 ms |
| ws_sessions | 51 (each VU holds the socket for 12s, then may reconnect) |
| ws_connecting | avg 15.5ms, med 6.4ms, p95 40.1ms, max 237.5ms |
| ws_msgs_sent | 13730 |
| ws_msgs_received | 125278 |
| data received / sent | 15 MB / 1.5 MB |

## Earlier runs, before applied events had their own id

An empty `fanout_latency` trend reports 0, so `p(99)<150` can pass with no samples. The first smoke did that: 20 sessions, 0 stroke samples, exit 0. Do not read that run as a passing latency result.

A 100 VU / 10 room run then recorded 2354 samples, every one between 2m 0s and 2m 11s, and exited 99. Board ingest published `wb.board.stroke_applied.v1` with the same `Nats-Msg-Id` as the ingest stroke. JetStream deduplicates that id for 2 minutes on the BOARD stream, so the applied event was dropped until the id aged out. Applied board events and `wb.chat.message_sent.v1` now use a different id. The pre-tuning smoke is the measurement after that id change. The 100 VU run was not repeated.
