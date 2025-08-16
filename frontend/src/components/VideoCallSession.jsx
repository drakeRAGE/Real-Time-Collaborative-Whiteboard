import React, { useEffect, useState, useRef } from 'react';
import { FiVideo, FiVideoOff, FiMic, FiMicOff, FiPhoneOff, FiPhone } from 'react-icons/fi';
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
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-gray-200 bg-gray-50">
        <h3 className="text-lg font-semibold text-gray-800">Video Call</h3>
        <div className="flex items-center gap-2">
          {inCall && (
            <span className="px-2 py-1 bg-green-100 text-green-600 rounded-full text-sm font-medium flex items-center gap-1">
              <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
              Live
            </span>
          )}
          <span className="px-2 py-1 bg-indigo-100 text-indigo-600 rounded-full text-sm font-medium">
            {participants.length}
          </span>
        </div>
      </div>

      {!inCall ? (
        /* Call Start Screen */
        <div className="flex-1 flex flex-col items-center justify-center p-6 text-center">
          <div className="w-20 h-20 bg-indigo-100 rounded-full flex items-center justify-center mb-4">
            <FiVideo className="text-3xl text-indigo-600" />
          </div>
          <h4 className="text-lg font-semibold text-gray-800 mb-2">Start Video Call</h4>
          <p className="text-sm text-gray-600 mb-6 leading-relaxed">
            Connect with your team members through video. Make sure your camera and microphone are working.
          </p>
          <button
            onClick={joinCall}
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-6 py-3 rounded-lg flex items-center gap-2 transition-colors font-medium"
          >
            <FiPhone className="text-lg" />
            Join Call
          </button>
        </div>
      ) : (
        /* Active Call Interface */
        <>
          {/* Video Grid */}
          <div className="flex-1 p-2 bg-gray-900 overflow-hidden">
            <div className="grid grid-cols-1 gap-2 h-full">
              {/* Local Video */}
              <div className="relative bg-gray-800 rounded-lg overflow-hidden min-h-[120px]">
                {cameraEnabled ? (
                  <video
                    ref={localVideoRef}
                    autoPlay
                    muted
                    playsInline
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="absolute inset-0 flex items-center justify-center bg-gray-800 text-white">
                    <div className="text-center">
                      <div className="w-12 h-12 mx-auto bg-gray-700 rounded-full flex items-center justify-center mb-2">
                        <span className="text-lg font-semibold">{username?.charAt(0) || 'Y'}</span>
                      </div>
                      <p className="text-xs">{username || 'You'}</p>
                    </div>
                  </div>
                )}
                <div className="absolute bottom-2 left-2 bg-black bg-opacity-60 px-2 py-1 rounded text-xs text-white">
                  You {!micEnabled && '(muted)'}
                </div>
              </div>

              {/* Remote Videos */}
              {participants
                .filter(p => p.userId !== userId)
                .slice(0, 2)
                .map(participant => (
                  <div key={participant.userId} className="relative bg-gray-800 rounded-lg overflow-hidden min-h-[120px]">
                    {streams[participant.userId] && participant.cameraEnabled ? (
                      <RemoteVideo
                        stream={streams[participant.userId]}
                        className="w-full h-full object-cover"
                      />
                    ) : (
                      <div className="absolute inset-0 flex items-center justify-center bg-gray-800 text-white">
                        <div className="text-center">
                          <div className="w-12 h-12 mx-auto bg-gray-700 rounded-full flex items-center justify-center mb-2">
                            <span className="text-lg font-semibold">
                              {participant.username?.charAt(0) || 'U'}
                            </span>
                          </div>
                          <p className="text-xs">{participant.username || 'User'}</p>
                        </div>
                      </div>
                    )}
                    <div className="absolute bottom-2 left-2 bg-black bg-opacity-60 px-2 py-1 rounded text-xs text-white">
                      {participant.username} {!participant.micEnabled && '(muted)'}
                    </div>
                  </div>
                ))}
            </div>

            {/* More participants indicator */}
            {participants.filter(p => p.userId !== userId).length > 2 && (
              <div className="mt-2 text-center">
                <span className="text-xs text-gray-400">
                  +{participants.filter(p => p.userId !== userId).length - 2} more participants
                </span>
              </div>
            )}
          </div>

          {/* Controls */}
          <div className="p-4 bg-gray-800 border-t border-gray-700">
            <div className="flex justify-center gap-3">
              <button
                onClick={toggleMic}
                className={`p-2 rounded-full transition-colors ${micEnabled ? 'bg-gray-700 hover:bg-gray-600' : 'bg-red-600 hover:bg-red-700'
                  }`}
                title={micEnabled ? 'Mute microphone' : 'Unmute microphone'}
              >
                {micEnabled ? (
                  <FiMic size={18} color="white" />
                ) : (
                  <FiMicOff size={18} color="white" />
                )}
              </button>

              <button
                onClick={toggleCamera}
                className={`p-2 rounded-full transition-colors ${cameraEnabled ? 'bg-gray-700 hover:bg-gray-600' : 'bg-red-600 hover:bg-red-700'
                  }`}
                title={cameraEnabled ? 'Turn off camera' : 'Turn on camera'}
              >
                {cameraEnabled ? (
                  <FiVideo size={18} color="white" />
                ) : (
                  <FiVideoOff size={18} color="white" />
                )}
              </button>

              <button
                onClick={leaveCall}
                className="p-2 rounded-full bg-red-600 hover:bg-red-700 transition-colors"
                title="Leave call"
              >
                <FiPhoneOff size={18} color="white" />
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}