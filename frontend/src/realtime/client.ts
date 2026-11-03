import type { BoardCleared, RedoApplied, ShapeAdded, StrokeAdded, UndoApplied } from './gen/whiteboard/board/v1/board';
import type { Message as ChatMessage } from './gen/whiteboard/chat/v1/chat';
import type { CursorMoved, UserOffline, UserOnline } from './gen/whiteboard/presence/v1/presence';
import {
  Ack,
  type CallEnded,
  type CallJoin,
  type CallLeave,
  type CallSignal,
  type CallSignalRelayed,
  type CallState,
  type CallToggle,
  type CallToggled,
  type CallUserJoined,
  type CallUserLeft,
  type ChatSend,
  type ClearInput,
  ClientEnvelope,
  type CursorMove,
  type Error as StatusError,
  type JoinRoom,
  type LeaveRoom,
  type RedoInput,
  type RoomState,
  ServerEnvelope,
  type ShapeInput,
  type StrokeInput,
  type UndoInput,
} from './gen/whiteboard/realtime/v1/envelope';
import { ulid } from './ulid';

export type ConnectionState = 'connecting' | 'open' | 'reconnecting' | 'closed';

export type Outbound =
  | { joinRoom: JoinRoom }
  | { leaveRoom: LeaveRoom }
  | { strokeInput: StrokeInput }
  | { shapeInput: ShapeInput }
  | { clear: ClearInput }
  | { undo: UndoInput }
  | { redo: RedoInput }
  | { chatSend: ChatSend }
  | { cursorMove: CursorMove }
  | { callJoin: CallJoin }
  | { callLeave: CallLeave }
  | { callSignal: CallSignal }
  | { callToggle: CallToggle };

export interface ServerEvents {
  ack: Ack;
  error: StatusError;
  roomState: RoomState;
  userOnline: UserOnline;
  userOffline: UserOffline;
  strokeApplied: StrokeAdded;
  shapeApplied: ShapeAdded;
  boardCleared: BoardCleared;
  undoApplied: UndoApplied;
  redoApplied: RedoApplied;
  chatMessage: ChatMessage;
  cursorMoved: CursorMoved;
  callState: CallState;
  callUserJoined: CallUserJoined;
  callUserLeft: CallUserLeft;
  callEnded: CallEnded;
  callToggled: CallToggled;
  callSignal: CallSignalRelayed;
}

export interface SocketLike {
  binaryType: string;
  readyState: number;
  send(data: Uint8Array): void;
  close(): void;
  onopen: (() => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
  onclose: (() => void) | null;
  onerror: (() => void) | null;
}

export type SocketCtor = new (url: string, protocols?: string | string[]) => SocketLike;

export interface RealtimeClientOptions {
  url: string;
  /** Called on every socket open, including reconnects. Return a fresh access token. */
  getToken: () => Promise<string>;
  WebSocket?: SocketCtor;
  random?: () => number;
  heartbeatMs?: number;
  ackTimeoutMs?: number;
}

interface Pending {
  resolve: (ack: Ack) => void;
  reject: (err: Error) => void;
  timer: ReturnType<typeof setTimeout> | null;
}

const OPEN = 1;

/**
 * Full jittered delay. `random` of 0.5 yields exact 1s, 2s, 4s, 8s, 16s, then 30s.
 * The result stays inside 1s–30s.
 */
export function backoffDelay(attempt: number, random: () => number): number {
  const exp = Math.min(30_000, 1000 * 2 ** attempt);
  const jitter = (random() - 0.5) * exp * 0.2;
  return Math.round(Math.min(30_000, Math.max(1000, exp + jitter)));
}

function isDurable(payload: Outbound): boolean {
  return 'strokeInput' in payload || 'chatSend' in payload;
}

export class AckError extends Error {
  readonly ack: Ack;
  constructor(ack: Ack) {
    super(ack.error?.message || 'request rejected');
    this.name = 'AckError';
    this.ack = ack;
  }
}

type Listener = (payload: ServerEvents[keyof ServerEvents]) => void;

/**
 * One binary WebSocket to the gateway. Strokes and chat wait in a FIFO queue
 * until they are acked, and are written again once after the socket drops.
 * The browser answers protocol pings; `heartbeatMs` closes a socket that is
 * no longer OPEN so the backoff path can run.
 */
export class RealtimeClient {
  private readonly url: string;
  private readonly getToken: () => Promise<string>;
  private readonly Socket: SocketCtor;
  private readonly random: () => number;
  private readonly heartbeatMs: number;
  private readonly ackTimeoutMs: number;

  private _state: ConnectionState = 'closed';
  private intentional = false;
  private everOpen = false;
  private opening = false;
  private attempt = 0;
  private ws: SocketLike | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private beat: ReturnType<typeof setInterval> | null = null;
  private readonly queue: ClientEnvelope[] = [];
  private readonly sent = new Set<string>();
  private readonly pending = new Map<string, Pending>();
  private readonly listeners = new Map<keyof ServerEvents, Set<Listener>>();
  private readonly stateListeners = new Set<(state: ConnectionState) => void>();

  constructor(options: RealtimeClientOptions) {
    this.url = options.url;
    this.getToken = options.getToken;
    this.Socket = options.WebSocket ?? (globalThis.WebSocket as unknown as SocketCtor);
    this.random = options.random ?? Math.random;
    this.heartbeatMs = options.heartbeatMs ?? 30_000;
    this.ackTimeoutMs = options.ackTimeoutMs ?? 10_000;
  }

  get state(): ConnectionState {
    return this._state;
  }

  connect(): void {
    if (this.intentional || this.ws || this.opening || this.timer) return;
    if (this._state === 'closed') this.setState('connecting');
    void this.openSocket();
  }

  /** Drop backoff and open again. Used by the connection banner. */
  retry(): void {
    if (this.intentional) return;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.attempt = 0;
    if (this._state === 'open') return;
    if (this.ws) {
      const sock = this.ws;
      this.ws = null;
      sock.close();
    }
    if (this._state === 'closed') this.setState('connecting');
    void this.openSocket();
  }

  close(): void {
    this.intentional = true;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.stopHeartbeat();
    this.rejectAll(new Error('closed'));
    const sock = this.ws;
    this.ws = null;
    sock?.close();
    this.setState('closed');
  }

  send(payload: Outbound): Promise<Ack> {
    const env = ClientEnvelope.fromPartial({ clientMsgId: ulid(), ...payload });
    const promise = this.createPending(env.clientMsgId);
    const durable = isDurable(payload);
    if (durable) this.queue.push(env);
    if (this._state === 'open' && this.ws?.readyState === OPEN) {
      this.write(env);
    } else if (!durable) {
      this.fail(env.clientMsgId, new Error('not connected'));
    }
    return promise;
  }

  on<K extends keyof ServerEvents>(type: K, handler: (payload: ServerEvents[K]) => void): () => void {
    let set = this.listeners.get(type);
    if (!set) {
      set = new Set();
      this.listeners.set(type, set);
    }
    const wrapped = handler as Listener;
    set.add(wrapped);
    return () => set.delete(wrapped);
  }

  onState(handler: (state: ConnectionState) => void): () => void {
    this.stateListeners.add(handler);
    return () => this.stateListeners.delete(handler);
  }

  private async openSocket(): Promise<void> {
    if (this.intentional || this.ws || this.opening) return;
    this.opening = true;
    try {
      const token = await this.getToken();
      if (this.intentional || this.ws) return;
      this.attach(token);
    } catch {
      if (!this.intentional) this.schedule();
    } finally {
      this.opening = false;
    }
  }

  private attach(token: string): void {
    const sock = new this.Socket(this.url, ['bearer', token]);
    sock.binaryType = 'arraybuffer';
    this.ws = sock;
    sock.onopen = () => {
      if (this.ws !== sock) return;
      this.attempt = 0;
      this.everOpen = true;
      this.setState('open');
      this.startHeartbeat();
      this.flush();
    };
    sock.onmessage = (ev) => {
      if (this.ws !== sock) return;
      this.handleMessage(ev.data);
    };
    sock.onerror = () => undefined;
    sock.onclose = () => {
      if (this.ws !== sock) return;
      this.ws = null;
      this.stopHeartbeat();
      this.parkInflight();
      if (this.intentional) {
        this.setState('closed');
        return;
      }
      this.schedule();
    };
  }

  private schedule(): void {
    if (this.intentional) return;
    if (this.timer) clearTimeout(this.timer);
    this.setState(this.everOpen ? 'reconnecting' : 'connecting');
    const delay = backoffDelay(this.attempt, this.random);
    this.attempt += 1;
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.openSocket();
    }, delay);
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    if (this.heartbeatMs <= 0) return;
    this.beat = setInterval(() => {
      const sock = this.ws;
      if (sock && sock.readyState === OPEN) return;
      if (sock && this.ws === sock) sock.close();
    }, this.heartbeatMs);
  }

  private stopHeartbeat(): void {
    if (this.beat) clearInterval(this.beat);
    this.beat = null;
  }

  private flush(): void {
    for (const env of this.queue) {
      if (this.sent.has(env.clientMsgId)) continue;
      this.write(env);
    }
  }

  private write(env: ClientEnvelope): void {
    this.ws?.send(ClientEnvelope.encode(env).finish());
    this.sent.add(env.clientMsgId);
    this.arm(env.clientMsgId);
  }

  /** Unacked durable messages stay queued; their ack timers wait for the replay. */
  private parkInflight(): void {
    for (const env of this.queue) {
      this.sent.delete(env.clientMsgId);
      const slot = this.pending.get(env.clientMsgId);
      if (slot?.timer) {
        clearTimeout(slot.timer);
        slot.timer = null;
      }
    }
    for (const [id, slot] of this.pending) {
      if (this.queue.some((env) => env.clientMsgId === id)) continue;
      if (slot.timer) clearTimeout(slot.timer);
      this.pending.delete(id);
      slot.reject(new Error('disconnected'));
    }
  }

  private handleMessage(data: unknown): void {
    const bytes = toBytes(data);
    if (!bytes) return;
    let env: ServerEnvelope;
    try {
      env = ServerEnvelope.decode(bytes);
    } catch {
      return;
    }
    this.dispatch(env);
  }

  private dispatch(env: ServerEnvelope): void {
    if (env.ack) {
      this.settle(env.ack);
      this.emit('ack', env.ack);
      return;
    }
    if (env.error) return this.emit('error', env.error);
    if (env.roomState) return this.emit('roomState', env.roomState);
    if (env.userOnline) return this.emit('userOnline', env.userOnline);
    if (env.userOffline) return this.emit('userOffline', env.userOffline);
    if (env.strokeApplied) return this.emit('strokeApplied', env.strokeApplied);
    if (env.shapeApplied) return this.emit('shapeApplied', env.shapeApplied);
    if (env.boardCleared) return this.emit('boardCleared', env.boardCleared);
    if (env.undoApplied) return this.emit('undoApplied', env.undoApplied);
    if (env.redoApplied) return this.emit('redoApplied', env.redoApplied);
    if (env.chatMessage) return this.emit('chatMessage', env.chatMessage);
    if (env.cursorMoved) return this.emit('cursorMoved', env.cursorMoved);
    if (env.callState) return this.emit('callState', env.callState);
    if (env.callUserJoined) return this.emit('callUserJoined', env.callUserJoined);
    if (env.callUserLeft) return this.emit('callUserLeft', env.callUserLeft);
    if (env.callEnded) return this.emit('callEnded', env.callEnded);
    if (env.callToggled) return this.emit('callToggled', env.callToggled);
    if (env.callSignal) return this.emit('callSignal', env.callSignal);
  }

  private settle(ack: Ack): void {
    const slot = this.pending.get(ack.clientMsgId);
    if (!slot) return;
    if (slot.timer) clearTimeout(slot.timer);
    this.pending.delete(ack.clientMsgId);
    this.sent.delete(ack.clientMsgId);
    const index = this.queue.findIndex((env) => env.clientMsgId === ack.clientMsgId);
    if (index >= 0) this.queue.splice(index, 1);
    if (ack.error && (ack.error.code !== 0 || ack.error.message !== '')) {
      slot.reject(new AckError(ack));
      return;
    }
    slot.resolve(ack);
  }

  private emit<K extends keyof ServerEvents>(type: K, payload: ServerEvents[K]): void {
    const set = this.listeners.get(type);
    if (!set) return;
    for (const handler of set) handler(payload);
  }

  private createPending(id: string): Promise<Ack> {
    return new Promise<Ack>((resolve, reject) => {
      this.pending.set(id, { resolve, reject, timer: null });
    });
  }

  private arm(id: string): void {
    const slot = this.pending.get(id);
    if (!slot) return;
    if (slot.timer) clearTimeout(slot.timer);
    slot.timer = setTimeout(() => {
      this.pending.delete(id);
      this.sent.delete(id);
      const index = this.queue.findIndex((env) => env.clientMsgId === id);
      if (index >= 0) this.queue.splice(index, 1);
      slot.reject(new Error('ack timeout'));
    }, this.ackTimeoutMs);
  }

  private fail(id: string, error: Error): void {
    const slot = this.pending.get(id);
    if (!slot) return;
    if (slot.timer) clearTimeout(slot.timer);
    this.pending.delete(id);
    queueMicrotask(() => slot.reject(error));
  }

  private rejectAll(error: Error): void {
    for (const slot of this.pending.values()) {
      if (slot.timer) clearTimeout(slot.timer);
      slot.reject(error);
    }
    this.pending.clear();
    this.queue.splice(0, this.queue.length);
    this.sent.clear();
  }

  private setState(next: ConnectionState): void {
    if (this._state === next) return;
    this._state = next;
    for (const handler of this.stateListeners) handler(next);
  }
}

function toBytes(data: unknown): Uint8Array | null {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  return null;
}
