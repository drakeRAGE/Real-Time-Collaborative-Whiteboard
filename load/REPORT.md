# WebSocket load report

Local Docker Compose (`deploy/compose/docker-compose.yml`). Gateway `http://localhost:8080/readyz` was 200 before the runs. k6 2.3.0 at `C:\Program Files\k6\k6.exe`. Each VU has its own user because the hub allows 3 sockets per user and `stroke_input` is 20/s/user. The plan JSON is produced by `load/harness` (`-mode prep`) and is not committed (it contains JWTs). `load/ws_load.js` refuses to start when `K6_TOKEN` or `K6_PLAN` is unset.

Auth matches the gateway: `Sec-WebSocket-Protocol: bearer, <jwt>`, `Origin: http://localhost:5173`. The plan's `ws` field must be `ws://localhost:8080/ws` when k6 runs on the host. A prep run inside the compose network writes `ws://gateway:8080/ws`, which the host cannot dial.

The 500 VU / 50 room ramp was not run. The 100 VU run already missed the fan-out SLO, and board-ingest was still tens of thousands of messages behind when that run finished.

## Smoke: 20 VUs, 2 rooms

```
$env:K6_PROFILE = 'smoke'
$env:K6_ORIGIN = 'http://localhost:5173'
$env:K6_PLAN = "$env:TEMP\k6-plan.json"   # ws rewritten to ws://localhost:8080/ws
$env:K6_TOKEN = <first plan token>
& 'C:\Program Files\k6\k6.exe' run --summary-export $env:TEMP\k6-smoke.json load\ws_load.js
```

Exit 0. Stages: 10s to 20 VUs, 20s hold, 10s graceful ramp-down. Runtime about 60s. Threshold lines in the console were checkmarks, but `fanout_latency` had no samples (avg = min = med = max = p90 = p95 = p99 = 0). An empty trend reports 0 and the `p(99)<150` check passes without measuring fan-out. Do not read this run as a passing latency result.

| Metric | Value |
| --- | --- |
| errors | 0.00% (0 / 26137) |
| fanout_latency p99 | 0 (no stroke samples) |
| ws upgrade | 20 sessions |
| ws_connecting | avg 12.66ms, med 5.71ms, p95 45.83ms, max 106.92ms |
| ws_msgs_sent | 26138 (435/s) |
| ws_msgs_received | 209121 (3484/s) |
| data received / sent | 23 MB / 2.8 MB |

Received traffic in this run was acks and cursor fan-out. Stroke frames had not come back before the scenario ended.

## 100 VUs, 10 rooms (largest run)

```
Remove-Item Env:K6_PROFILE
$env:K6_ORIGIN = 'http://localhost:5173'
$env:K6_PLAN = "$env:TEMP\k6-plan-100-host.json"   # 100 users, 10 rooms, ws://localhost:8080/ws
$env:K6_TOKEN = <first plan token>
& 'C:\Program Files\k6\k6.exe' run --summary-export $env:TEMP\k6-100.json load\ws_load.js
```

Exit 99. Scenario `fanout`: 30s to 100, 30s to 50, 30s to 100, 45s hold, 20s to 0, plus 10s graceful ramp-down. Wall clock 2m45s. `errors` stayed under 0.1%. `fanout_latency` did not.

| Metric | Value |
| --- | --- |
| errors | 0.00% (0 / 199131 acks) |
| fanout_samples | 2354 (14.3/s) |
| fanout_latency min | 120294 ms (2m 0s) |
| fanout_latency avg | 124585 ms (2m 5s) |
| fanout_latency med | 124077 ms |
| fanout_latency p90 | 128472 ms |
| fanout_latency p95 | 129092 ms |
| fanout_latency p99 | 2m 10s (threshold `p(99)<150` failed) |
| fanout_latency max | 130988 ms |
| ws upgrade checks | 3/3 status 101 |
| ws_sessions | 144 |
| ws_connecting | avg 37.6ms, med 18.0ms, p95 131.4ms, max 400.1ms |
| ws_msgs_sent | 199678 (1210/s) |
| ws_msgs_received | 1413208 (8565/s) |
| data received / sent | 150 MB / 23 MB |
| iterations | 3 completed, 141 interrupted (stage drops and ramp-down) |

The socket layer held the ramp: VUs sat at 100/100 for the hold, and every observed ack had error code 0. Stroke fan-out did not. 100 VUs at 5 strokes/s is about 500 strokes/s into `wb.board.stroke.v1`. Right after this run the BOARD stream had 65918 messages and two subjects, and the `board-ingest` consumer had delivered 18704 with 41763 still pending.

## Bottleneck

Board ingest cannot keep up with this stroke rate on the local Compose stack, and applied strokes share the ingest `Nats-Msg-Id` (the stroke ULID). JetStream deduplicates that id for 2 minutes on the BOARD stream (`pkg/natsx` `Duplicates: 2m`).

- While ingest is inside that window, `wb.board.stroke_applied.v1` is stored as a duplicate and never reaches the gateway fan-out consumer. Clients keep receiving acks and cursors, and `fanout_latency` stays empty. That is the smoke run.
- Once ingest lags past 2 minutes, the id has aged out of the duplicate window, the applied event is stored, and clients see the stroke about 2 minutes after they sent it. That is the 100 VU run: every one of the 2354 samples was between 2m 0s and 2m 11s, and most strokes were still queued on `board-ingest`.

The p99 SLO of 150ms was not met at 100 VUs. A 500 VU run would only deepen the same backlog, so it was not started.
