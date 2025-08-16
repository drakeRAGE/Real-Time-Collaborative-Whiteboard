import React, { useEffect, useState, useRef } from 'react';
import { FiVideo, FiVideoOff, FiMic, FiMicOff, FiPhoneOff } from 'react-icons/fi';
import Peer from 'simple-peer';
import RemoteVideo from './RemoteVideo';

export default function VideoCallSession({ socket, roomId, userId, username }) {
  const [inCall, setInCall] = useState(false);
  const [callOpen, setCallOpen] = useState(false);
  const [micEnabled, setMicEnabled] = useState(true);
  const [cameraEnabled, setCameraEnabled] = useState(true);
  const [participants, setParticipants] = useState([]);
  const [streams, setStreams] = useState({});
  const [peers, setPeers] = useState({});
  
  const localVideoRef = useRef(null);
  const localStreamRef = useRef(null);
  const peersRef = useRef({});

  // Initialize media stream
  const initializeMedia = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: cameraEnabled,
        audio: micEnabled
      });
      
      localStreamRef.current = stream;
      if (localVideoRef.current) {
        localVideoRef.current.srcObject = stream;
      }
      
      return stream;
    } catch (error) {
      console.error('Error accessing media devices:', error);
      alert('Could not access camera or microphone');
      return null;
    }
  };

  // Join call
  const joinCall = async () => {
    const stream = await initializeMedia();
    if (!stream) return;
    
    setInCall(true);
    setCallOpen(true);
    
    socket.emit('call:join', { roomId });
  };

  // Leave call
  const leaveCall = () => {
    socket.emit('call:leave', { roomId });
    setInCall(false);
    
    // Close all peer connections
    Object.values(peersRef.current).forEach(peer => {
      if (peer) peer.destroy();
    });
    
    // Stop local stream
    if (localStreamRef.current) {
      localStreamRef.current.getTracks().forEach(track => track.stop());
      localStreamRef.current = null;
    }
    
    setPeers({});
    peersRef.current = {};
    setStreams({});
  };

  // Toggle microphone
  const toggleMic = () => {
    if (localStreamRef.current) {
      const audioTracks = localStreamRef.current.getAudioTracks();
      audioTracks.forEach(track => {
        track.enabled = !micEnabled;
      });
      
      setMicEnabled(!micEnabled);
      socket.emit('call:toggleMic', { roomId, enabled: !micEnabled });
    }
  };

  // Toggle camera
  const toggleCamera = () => {
    if (localStreamRef.current) {
      const videoTracks = localStreamRef.current.getVideoTracks();
      videoTracks.forEach(track => {
        track.enabled = !cameraEnabled;
      });
      
      setCameraEnabled(!cameraEnabled);
      socket.emit('call:toggleCamera', { roomId, enabled: !cameraEnabled });
    }
  };

  const ICE_CONFIG = {
    iceServers: [
      { urls: 'stun:stun.l.google.com:19302' },
      // Optional TURN server if you have one:
      // { urls: 'turn:your-turn-server.com:3478', username: 'user', credential: 'pass' }
    ]
  };
  // Create a peer connection to a new user
  const createPeer = (participantId, stream) => {
    const peer = new Peer({
      initiator: true,
      trickle: false,
      stream,
      config: ICE_CONFIG
    });

    peer.on('signal', signal => {
      socket.emit('call:signal', { roomId, to: participantId, signal });
    });

    peer.on('stream', remoteStream => {
      setStreams(prev => ({ ...prev, [participantId]: remoteStream }));
    });

    peer.on('error', e => console.error('peer error', e));
    peer.on('close', () => {
      // cleanup
      setPeers(prev => { const p = { ...prev }; delete p[participantId]; return p; });
      setStreams(prev => { const s = { ...prev }; delete s[participantId]; return s; });
      delete peersRef.current[participantId];
    });

    return peer;
  };

  // Add a peer connection from an existing user
  const addPeer = (participantId, signal, stream) => {
    const peer = new Peer({
      initiator: false,
      trickle: false,
      stream,
      config: ICE_CONFIG
    });

    peer.on('signal', sig => {
      socket.emit('call:signal', { roomId, to: participantId, signal: sig });
    });

    peer.on('stream', remoteStream => {
      setStreams(prev => ({ ...prev, [participantId]: remoteStream }));
    });

    peer.on('error', e => console.error('peer error', e));
    peer.on('close', () => {
      setPeers(prev => { const p = { ...prev }; delete p[participantId]; return p; });
      setStreams(prev => { const s = { ...prev }; delete s[participantId]; return s; });
      delete peersRef.current[participantId];
    });

    peer.signal(signal);

    return peer;
  };

  // Handle socket events
  useEffect(() => {
    if (!socket || !roomId || !inCall) return;

    // When a new user joins the call
    socket.on('call:userJoined', ({ userId: joinedUserId, username, participants: callParticipants }) => {
      setParticipants(callParticipants);
      
      // If this is a new user and we have a stream, create a peer connection
      if (joinedUserId !== userId && localStreamRef.current && !peersRef.current[joinedUserId]) {
        const peer = createPeer(joinedUserId, localStreamRef.current);
        peersRef.current[joinedUserId] = peer;
        
        setPeers(prev => ({
          ...prev,
          [joinedUserId]: peer
        }));
      }
    });

    // When a user leaves the call
    socket.on('call:userLeft', ({ userId: leftUserId, participants: callParticipants }) => {
      setParticipants(callParticipants);
      
      // Close and remove the peer connection
      if (peersRef.current[leftUserId]) {
        peersRef.current[leftUserId].destroy();
        delete peersRef.current[leftUserId];
        
        setPeers(prev => {
          const newPeers = { ...prev };
          delete newPeers[leftUserId];
          return newPeers;
        });
        
        setStreams(prev => {
          const newStreams = { ...prev };
          delete newStreams[leftUserId];
          return newStreams;
        });
      }
    });

    // When the call ends
    socket.on('call:ended', () => {
      leaveCall();
    });

    // When receiving a signal from another user
    socket.on('call:signal', ({ from, signal }) => {
      // If we don't have a peer connection to this user yet, create one
      if (!peersRef.current[from] && localStreamRef.current) {
        const peer = addPeer(from, signal, localStreamRef.current);
        peersRef.current[from] = peer;
        
        setPeers(prev => ({
          ...prev,
          [from]: peer
        }));
      } else if (peersRef.current[from]) {
        // Otherwise, signal the existing peer
        peersRef.current[from].signal(signal);
      }
    });

    // When a user toggles their mic
    socket.on('call:micToggled', ({ userId: toggledUserId, enabled }) => {
      setParticipants(prev => 
        prev.map(p => 
          p.userId === toggledUserId ? { ...p, micEnabled: enabled } : p
        )
      );
    });

    // When a user toggles their camera
    socket.on('call:cameraToggled', ({ userId: toggledUserId, enabled }) => {
      setParticipants(prev => 
        prev.map(p => 
          p.userId === toggledUserId ? { ...p, cameraEnabled: enabled } : p
        )
      );
    });

    // When receiving the current call state
    socket.on('call:state', (callSession) => {
      setParticipants(callSession.participants);
    });

    return () => {
      socket.off('call:userJoined');
      socket.off('call:userLeft');
      socket.off('call:ended');
      socket.off('call:signal');
      socket.off('call:micToggled');
      socket.off('call:cameraToggled');
      socket.off('call:state');
    };
  }, [socket, roomId, userId, inCall]);

  // Clean up when component unmounts
  useEffect(() => {
    return () => {
      if (inCall) {
        leaveCall();
      }
    };
  }, []);

  // Toggle call panel
  const toggleCallPanel = () => {
    if (!inCall) {
      joinCall();
    } else {
      setCallOpen(!callOpen);
    }
  };

  return (
    <>
      {/* Call button */}
      <button
        onClick={toggleCallPanel}
        aria-label={inCall ? 'Toggle call panel' : 'Join call'}
        className={`fixed bottom-5 left-5 z-50 flex items-center justify-center w-14 h-14 rounded-full shadow-lg ${inCall ? 'bg-green-600 hover:bg-green-700' : 'bg-blue-600 hover:bg-blue-700'} text-white transition-colors`}
      >
        {inCall ? (
          <FiVideo size={28} />
        ) : (
          <FiVideo size={28} />
        )}
      </button>

      {/* Call panel */}
      <aside
        className={`fixed bottom-5 left-6 z-40 w-[90vw] max-w-[800px] h-[480px] bg-gray-900 rounded-lg shadow-2xl flex flex-col overflow-hidden transform transition-transform duration-300 ${callOpen && inCall ? 'translate-y-0 opacity-100' : 'translate-y-12 opacity-0 pointer-events-none'}`}
        role="region"
        aria-label="Video call"
      >
        {/* Header */}
        <header className="flex items-center justify-between bg-gray-800 px-4 py-3">
          <h2 className="text-white font-semibold text-lg select-none">Video Call ({participants.length} participants)</h2>
          <button
            onClick={() => setCallOpen(false)}
            aria-label="Minimize call"
            className="text-white hover:text-gray-200"
          >
            <FiVideoOff size={20} />
          </button>
        </header>

        {/* Video grid */}
        <div className="flex-1 bg-gray-900 p-2 overflow-hidden">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 h-full">
            {/* Local video */}
            <div className="relative bg-gray-800 rounded overflow-hidden">
              <video
                ref={localVideoRef}
                autoPlay
                muted
                playsInline
                className={`w-full h-full object-cover ${!cameraEnabled ? 'hidden' : ''}`}
              />
              {!cameraEnabled && (
                <div className="absolute inset-0 flex items-center justify-center bg-gray-800 text-white">
                  <div className="text-center">
                    <div className="w-20 h-20 mx-auto bg-gray-700 rounded-full flex items-center justify-center mb-2">
                      <span className="text-2xl font-semibold">{username?.charAt(0) || 'U'}</span>
                    </div>
                    <p className="text-sm">{username || 'You'}</p>
                  </div>
                </div>
              )}
              <div className="absolute bottom-2 left-2 text-white bg-black bg-opacity-50 px-2 py-1 rounded text-xs">
                You {!micEnabled && '(muted)'}
              </div>
            </div>

            {/* Remote videos */}
            {participants
              .filter(p => p.userId !== userId)
              .map(participant => (
                <div key={participant.userId} className="relative bg-gray-800 rounded overflow-hidden">
                  {streams[participant.userId] && participant.cameraEnabled ? (
                    <RemoteVideo stream={streams[participant.userId]} className="w-full h-full object-cover" />
                  ) : (
                    <div className="absolute inset-0 flex items-center justify-center bg-gray-800 text-white">
                      <div className="text-center">
                        <div className="w-20 h-20 mx-auto bg-gray-700 rounded-full flex items-center justify-center mb-2">
                          <span className="text-2xl font-semibold">{participant.username?.charAt(0) || 'U'}</span>
                        </div>
                        <p className="text-sm">{participant.username || 'User'}</p>
                      </div>
                    </div>
                  )}
                  <div className="absolute bottom-2 left-2 text-white bg-black bg-opacity-50 px-2 py-1 rounded text-xs">
                    {participant.username} {!participant.micEnabled && '(muted)'}
                  </div>
                </div>
              ))}
          </div>
        </div>

        {/* Controls */}
        <div className="flex justify-center items-center gap-4 p-4 bg-gray-800">
          <button
            onClick={toggleMic}
            className={`p-3 rounded-full ${micEnabled ? 'bg-gray-700 hover:bg-gray-600' : 'bg-red-600 hover:bg-red-700'}`}
            aria-label={micEnabled ? 'Mute microphone' : 'Unmute microphone'}
          >
            {micEnabled ? <FiMic size={24} color="white" /> : <FiMicOff size={24} color="white" />}
          </button>
          
          <button
            onClick={toggleCamera}
            className={`p-3 rounded-full ${cameraEnabled ? 'bg-gray-700 hover:bg-gray-600' : 'bg-red-600 hover:bg-red-700'}`}
            aria-label={cameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          >
            {cameraEnabled ? <FiVideo size={24} color="white" /> : <FiVideoOff size={24} color="white" />}
          </button>
          
          <button
            onClick={leaveCall}
            className="p-3 rounded-full bg-red-600 hover:bg-red-700"
            aria-label="Leave call"
          >
            <FiPhoneOff size={24} color="white" />
          </button>
        </div>
      </aside>
    </>
  );
}