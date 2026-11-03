import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { createRoom } from '../api/gateway';
import { useAuth } from '../context/AuthContext';
import { useToast } from '../context/ToastContext';
import { isOnline } from '../utils/networkUtils';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export default function RoomSelection() {
  const [roomId, setRoomId] = useState('');
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const { session } = useAuth();
  const toast = useToast();

  const online = () => {
    if (isOnline()) return true;
    toast('Network error: Please check your internet connection');
    return false;
  };

  const handleJoinRoom = (event: FormEvent) => {
    event.preventDefault();
    if (!online()) return;
    const id = roomId.trim();
    if (!UUID.test(id)) {
      toast('Enter a room id from Create New Room');
      return;
    }
    navigate(`/canva/${id}`);
  };

  const handleCreateRoom = () => {
    if (!online()) return;
    const token = session?.access_token;
    if (!token) {
      toast('You are not signed in');
      return;
    }
    setBusy(true);
    void createRoom(token, 'Untitled')
      .then((id) => navigate(`/canva/${id}`))
      .catch((err: unknown) => toast(err instanceof Error ? err.message : 'Could not create the room'))
      .finally(() => setBusy(false));
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-gray-900 via-gray-800 to-black p-4">
      <div className="w-full max-w-md rounded-2xl border border-white/20 bg-white/10 p-8 shadow-xl backdrop-blur-lg">
        <h1 className="mb-6 text-center text-3xl font-bold text-white">Collaborative Whiteboard</h1>
        <p className="mb-8 text-center text-gray-300">Join an existing room or create a new one to start collaborating.</p>
        <form onSubmit={handleJoinRoom} className="space-y-5">
          <div>
            <label htmlFor="room-id" className="mb-1 block text-sm text-gray-200">Room ID</label>
            <input id="room-id" value={roomId} onChange={(event) => setRoomId(event.target.value)} placeholder="Enter Room ID" className="w-full rounded-lg border border-gray-700 bg-gray-900/60 px-4 py-2 text-white outline-none" />
          </div>
          <button type="submit" className="w-full rounded-lg bg-indigo-600 py-2 font-semibold text-white">Join Room</button>
        </form>
        <div className="my-6 flex items-center">
          <div className="h-px flex-1 bg-gray-700" />
          <span className="px-4 text-sm text-gray-400">OR</span>
          <div className="h-px flex-1 bg-gray-700" />
        </div>
        <button type="button" disabled={busy} onClick={handleCreateRoom} className="w-full rounded-lg bg-pink-500 py-2 font-semibold text-white disabled:opacity-50">
          {busy ? 'Creating…' : 'Create New Room'}
        </button>
      </div>
    </div>
  );
}
