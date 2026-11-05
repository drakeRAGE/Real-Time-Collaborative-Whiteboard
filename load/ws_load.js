// WebSocket fan-out load. Gateway auth is Sec-WebSocket-Protocol: bearer, <jwt>.
// The hub allows 3 sockets per user and stroke_input is 20/s/user, so each VU
// needs its own user. K6_PLAN is the JSON from `go run ./load/harness -mode prep`.
//
// Default shape ramps to every VU in the plan (500 across 50 rooms when the
// plan was built that way): 5 strokes/s and 20 cursors/s each.
// K6_PROFILE=smoke caps the ramp at 20 VUs for 30s.

import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Counter, Trend, Rate } from 'k6/metrics';
import { classify, cursorEnvelope, joinEnvelope, latencyFromStamp, stampPoint, strokeEnvelope, ulid } from './proto.js';

const fanoutLatency = new Trend('fanout_latency', true);
const fanoutSamples = new Counter('fanout_samples');
const errors = new Rate('errors');

if (!__ENV.K6_TOKEN) {
  throw new Error(
    'K6_TOKEN is unset. Refusing to open WebSockets. GET /ws accepts a JWT in Sec-WebSocket-Protocol after the bearer token (see services/gateway/internal/ws/hub.go). A single token cannot drive this scenario: maxConnsPerUser is 3 and stroke_input is limited to 20/s/user. Create per-VU users with load/harness -mode prep and export the first token as K6_TOKEN.',
  );
}
if (!__ENV.K6_PLAN) {
  throw new Error(
    'K6_PLAN is unset. Refusing to guess rooms. Run: go run ./load/harness -mode prep -users 500 -per-room 10 -out <file>, then set K6_PLAN to that file and K6_TOKEN to its first token.',
  );
}

let planText = open(__ENV.K6_PLAN);
if (planText.charCodeAt(0) === 0xfeff) {
  planText = planText.slice(1);
} else if (planText.charCodeAt(0) === 0xef && planText.charCodeAt(1) === 0xbb && planText.charCodeAt(2) === 0xbf) {
  planText = planText.slice(3);
}
const plan = JSON.parse(planText);
if (!plan.ws || !Array.isArray(plan.vus) || plan.vus.length === 0) {
  throw new Error('K6_PLAN must be JSON with ws and a non-empty vus array');
}

const population = plan.vus.length;
const smoke = __ENV.K6_PROFILE === 'smoke';
const target = smoke ? Math.min(20, population) : population;

function stages() {
  if (smoke) {
    return [
      { duration: '10s', target },
      { duration: '20s', target },
    ];
  }
  const half = Math.max(1, Math.ceil(target / 2));
  return [
    { duration: '30s', target: Math.min(100, target) },
    { duration: '30s', target: Math.min(half, target) },
    { duration: '30s', target },
    { duration: '45s', target },
    { duration: '20s', target: 0 },
  ];
}

const sessionMs = Number(__ENV.K6_SESSION_MS || (smoke ? 12000 : 20000));

export const options = {
  scenarios: {
    fanout: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: stages(),
      gracefulRampDown: '15s',
      gracefulStop: '30s',
    },
  },
  thresholds: {
    fanout_latency: ['p(99)<150'],
    fanout_samples: ['count>0'],
    errors: ['rate<0.001'],
  },
};

function onFrame(data, selfId) {
  const msg = classify(data);
  if (msg.kind === 'ack' || msg.kind === 'error') {
    errors.add(msg.code ? 1 : 0);
    return;
  }
  // Only another user's echo is fan-out. A same-millisecond self-echo is 0ms and
  // hid the real duration when no iteration was allowed to finish.
  if (msg.kind === 'stroke' && msg.stamp && msg.userId && msg.userId !== selfId) {
    fanoutSamples.add(1);
    fanoutLatency.add(latencyFromStamp(msg.stamp, Date.now()));
  }
}

export default function () {
  const slot = plan.vus[(__VU - 1) % plan.vus.length];
  const params = {
    headers: {
      Origin: __ENV.K6_ORIGIN || 'http://localhost:5173',
      'Sec-WebSocket-Protocol': `bearer, ${slot.token}`,
    },
  };
  const res = ws.connect(plan.ws, params, (socket) => {
    // Close before the scenario interrupts the VU, or k6 reports no finished iteration
    // and an empty time trend as p99=0s.
    socket.setTimeout(() => socket.close(), sessionMs);
    socket.on('binaryMessage', (data) => onFrame(data, slot.userId));
    socket.on('open', () => {
      socket.sendBinary(joinEnvelope(slot.roomId, ulid(Date.now())));
      // Join is synchronous on the gateway. Strokes sent before it acks are rejected.
      socket.setTimeout(() => {
        socket.setInterval(() => {
          const now = Date.now();
          socket.sendBinary(strokeEnvelope(slot.roomId, ulid(now), ulid(now + 1), stampPoint(now)));
        }, 200);
        socket.setInterval(() => {
          const y = (__VU * 17) % 1000;
          socket.sendBinary(cursorEnvelope(slot.roomId, ulid(Date.now()), y, (y + 20) % 1000));
        }, 50);
      }, 1000);
    });
  });
  const ok = check(res, { 'ws upgrade 101': (r) => r && r.status === 101 });
  if (!ok) {
    errors.add(1);
    if (__ITER === 0) {
      console.log('upgrade status ' + (res && res.status) + ' error ' + (res && res.error));
    }
    // A refused handshake returns immediately. Sleeping keeps one VU from
    // opening hundreds of sockets before the next stage.
    sleep(Math.max(1, sessionMs / 1000));
  }
}
