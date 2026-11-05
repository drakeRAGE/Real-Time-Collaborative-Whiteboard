// ASCII protobuf codec for ClientEnvelope strokes, cursors, and joins.
// Field numbers match proto/whiteboard/realtime/v1/envelope.proto.

const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

export function ulid(now) {
  let time = now;
  const chars = new Array(26);
  for (let i = 9; i >= 0; i--) {
    chars[i] = CROCKFORD[time % 32];
    time = Math.floor(time / 32);
  }
  for (let i = 10; i < 26; i++) {
    chars[i] = CROCKFORD[Math.floor(Math.random() * 32)];
  }
  return chars.join('');
}

function varint(value) {
  const out = [];
  let n = value >>> 0;
  while (n > 0x7f) {
    out.push((n & 0x7f) | 0x80);
    n >>>= 7;
  }
  out.push(n);
  return out;
}

function key(field, wire) {
  return varint((field << 3) | wire);
}

function ascii(text) {
  const out = [];
  for (let i = 0; i < text.length; i++) out.push(text.charCodeAt(i) & 0xff);
  return out;
}

function fieldString(field, text) {
  const raw = ascii(text);
  return key(field, 2).concat(varint(raw.length), raw);
}

function fieldVarint(field, value) {
  return key(field, 0).concat(varint(value));
}

function fieldDouble(field, value) {
  const buf = new ArrayBuffer(8);
  new DataView(buf).setFloat64(0, value, true);
  return key(field, 1).concat(Array.from(new Uint8Array(buf)));
}

function fieldMessage(field, inner) {
  return key(field, 2).concat(varint(inner.length), inner);
}

function pack(bytes) {
  return Uint8Array.from(bytes).buffer;
}

function point(x, y) {
  return fieldDouble(1, x).concat(fieldDouble(2, y));
}

export function joinEnvelope(roomId, clientMsgId) {
  const join = fieldString(1, roomId);
  return pack(fieldString(1, clientMsgId).concat(fieldMessage(10, join)));
}

// First point carries the client send time: x = unix seconds % 10000, y = ms.
// Both stay inside the protovalidate 0–10000 range.
export function stampPoint(now) {
  return { x: Math.floor(now / 1000) % 10000, y: now % 1000 };
}

export function latencyFromStamp(sent, now) {
  const nowX = Math.floor(now / 1000) % 10000;
  const nowY = now % 1000;
  let delta = (nowX - sent.x) * 1000 + (nowY - sent.y);
  if (delta < -500) delta += 10000 * 1000;
  if (delta < 0) delta = 0;
  return delta;
}

export function strokeEnvelope(roomId, strokeId, clientMsgId, sent) {
  const pts = fieldMessage(3, point(sent.x, sent.y)).concat(fieldMessage(3, point(10, 10)));
  const stroke = fieldString(1, strokeId)
    .concat(fieldString(2, roomId))
    .concat(pts)
    .concat(fieldString(4, '#112233'))
    .concat(fieldDouble(5, 2))
    .concat(fieldVarint(6, 1));
  return pack(fieldString(1, clientMsgId).concat(fieldMessage(12, stroke)));
}

export function cursorEnvelope(roomId, clientMsgId, x, y) {
  const cursor = fieldString(1, roomId).concat(fieldDouble(2, x), fieldDouble(3, y));
  return pack(fieldString(1, clientMsgId).concat(fieldMessage(18, cursor)));
}

function readVarint(buf, offset) {
  let result = 0;
  let shift = 0;
  let i = offset;
  while (i < buf.length && shift < 35) {
    const byte = buf[i];
    i += 1;
    result += (byte & 0x7f) * 2 ** shift;
    if ((byte & 0x80) === 0) return [result, i];
    shift += 7;
  }
  return [result, i];
}

function readDouble(buf, offset) {
  const view = new DataView(buf.buffer, buf.byteOffset + offset, 8);
  return view.getFloat64(0, true);
}

function walk(buf) {
  const fields = [];
  let i = 0;
  while (i < buf.length) {
    const tag = readVarint(buf, i);
    i = tag[1];
    const field = tag[0] >>> 3;
    const wire = tag[0] & 7;
    if (wire === 0) {
      const value = readVarint(buf, i);
      fields.push({ field, value: value[0] });
      i = value[1];
    } else if (wire === 2) {
      const len = readVarint(buf, i);
      const start = len[1];
      const end = start + len[0];
      fields.push({ field, bytes: buf.subarray(start, end) });
      i = end;
    } else if (wire === 1) {
      fields.push({ field, number: readDouble(buf, i) });
      i += 8;
    } else if (wire === 5) {
      i += 4;
    } else {
      break;
    }
  }
  return fields;
}

function asBytes(data) {
  if (data instanceof Uint8Array) return data;
  if (typeof data === 'string') {
    const out = new Uint8Array(data.length);
    for (let i = 0; i < data.length; i++) out[i] = data.charCodeAt(i) & 0xff;
    return out;
  }
  if (data && data.buffer && typeof data.byteLength === 'number' && typeof data.byteOffset === 'number') {
    return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
  }
  return new Uint8Array(data);
}

function textOf(bytes) {
  let out = '';
  for (let i = 0; i < bytes.length; i++) out += String.fromCharCode(bytes[i]);
  return out;
}

/** @returns {{kind:string, code?:number, userId?:string, stamp?:{x:number,y:number}}} */
export function classify(data) {
  const top = walk(asBytes(data));
  const stroke = top.find((field) => field.field === 13 && field.bytes);
  if (stroke) {
    const added = walk(stroke.bytes);
    const body = added.find((field) => field.field === 1 && field.bytes);
    if (!body) return { kind: 'stroke' };
    const fields = walk(body.bytes);
    const user = fields.find((field) => field.field === 3 && field.bytes);
    const points = fields.filter((field) => field.field === 4 && field.bytes);
    const first = points[0];
    if (!first) return { kind: 'stroke', userId: user ? textOf(user.bytes) : '' };
    const coords = walk(first.bytes);
    const x = coords.find((field) => field.field === 1 && field.number !== undefined);
    const y = coords.find((field) => field.field === 2 && field.number !== undefined);
    if (!x || !y) return { kind: 'stroke', userId: user ? textOf(user.bytes) : '' };
    return { kind: 'stroke', userId: user ? textOf(user.bytes) : '', stamp: { x: x.number, y: y.number } };
  }
  const err = top.find((field) => field.field === 2 && field.bytes);
  if (err) {
    const code = walk(err.bytes).find((field) => field.field === 1);
    return { kind: 'error', code: code ? code.value : 0 };
  }
  const ack = top.find((field) => field.field === 1 && field.bytes);
  if (ack) {
    const nested = walk(ack.bytes).find((field) => field.field === 2 && field.bytes);
    if (!nested) return { kind: 'ack', code: 0 };
    const code = walk(nested.bytes).find((field) => field.field === 1);
    return { kind: 'ack', code: code ? code.value : 0 };
  }
  return { kind: 'other' };
}
