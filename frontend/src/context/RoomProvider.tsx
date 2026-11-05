import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import type { PresentMember } from '../realtime/gen/whiteboard/realtime/v1/envelope';
import { Role } from '../realtime/gen/whiteboard/room/v1/room';
import { useAuth } from './AuthContext';
import { useSocket } from './SocketProvider';

interface RoomValue {
  members: PresentMember[];
  adminId: string;
  role: Role;
  isAdmin: boolean;
}

const RoomContext = createContext<RoomValue | null>(null);

// Strict Mode runs effect cleanup and immediately remounts. Leaving in that
// cleanup deletes a one-member room before the remount can join again.
const pendingLeave = new Map<string, ReturnType<typeof setTimeout>>();

function cancelLeave(roomId: string): void {
  const timer = pendingLeave.get(roomId);
  if (timer === undefined) return;
  clearTimeout(timer);
  pendingLeave.delete(roomId);
}

export function RoomProvider({ roomId, children }: { roomId: string; children: ReactNode }) {
  const { userId } = useAuth();
  const { client } = useSocket();
  const [members, setMembers] = useState<PresentMember[]>([]);
  const [adminId, setAdminId] = useState('');

  useEffect(() => {
    if (!client || !roomId) return;
    cancelLeave(roomId);
    let active = true;
    const join = () => {
      if (!active || client.state !== 'open') return;
      void client.send({ joinRoom: { roomId, inviteToken: '' } }).catch(() => undefined);
    };
    join();
    const offState = client.onState((next) => {
      if (next === 'open') join();
    });
    const offRoom = client.on('roomState', (state) => {
      if (state.roomId !== roomId) return;
      setMembers(state.members);
      setAdminId(state.adminId);
    });
    const rt = client;
    const id = roomId;
    return () => {
      active = false;
      offState();
      offRoom();
      cancelLeave(id);
      const timer = setTimeout(() => {
        pendingLeave.delete(id);
        if (rt.state === 'open') {
          void rt.send({ leaveRoom: { roomId: id } }).catch(() => undefined);
        }
      }, 0);
      pendingLeave.set(id, timer);
    };
  }, [client, roomId]);

  const me = members.find((member) => member.userId === userId);
  const value = useMemo<RoomValue>(() => ({
    members,
    adminId,
    role: me?.role ?? Role.ROLE_UNSPECIFIED,
    isAdmin: Boolean(userId) && adminId !== '' && userId === adminId,
  }), [members, adminId, me?.role, userId]);

  return <RoomContext.Provider value={value}>{children}</RoomContext.Provider>;
}

export function useRoom(): RoomValue {
  const value = useContext(RoomContext);
  if (!value) throw new Error('useRoom requires RoomProvider');
  return value;
}
