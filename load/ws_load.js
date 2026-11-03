// WebSocket fan-out load. Gateway auth is Sec-WebSocket-Protocol: bearer, <jwt>.
// The hub allows 3 sockets per user and stroke_input is 20/s/user, so each VU
// needs its own user. K6_PLAN is the JSON from `go run ./load/harness -mode prep`.
//
// Default shape ramps to every VU in the plan (500 across 50 rooms when the
// plan was built that way): 5 strokes/s and 20 cursors/s each.
// K6_PROFILE=smoke caps the ramp at 20 VUs for 30s.

import ws from 'k6/ws';
import { check } from 'k6';
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

const plan = JSON.parse(open(__ENV.K6_PLAN));
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

export const options = {
  scenarios: {
    fanout: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: stages(),
      gracefulRampDown: '10s',
    },
  },
  thresholds: {
    fanout_latency: ['p(99)<150'],
    errors: ['rate<0.001'],
  },
};

export default function () {
  const slot = plan.vus[(__VU - 1) % plan.vus.length];
  const params = {
    headers: {
      Origin: __ENV.K6_ORIGIN || 'http://localhost:5173',
      'Sec-WebSocket-Protocol': `bearer, ${slot.token}`,
    },
  };
  const res = ws.connect(plan.ws, params, (socket) => {
    socket.on('binaryMessage', (data) => {
      const msg = classify(data);
      if (msg.kind === 'ack' || msg.kind === 'error') {
        errors.add(msg.code ? 1 : 0);
        return;
      }
      if (msg.kind === 'stroke' && msg.stamp) {
        fanoutSamples.add(1);
        fanoutLatency.add(latencyFromStamp(msg.stamp, Date.now()));
      }
    });
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
  if (!ok) errors.add(1);
}
