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

export function RoomProvider({ roomId, children }: { roomId: string; children: ReactNode }) {
  const { userId } = useAuth();
  const { client } = useSocket();
  const [members, setMembers] = useState<PresentMember[]>([]);
  const [adminId, setAdminId] = useState('');

  useEffect(() => {
    if (!client || !roomId) return;
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
    return () => {
      active = false;
      offState();
      offRoom();
      if (client.state === 'open') {
        void client.send({ leaveRoom: { roomId } }).catch(() => undefined);
      }
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
