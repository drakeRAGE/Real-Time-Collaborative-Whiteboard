import { useEffect, useRef, useState } from 'react';
import { FiMic, FiMicOff, FiPhone, FiPhoneOff, FiVideo, FiVideoOff } from 'react-icons/fi';
import Peer, { type Instance as PeerInstance } from 'simple-peer';
import { useSocket } from '../context/SocketProvider';
import { useToast } from '../context/ToastContext';
import { MediaKind, type CallParticipant } from '../realtime/gen/whiteboard/realtime/v1/envelope';
import { callSignalPayload, parseCallSignal, readIceServers } from '../realtime/signal';
import RemoteVideo from './RemoteVideo';

type LinkState = 'connecting' | 'connected' | 'failed';

function destroyPeer(peer: PeerInstance): void {
  try {
    peer.destroy();
  } catch (err) {
    void err;
  }
}

export default function VideoCallSession({ roomId, userId, username }: { roomId: string; userId: string | null; username: string }) {
  const { client } = useSocket();
  const toast = useToast();
  const [inCall, setInCall] = useState(false);
  const [micEnabled, setMicEnabled] = useState(true);
  const [cameraEnabled, setCameraEnabled] = useState(true);
  const [participants, setParticipants] = useState<CallParticipant[]>([]);
  const [streams, setStreams] = useState<Record<string, MediaStream>>({});
  const [links, setLinks] = useState<Record<string, LinkState>>({});
  const localVideoRef = useRef<HTMLVideoElement>(null);
  const localStreamRef = useRef<MediaStream | null>(null);
  const peersRef = useRef<Record<string, PeerInstance>>({});
  const inCallRef = useRef(false);
  const clientRef = useRef(client);
  clientRef.current = client;

  const stopLocal = () => {
    localStreamRef.current?.getTracks().forEach((track) => track.stop());
    localStreamRef.current = null;
    if (localVideoRef.current) localVideoRef.current.srcObject = null;
  };

  const leaveCall = (notify: boolean) => {
    if (!inCallRef.current) return;
    inCallRef.current = false;
    const rt = clientRef.current;
    if (notify && rt && roomId) {
      void rt.send({ callLeave: { roomId } }).catch(() => undefined);
    }
    for (const peer of Object.values(peersRef.current)) destroyPeer(peer);
    peersRef.current = {};
    stopLocal();
    setLinks({});
    setStreams({});
    setInCall(false);
    setParticipants([]);
  };

  const leaveRef = useRef(leaveCall);
  leaveRef.current = leaveCall;

  useEffect(() => () => leaveRef.current(true), []);

  useEffect(() => {
    if (!client || !roomId || !inCall) return;
    const iceServers = readIceServers(import.meta.env.VITE_ICE_SERVERS);
    const openPeer = (id: string, initiator: boolean, signal?: object) => {
      const stream = localStreamRef.current;
      if (!stream || peersRef.current[id]) {
        if (signal && peersRef.current[id]) peersRef.current[id]?.signal(signal);
        return;
      }
      const peer = new Peer({ initiator, trickle: false, stream, config: { iceServers } });
      peersRef.current[id] = peer;
      setLinks((prev) => ({ ...prev, [id]: 'connecting' }));
      peer.on('signal', (data) => {
        void client.send({
          callSignal: { roomId, targetUserId: id, payload: callSignalPayload(data) },
        }).catch(() => undefined);
      });
      peer.on('stream', (remote) => setStreams((prev) => ({ ...prev, [id]: remote })));
      peer.on('connect', () => setLinks((prev) => ({ ...prev, [id]: 'connected' })));
      peer.on('error', () => setLinks((prev) => ({ ...prev, [id]: 'failed' })));
      peer.on('close', () => {
        delete peersRef.current[id];
        setLinks((prev) => dropKey(prev, id));
        setStreams((prev) => dropKey(prev, id));
      });
      if (signal) peer.signal(signal);
    };

    const offState = client.on('callState', (msg) => {
      if (msg.roomId === roomId) setParticipants(msg.participants);
    });
    const offJoined = client.on('callUserJoined', (msg) => {
      if (msg.roomId !== roomId) return;
      setParticipants(msg.participants);
      if (msg.userId !== userId) openPeer(msg.userId, true);
    });
    const offLeft = client.on('callUserLeft', (msg) => {
      if (msg.roomId !== roomId) return;
      setParticipants(msg.participants);
      const peer = peersRef.current[msg.userId];
      if (peer) destroyPeer(peer);
      delete peersRef.current[msg.userId];
      setLinks((prev) => dropKey(prev, msg.userId));
      setStreams((prev) => dropKey(prev, msg.userId));
    });
    const offEnded = client.on('callEnded', (msg) => {
      if (msg.roomId === roomId) leaveRef.current(false);
    });
    const offSignal = client.on('callSignal', (msg) => {
      if (msg.roomId !== roomId) return;
      try {
        openPeer(msg.fromUserId, false, parseCallSignal(msg.payload));
      } catch (err) {
        void err;
      }
    });
    const offToggle = client.on('callToggled', (msg) => {
      if (msg.roomId !== roomId) return;
      setParticipants((prev) => prev.map((person) => {
        if (person.userId !== msg.userId) return person;
        if (msg.media === MediaKind.MEDIA_KIND_MIC) return { ...person, micEnabled: msg.enabled };
        if (msg.media === MediaKind.MEDIA_KIND_CAMERA) return { ...person, cameraEnabled: msg.enabled };
        return person;
      }));
    });
    return () => {
      offState();
      offJoined();
      offLeft();
      offEnded();
      offSignal();
      offToggle();
    };
  }, [client, roomId, inCall, userId]);

  const joinCall = async () => {
    if (!client || !roomId) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
      localStreamRef.current = stream;
      if (localVideoRef.current) localVideoRef.current.srcObject = stream;
      inCallRef.current = true;
      setInCall(true);
      setMicEnabled(true);
      setCameraEnabled(true);
      await client.send({ callJoin: { roomId } });
    } catch (err) {
      stopLocal();
      inCallRef.current = false;
      setInCall(false);
      toast(err instanceof Error ? err.message : 'Could not access camera or microphone');
    }
  };

  const toggleMic = () => {
    const next = !micEnabled;
    localStreamRef.current?.getAudioTracks().forEach((track) => { track.enabled = next; });
    setMicEnabled(next);
    if (client && roomId) {
      void client.send({ callToggle: { roomId, media: MediaKind.MEDIA_KIND_MIC, enabled: next } }).catch(() => undefined);
    }
  };

  const toggleCamera = () => {
    const next = !cameraEnabled;
    localStreamRef.current?.getVideoTracks().forEach((track) => { track.enabled = next; });
    setCameraEnabled(next);
    if (client && roomId) {
      void client.send({ callToggle: { roomId, media: MediaKind.MEDIA_KIND_CAMERA, enabled: next } }).catch(() => undefined);
    }
  };

  const remotes = participants.filter((person) => person.userId !== userId);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-gray-200 bg-gray-50 p-4">
        <h3 className="text-lg font-semibold text-gray-800">Video Call</h3>
        <span className="rounded-full bg-indigo-100 px-2 py-1 text-sm text-indigo-600">{participants.length}</span>
      </div>
      {!inCall ? (
        <div className="flex flex-1 flex-col items-center justify-center p-6 text-center">
          <FiVideo className="mb-4 text-3xl text-indigo-600" />
          <button type="button" onClick={() => void joinCall()} className="flex items-center gap-2 rounded-lg bg-indigo-600 px-6 py-3 text-white">
            <FiPhone /> Join Call
          </button>
        </div>
      ) : (
        <>
          <div className="grid flex-1 grid-cols-1 gap-2 overflow-hidden bg-gray-900 p-2">
            <div className="relative min-h-[120px] overflow-hidden rounded-lg bg-gray-800">
              {cameraEnabled ? (
                <video ref={localVideoRef} autoPlay muted playsInline className="h-full w-full object-cover" />
              ) : (
                <p className="p-4 text-sm text-white">{username || 'You'}</p>
              )}
              <span className="absolute bottom-2 left-2 rounded bg-black/60 px-2 py-1 text-xs text-white">You {!micEnabled && '(muted)'}</span>
            </div>
            {remotes.map((person) => (
              <div key={person.userId} className="relative min-h-[120px] overflow-hidden rounded-lg bg-gray-800">
                <LinkChip state={links[person.userId] ?? 'connecting'} />
                {streams[person.userId] && person.cameraEnabled ? (
                  <RemoteVideo stream={streams[person.userId] ?? null} className="h-full w-full object-cover" />
                ) : (
                  <p className="p-4 text-sm text-white">{person.username || 'User'}</p>
                )}
                <span className="absolute bottom-2 left-2 rounded bg-black/60 px-2 py-1 text-xs text-white">
                  {person.username} {!person.micEnabled && '(muted)'}
                </span>
              </div>
            ))}
          </div>
          <div className="flex justify-center gap-3 border-t border-gray-700 bg-gray-800 p-4">
            <button type="button" aria-label={micEnabled ? 'Mute microphone' : 'Unmute microphone'} className={`rounded-full p-2 ${micEnabled ? 'bg-gray-700' : 'bg-red-600'}`} onClick={toggleMic}>
              {micEnabled ? <FiMic color="white" /> : <FiMicOff color="white" />}
            </button>
            <button type="button" aria-label={cameraEnabled ? 'Turn camera off' : 'Turn camera on'} className={`rounded-full p-2 ${cameraEnabled ? 'bg-gray-700' : 'bg-red-600'}`} onClick={toggleCamera}>
              {cameraEnabled ? <FiVideo color="white" /> : <FiVideoOff color="white" />}
            </button>
            <button type="button" aria-label="Leave call" className="rounded-full bg-red-600 p-2" onClick={() => leaveCall(true)}>
              <FiPhoneOff color="white" />
            </button>
          </div>
        </>
      )}
    </div>
  );
}

function dropKey<T>(record: Record<string, T>, key: string): Record<string, T> {
  const next = { ...record };
  delete next[key];
  return next;
}

function LinkChip({ state }: { state: LinkState }) {
  const label = state === 'connected' ? 'Connected' : state === 'failed' ? 'Failed' : 'Connecting';
  const tone = state === 'connected' ? 'bg-green-600' : state === 'failed' ? 'bg-red-600' : 'bg-amber-500';
  return <span className={`absolute top-2 right-2 rounded px-2 py-0.5 text-xs text-white ${tone}`}>{label}</span>;
}
