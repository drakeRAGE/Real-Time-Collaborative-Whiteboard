import { describe, expect, it } from 'vitest';
import { MediaKind, ClientEnvelope } from './gen/whiteboard/realtime/v1/envelope';
import { callSignalPayload, parseCallSignal, readIceServers } from './signal';
import { ulid } from './ulid';

const ROOM = '11111111-1111-4111-8111-111111111111';
const PEER = '22222222-2222-4222-8222-222222222222';

describe('call signaling shapes', () => {
  it('round-trips a simple-peer signal inside call_signal', () => {
    const signal = { type: 'offer', sdp: 'v=0' };
    const env = ClientEnvelope.fromPartial({
      clientMsgId: ulid(1_700_000_000_000, () => 0),
      callSignal: {
        roomId: ROOM,
        targetUserId: PEER,
        payload: callSignalPayload(signal),
      },
    });
    const decoded = ClientEnvelope.decode(ClientEnvelope.encode(env).finish());
    expect(decoded.callSignal?.roomId).toBe(ROOM);
    expect(decoded.callSignal?.targetUserId).toBe(PEER);
    expect(decoded.callSignal?.payload).toBeDefined();
    expect(parseCallSignal(decoded.callSignal!.payload)).toEqual(signal);
  });

  it('encodes mic and camera toggles as call_toggle', () => {
    const env = ClientEnvelope.fromPartial({
      clientMsgId: ulid(1_700_000_000_000, () => 0),
      callToggle: { roomId: ROOM, media: MediaKind.MEDIA_KIND_MIC, enabled: false },
    });
    const decoded = ClientEnvelope.decode(ClientEnvelope.encode(env).finish());
    expect(decoded.callToggle).toMatchObject({
      roomId: ROOM,
      media: MediaKind.MEDIA_KIND_MIC,
      enabled: false,
    });
    expect(decoded.callSignal).toBeUndefined();
  });

  it('reads ICE servers from JSON and falls back when the value is unusable', () => {
    expect(readIceServers(undefined)[0]?.urls).toBe('stun:stun.l.google.com:19302');
    expect(readIceServers('{"urls":"stun:example"}')).toEqual([{ urls: 'stun:stun.l.google.com:19302' }]);
    expect(readIceServers('[{"urls":"stun:example:3478","username":"u","credential":"p"}]')).toEqual([
      { urls: 'stun:example:3478', username: 'u', credential: 'p' },
    ]);
  });
});
