import { useCallback, useEffect, useRef } from 'react';
import { type Point, type Stroke, Tool } from '../realtime/gen/whiteboard/board/v1/board';
import { ulid } from '../realtime/ulid';
import { useSocket } from '../context/SocketProvider';

interface StrokeBatcherOptions {
  roomId: string;
  color: string;
  width: number;
  eraser: boolean;
  onStroke: (stroke: Stroke) => void;
}

export function useStrokeBatcher(options: StrokeBatcherOptions): {
  addPoint: (point: Point) => void;
  endStroke: () => void;
} {
  const { client } = useSocket();
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const clientRef = useRef(client);
  clientRef.current = client;
  const pointsRef = useRef<Point[]>([]);
  const frameRef = useRef(0);
  const lastCursorRef = useRef(0);

  const flush = useCallback(() => {
    frameRef.current = 0;
    const points = pointsRef.current.splice(0);
    if (points.length === 0) return;
    const { roomId, color, width, eraser, onStroke } = optionsRef.current;
    const stroke: Stroke = {
      id: ulid(),
      roomId,
      userId: '',
      points,
      color: eraser ? '#ffffff' : color,
      width,
      tool: eraser ? Tool.TOOL_ERASER : Tool.TOOL_PEN,
    };
    onStroke(stroke);
    const rt = clientRef.current;
    if (!rt || !roomId) return;
    void rt.send({
      strokeInput: {
        strokeId: stroke.id,
        roomId,
        points,
        color: stroke.color,
        width,
        tool: stroke.tool,
      },
    }).catch(() => undefined);
  }, []);

  const addPoint = useCallback((point: Point) => {
    pointsRef.current.push(point);
    if (frameRef.current) return;
    frameRef.current = requestAnimationFrame(flush);
  }, [flush]);

  const endStroke = useCallback(() => {
    if (frameRef.current) cancelAnimationFrame(frameRef.current);
    flush();
  }, [flush]);

  useEffect(() => () => {
    if (frameRef.current) cancelAnimationFrame(frameRef.current);
  }, []);

  useEffect(() => {
    const move = (event: MouseEvent) => {
      const canvas = document.querySelector('canvas');
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;
      if (x < 0 || y < 0 || x > rect.width || y > rect.height) return;
      const now = performance.now();
      if (now - lastCursorRef.current < 1000 / 30) return;
      lastCursorRef.current = now;
      const rt = clientRef.current;
      const roomId = optionsRef.current.roomId;
      if (!rt || rt.state !== 'open' || !roomId) return;
      void rt.send({ cursorMove: { roomId, x, y } }).catch(() => undefined);
    };
    window.addEventListener('mousemove', move);
    return () => window.removeEventListener('mousemove', move);
  }, []);

  return { addPoint, endStroke };
}
