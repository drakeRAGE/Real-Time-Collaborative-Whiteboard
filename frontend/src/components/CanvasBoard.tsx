import { useLayoutEffect, useRef, type MouseEvent as ReactMouseEvent, type RefObject } from 'react';
import type { Point } from '../realtime/gen/whiteboard/board/v1/board';

interface CanvasBoardProps {
  canvasRef: RefObject<HTMLCanvasElement | null>;
  mode: 'pen' | 'cursor' | 'shape';
  eraser: boolean;
  onPoint: (point: Point) => void;
  onStrokeEnd: () => void;
  onShape: (start: Point, end: Point) => void;
  onResize: () => void;
}

export default function CanvasBoard({
  canvasRef,
  mode,
  eraser,
  onPoint,
  onStrokeEnd,
  onShape,
  onResize,
}: CanvasBoardProps) {
  const drawingRef = useRef(false);
  const startRef = useRef<Point | null>(null);
  const onResizeRef = useRef(onResize);
  onResizeRef.current = onResize;

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    const parent = canvas?.parentElement;
    if (!canvas || !parent) return;
    const fit = () => {
      const width = parent.clientWidth;
      const height = parent.clientHeight;
      if (width === 0 || height === 0) return;
      if (canvas.width === width && canvas.height === height) return;
      canvas.width = width;
      canvas.height = height;
      onResizeRef.current();
    };
    fit();
    window.addEventListener('resize', fit);
    return () => window.removeEventListener('resize', fit);
  }, [canvasRef]);

  const pointOf = (event: ReactMouseEvent<HTMLCanvasElement>): Point => ({
    x: event.nativeEvent.offsetX,
    y: event.nativeEvent.offsetY,
  });

  const cursor = eraser
    ? "cursor-[url('/eraser.png'),_auto]"
    : mode === 'cursor'
      ? 'cursor-default'
      : 'cursor-crosshair';

  return (
    <canvas
      ref={canvasRef}
      className={`block h-full w-full ${cursor}`}
      onMouseDown={(event) => {
        if (mode === 'cursor') return;
        const point = pointOf(event);
        if (mode === 'shape') {
          startRef.current = point;
          return;
        }
        drawingRef.current = true;
        onPoint(point);
      }}
      onMouseMove={(event) => {
        if (mode !== 'pen' || !drawingRef.current) return;
        onPoint(pointOf(event));
      }}
      onMouseUp={(event) => {
        const point = pointOf(event);
        if (mode === 'shape' && startRef.current) {
          onShape(startRef.current, point);
          startRef.current = null;
        }
        if (drawingRef.current) {
          drawingRef.current = false;
          onStrokeEnd();
        }
      }}
      onMouseLeave={() => {
        if (!drawingRef.current) return;
        drawingRef.current = false;
        onStrokeEnd();
      }}
    />
  );
}
