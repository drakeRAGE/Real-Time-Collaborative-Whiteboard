/** Minimal protobuf writer for the forged-undo envelope. ASCII fields only. */

const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

export function ulid(now = Date.now()): string {
  let time = now;
  const chars = new Array<string>(26);
  for (let i = 9; i >= 0; i--) {
    chars[i] = CROCKFORD[time % 32] ?? '0';
    time = Math.floor(time / 32);
  }
  for (let i = 10; i < 26; i++) {
    chars[i] = CROCKFORD[Math.floor(Math.random() * 32)] ?? '0';
  }
  return chars.join('');
}

function varint(value: number): number[] {
  const out: number[] = [];
  let n = value >>> 0;
  while (n > 0x7f) {
    out.push((n & 0x7f) | 0x80);
    n >>>= 7;
  }
  out.push(n);
  return out;
}

function key(field: number, wire: number): number[] {
  return varint((field << 3) | wire);
}

function bytesOf(text: string): number[] {
  const out: number[] = [];
  for (let i = 0; i < text.length; i++) out.push(text.charCodeAt(i) & 0xff);
  return out;
}

function fieldString(field: number, text: string): number[] {
  const raw = bytesOf(text);
  return [...key(field, 2), ...varint(raw.length), ...raw];
}

function fieldMessage(field: number, inner: number[]): number[] {
  return [...key(field, 2), ...varint(inner.length), ...inner];
}

/** ClientEnvelope{client_msg_id, undo:{room_id}}. */
export function undoEnvelope(roomId: string, clientMsgId = ulid()): Uint8Array {
  const undo = fieldString(1, roomId);
  const body = [...fieldString(1, clientMsgId), ...fieldMessage(15, undo)];
  return Uint8Array.from(body);
}

export interface AckView {
  errorCode: number | null;
}

function readVarint(buf: Uint8Array, offset: number): [number, number] {
  let result = 0;
  let shift = 0;
  let i = offset;
  while (i < buf.length && shift < 35) {
    const byte = buf[i] ?? 0;
    i += 1;
    result += (byte & 0x7f) * 2 ** shift;
    if ((byte & 0x80) === 0) return [result, i];
    shift += 7;
  }
  return [result, i];
}

function walk(buf: Uint8Array): { field: number; bytes?: Uint8Array; value?: number }[] {
  const fields: { field: number; bytes?: Uint8Array; value?: number }[] = [];
  let i = 0;
  while (i < buf.length) {
    const [tag, next] = readVarint(buf, i);
    i = next;
    const field = tag >>> 3;
    const wire = tag & 7;
    if (wire === 0) {
      const [value, after] = readVarint(buf, i);
      fields.push({ field, value });
      i = after;
    } else if (wire === 2) {
      const [len, after] = readVarint(buf, i);
      const start = after;
      const end = start + len;
      fields.push({ field, bytes: buf.slice(start, end) });
      i = end;
    } else if (wire === 1) {
      i += 8;
    } else if (wire === 5) {
      i += 4;
    } else {
      break;
    }
  }
  return fields;
}

/** Pulls Ack.error.code from a ServerEnvelope, if this frame is an ack. */
export function ackErrorCode(frame: Uint8Array): number | null {
  const top = walk(frame);
  const ack = top.find((field) => field.field === 1 && field.bytes);
  if (!ack?.bytes) return null;
  const err = walk(ack.bytes).find((field) => field.field === 2 && field.bytes);
  if (!err?.bytes) return 0;
  const code = walk(err.bytes).find((field) => field.field === 1);
  return code?.value ?? 0;
}
