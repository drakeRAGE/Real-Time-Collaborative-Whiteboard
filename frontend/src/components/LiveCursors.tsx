import { useEffect, useState } from 'react';
import { useSocket } from '../context/SocketProvider';

interface Cursor {
  x: number;
  y: number;
  username: string;
  color: string;
}

export default function LiveCursors({ roomId, userId }: { roomId: string; userId: string | null }) {
  const { client } = useSocket();
  const [cursors, setCursors] = useState<Record<string, Cursor>>({});

  useEffect(() => {
    if (!client) return;
    const offMove = client.on('cursorMoved', (msg) => {
      if (msg.roomId !== roomId || msg.userId === userId) return;
      setCursors((prev) => ({
        ...prev,
        [msg.userId]: { x: msg.x, y: msg.y, username: msg.username, color: msg.color || '#9866ce' },
      }));
    });
    const offLeft = client.on('userOffline', (msg) => {
      if (msg.roomId !== roomId) return;
      setCursors((prev) => {
        const next = { ...prev };
        delete next[msg.userId];
        return next;
      });
    });
    return () => {
      offMove();
      offLeft();
    };
  }, [client, roomId, userId]);

  return (
    <div className="pointer-events-none absolute inset-0 z-[100]">
      {Object.entries(cursors).map(([id, cursor]) => (
        <div key={id} className="absolute flex -translate-x-1/2 -translate-y-1/2 flex-col items-center" style={{ left: cursor.x, top: cursor.y }}>
          <div className="h-3 w-3 rounded-full border-2 border-white" style={{ backgroundColor: cursor.color }} />
          {cursor.username && (
            <div className="mt-1 rounded-full px-2 text-xs text-white" style={{ backgroundColor: cursor.color }}>
              {cursor.username}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
