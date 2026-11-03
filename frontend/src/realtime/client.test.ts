import { afterEach, describe, expect, it, vi } from 'vitest';
import { backoffDelay, RealtimeClient, type Outbound, type SocketLike } from './client';
import { ClientEnvelope, ServerEnvelope } from './gen/whiteboard/realtime/v1/envelope';
import { Tool } from './gen/whiteboard/board/v1/board';
import { ulid } from './ulid';

const ROOM = '11111111-1111-4111-8111-111111111111';

class MockSocket implements SocketLike {
  static instances: MockSocket[] = [];
  binaryType = 'blob';
  readyState = 0;
  sent: Uint8Array[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(readonly url: string, readonly protocols?: string | string[]) {
    MockSocket.instances.push(this);
  }

  send(data: Uint8Array): void {
    this.sent.push(data.slice());
  }

  close(): void {
    this.readyState = 3;
    this.onclose?.();
  }

  serverOpen(): void {
    this.readyState = 1;
    this.onopen?.();
  }

  serverMessage(bytes: Uint8Array): void {
    this.onmessage?.({ data: bytes.slice().buffer });
  }
}

function ackFor(id: string): Uint8Array {
  return ServerEnvelope.encode(ServerEnvelope.fromPartial({
    ack: { clientMsgId: id },
  })).finish();
}

async function microtasks(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

function stroke(id: string): Outbound {
  return {
    strokeInput: {
      strokeId: id,
      roomId: ROOM,
      points: [{ x: 1, y: 2 }],
      color: '#112233',
      width: 4,
      tool: Tool.TOOL_PEN,
    },
  };
}

describe('RealtimeClient', () => {
  afterEach(() => {
    vi.useRealTimers();
    MockSocket.instances = [];
  });

  it('resolves send when the server acks the client_msg_id', async () => {
    vi.useFakeTimers();
    const client = new RealtimeClient({
      url: 'ws://localhost:8080/ws',
      getToken: async () => 'tok',
      WebSocket: MockSocket,
      random: () => 0.5,
      heartbeatMs: 60_000,
    });
    client.connect();
    await microtasks();
    const sock = MockSocket.instances[0];
    expect(sock?.protocols).toEqual(['bearer', 'tok']);
    sock?.serverOpen();
    const pending = client.send(stroke(ulid()));
    const sent = sock?.sent[0];
    expect(sent).toBeDefined();
    const env = ClientEnvelope.decode(sent!);
    sock?.serverMessage(ackFor(env.clientMsgId));
    await expect(pending).resolves.toMatchObject({ clientMsgId: env.clientMsgId });
    client.close();
  });

  it('replays an unacked stroke exactly once after reconnect and refreshes the token', async () => {
    vi.useFakeTimers();
    let n = 0;
    const getToken = vi.fn(async () => {
      n += 1;
      return `tok-${n}`;
    });
    const client = new RealtimeClient({
      url: 'ws://localhost:8080/ws',
      getToken,
      WebSocket: MockSocket,
      random: () => 0.5,
      heartbeatMs: 60_000,
    });
    client.connect();
    await microtasks();
    const first = MockSocket.instances[0];
    first?.serverOpen();
    const pending = client.send(stroke(ulid()));
    const original = ClientEnvelope.decode(first!.sent[0]!);
    first?.close();
    expect(client.state).toBe('reconnecting');

    await vi.advanceTimersByTimeAsync(999);
    expect(MockSocket.instances).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(getToken).toHaveBeenCalledTimes(2);
    const next = MockSocket.instances[1];
    expect(next?.protocols).toEqual(['bearer', 'tok-2']);
    expect(next?.sent).toHaveLength(0);
    next?.serverOpen();
    expect(next?.sent).toHaveLength(1);
    const replay = ClientEnvelope.decode(next!.sent[0]!);
    expect(replay.clientMsgId).toBe(original.clientMsgId);
    expect(replay.strokeInput?.strokeId).toBe(original.strokeInput?.strokeId);
    next?.serverMessage(ackFor(replay.clientMsgId));
    await expect(pending).resolves.toMatchObject({ clientMsgId: replay.clientMsgId });
    client.close();
  });

  it('waits out exponential backoff before opening the next socket', async () => {
    vi.useFakeTimers();
    const client = new RealtimeClient({
      url: 'ws://localhost:8080/ws',
      getToken: async () => 'tok',
      WebSocket: MockSocket,
      random: () => 0.5,
      heartbeatMs: 60_000,
    });
    expect(backoffDelay(0, () => 0.5)).toBe(1000);
    expect(backoffDelay(1, () => 0.5)).toBe(2000);
    expect(backoffDelay(5, () => 0.5)).toBe(30_000);

    client.connect();
    await microtasks();
    MockSocket.instances[0]?.close();
    await vi.advanceTimersByTimeAsync(999);
    expect(MockSocket.instances).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(MockSocket.instances).toHaveLength(2);

    MockSocket.instances[1]?.close();
    await vi.advanceTimersByTimeAsync(1999);
    expect(MockSocket.instances).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(MockSocket.instances).toHaveLength(3);
    client.close();
  });

  it('mints ULID ids the gateway accepts', () => {
    expect(ulid(1_700_000_000_000, () => 0)).toMatch(/^[0-7][0-9A-HJKMNP-TV-Z]{25}$/);
  });
});
