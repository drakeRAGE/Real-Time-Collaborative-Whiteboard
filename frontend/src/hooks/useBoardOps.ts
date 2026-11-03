import { useCallback, useEffect, useRef, useState, type RefObject } from 'react';
import { loadBoardHistory } from '../api/gateway';
import { useSocket } from '../context/SocketProvider';
import { applyBoardEvent, replayBoard, type BoardEvent, type DrawOp } from '../utils/canvasUtils';
import { supabase } from '../utils/supabase';

export function useBoardOps(roomId: string, canvasRef: RefObject<HTMLCanvasElement | null>): {
  commit: (event: BoardEvent) => void;
  repaint: () => void;
} {
  const { client } = useSocket();
  const [ops, setOps] = useState<DrawOp[]>([]);
  const opsRef = useRef<DrawOp[]>([]);

  const commit = useCallback((event: BoardEvent) => {
    const next = applyBoardEvent(opsRef.current, event);
    opsRef.current = next;
    setOps(next);
  }, []);

  const repaint = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    replayBoard(canvas, opsRef.current);
  }, [canvasRef]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    replayBoard(canvas, ops);
  }, [canvasRef, ops]);

  useEffect(() => {
    if (!client || !roomId) return;
    let cancel = false;
    const offs = [
      client.on('strokeApplied', (msg) => {
        if (msg.stroke && msg.stroke.roomId === roomId) commit({ type: 'stroke', stroke: msg.stroke });
      }),
      client.on('shapeApplied', (msg) => {
        if (msg.shape && msg.shape.roomId === roomId) commit({ type: 'shape', shape: msg.shape });
      }),
      client.on('boardCleared', (msg) => {
        if (msg.roomId === roomId) commit({ type: 'clear' });
      }),
      client.on('undoApplied', (msg) => {
        if (msg.roomId === roomId) commit({ type: 'undo', opId: msg.opId });
      }),
      client.on('redoApplied', (msg) => {
        if (msg.roomId === roomId) commit({ type: 'redo', opId: msg.opId });
      }),
    ];
    void (async () => {
      let token = '';
      if (supabase) {
        const { data } = await supabase.auth.getSession();
        token = data.session?.access_token ?? '';
      }
      if (!token || cancel) return;
      const initial = await loadBoardHistory(roomId, token);
      if (cancel || initial.length === 0) return;
      let current = initial;
      for (const op of opsRef.current) {
        if (!current.some((item) => item.id === op.id)) current = [...current, op];
      }
      opsRef.current = current;
      setOps(current);
    })();
    return () => {
      cancel = true;
      for (const off of offs) off();
    };
  }, [client, roomId, commit]);

  return { commit, repaint };
}
