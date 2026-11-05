import crypto from "node:crypto";
import http from "node:http";

// Handshake to the gateway /ws and expect the unauthenticated error frame.
const origin = process.env.SMOKE_ORIGIN || "http://localhost:5173";
const host = process.env.SMOKE_HOST || "127.0.0.1";
const port = Number(process.env.SMOKE_PORT || "8088");

function clientFrame(text) {
  const payload = Buffer.from(text);
  const mask = crypto.randomBytes(4);
  const frame = Buffer.alloc(6 + payload.length);
  frame[0] = 0x81;
  frame[1] = 0x80 | payload.length;
  mask.copy(frame, 2);
  for (let i = 0; i < payload.length; i++) {
    frame[6 + i] = payload[i] ^ mask[i % 4];
  }
  return frame;
}

let done = false;
function finish(ok, message) {
  if (done) {
    return;
  }
  done = true;
  if (ok) {
    console.log(message);
    process.exit(0);
  }
  console.error(message);
  process.exit(1);
}

const key = crypto.randomBytes(16).toString("base64");
const req = http.request({
  host,
  port,
  path: "/ws",
  headers: {
    Connection: "Upgrade",
    Upgrade: "websocket",
    "Sec-WebSocket-Version": "13",
    "Sec-WebSocket-Key": key,
    Origin: origin,
  },
  timeout: 8000,
});

req.on("upgrade", (_res, socket) => {
  socket.write(clientFrame("nope"));
  const chunks = [];
  const timer = setTimeout(() => finish(false, "timed out waiting for a server frame"), 8000);
  socket.on("data", (buf) => {
    chunks.push(buf);
    const data = Buffer.concat(chunks);
    if (data.length < 2) {
      return;
    }
    const opcode = data[0] & 0x0f;
    if (opcode === 2 || opcode === 8) {
      clearTimeout(timer);
      socket.end();
      finish(true, `server frame opcode ${opcode}`);
    }
  });
  socket.on("end", () => {
    clearTimeout(timer);
    finish(false, "socket closed before a server frame");
  });
  socket.on("error", (err) => {
    clearTimeout(timer);
    finish(false, err.message);
  });
});

req.on("response", (res) => {
  finish(false, `handshake HTTP ${res.statusCode}`);
});
req.on("error", (err) => finish(false, err.message));
req.on("timeout", () => {
  req.destroy();
  finish(false, "handshake timeout");
});
req.end();
