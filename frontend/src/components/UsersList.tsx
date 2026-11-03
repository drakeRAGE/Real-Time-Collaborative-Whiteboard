import { useEffect, useRef } from 'react';
import { useRoom } from '../context/RoomProvider';
import { useSocket } from '../context/SocketProvider';
import { useToast } from '../context/ToastContext';
import { useAuth } from '../context/AuthContext';

const AVATAR = ['#CBD5E1', '#E2E8F0', '#D1D5DB', '#F3F4F6', '#E5E7EB'];

function mutedColor(name: string): string {
  let sum = 0;
  for (let i = 0; i < name.length; i++) sum += name.charCodeAt(i);
  return AVATAR[sum % AVATAR.length] ?? '#E5E7EB';
}

export default function UsersList() {
  const { userId } = useAuth();
  const { members, adminId } = useRoom();
  const { client } = useSocket();
  const toast = useToast();
  const membersRef = useRef(members);
  membersRef.current = members;

  useEffect(() => {
    if (!client) return;
    const offJoined = client.on('userOnline', (msg) => {
      if (msg.userId === userId) return;
      toast(`${msg.username || 'Someone'} joined the room`);
    });
    const offLeft = client.on('userOffline', (msg) => {
      if (msg.userId === userId) return;
      const name = membersRef.current.find((member) => member.userId === msg.userId)?.username || 'Someone';
      toast(`${name} left the room`);
    });
    return () => {
      offJoined();
      offLeft();
    };
  }, [client, toast, userId]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-gray-200 bg-gray-50 p-4">
        <h3 className="text-lg font-semibold text-gray-800">Online Users</h3>
        <span className="rounded-full bg-indigo-100 px-2 py-1 text-sm font-medium text-indigo-600">{members.length}</span>
      </div>
      <div className="flex-1 overflow-y-auto">
        {members.length === 0 ? (
          <p className="p-6 text-center text-sm text-gray-500">No users online</p>
        ) : (
          <div className="p-2">
            {members.map((user) => {
              const isUserAdmin = user.userId === adminId;
              return (
                <div key={user.userId} className={`mb-2 flex items-center gap-3 rounded-lg border p-3 ${isUserAdmin ? 'border-indigo-200 bg-indigo-50' : 'border-gray-100 bg-white'}`}>
                  <div
                    className={`flex h-10 w-10 items-center justify-center rounded-full text-sm font-semibold text-gray-700 ${isUserAdmin ? 'ring-2 ring-indigo-300' : ''}`}
                    style={{ backgroundColor: mutedColor(user.username || '') }}
                  >
                    {(user.username || '?').charAt(0).toUpperCase()}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className={`truncate text-sm font-medium ${isUserAdmin ? 'text-indigo-700' : 'text-gray-900'}`}>{user.username || 'Anonymous'}</p>
                    <p className="text-xs text-gray-500">{isUserAdmin ? 'Administrator' : 'Member'}</p>
                  </div>
                  {isUserAdmin && <span className="rounded-full bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-700">Admin</span>}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
