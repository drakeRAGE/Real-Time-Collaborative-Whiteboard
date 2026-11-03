import {
  type BoardOp,
  type Point,
  ShapeKind,
  type Shape,
  type Stroke,
  Tool,
} from '../realtime/gen/whiteboard/board/v1/board';

export interface LegacyShape {
  name: string;
  start: Point;
  end: Point;
  color: string;
  width: number;
}

export interface DrawOp {
  id: string;
  active: boolean;
  stroke?: Stroke;
  shape?: Shape;
  legacy?: LegacyShape;
}

export type BoardEvent =
  | { type: 'stroke'; stroke: Stroke }
  | { type: 'shape'; shape: Shape }
  | { type: 'legacy'; id: string; legacy: LegacyShape }
  | { type: 'clear' }
  | { type: 'undo'; opId: string }
  | { type: 'redo'; opId: string };

export function applyBoardEvent(ops: DrawOp[], event: BoardEvent): DrawOp[] {
  switch (event.type) {
    case 'stroke':
      if (!event.stroke.id || ops.some((op) => op.id === event.stroke.id)) return ops;
      return [...ops, { id: event.stroke.id, active: true, stroke: event.stroke }];
    case 'shape':
      if (!event.shape.id || ops.some((op) => op.id === event.shape.id)) return ops;
      return [...ops, { id: event.shape.id, active: true, shape: event.shape }];
    case 'legacy':
      if (ops.some((op) => op.id === event.id)) return ops;
      return [...ops, { id: event.id, active: true, legacy: event.legacy }];
    case 'clear':
      return [];
    case 'undo':
      return ops.map((op) => (op.id === event.opId ? { ...op, active: false } : op));
    case 'redo':
      return ops.map((op) => (op.id === event.opId ? { ...op, active: true } : op));
  }
}

export function drawOpsFromHistory(history: readonly BoardOp[]): DrawOp[] {
  let ops: DrawOp[] = [];
  for (const op of history) {
    if (op.strokeAdded?.stroke) ops = applyBoardEvent(ops, { type: 'stroke', stroke: op.strokeAdded.stroke });
    else if (op.shapeAdded?.shape) ops = applyBoardEvent(ops, { type: 'shape', shape: op.shapeAdded.shape });
    else if (op.boardCleared) ops = applyBoardEvent(ops, { type: 'clear' });
    else if (op.undoApplied) ops = applyBoardEvent(ops, { type: 'undo', opId: op.undoApplied.opId });
    else if (op.redoApplied) ops = applyBoardEvent(ops, { type: 'redo', opId: op.redoApplied.opId });
  }
  return ops;
}

/** One painter for history load and live events. */
export function replayBoard(canvas: HTMLCanvasElement, ops: readonly DrawOp[]): void {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  for (const op of ops) {
    if (!op.active) continue;
    if (op.stroke) paintStroke(ctx, op.stroke);
    else if (op.shape) paintShape(ctx, op.shape);
    else if (op.legacy) paintLegacy(ctx, op.legacy);
  }
}

function paintStroke(ctx: CanvasRenderingContext2D, stroke: Stroke): void {
  const first = stroke.points[0];
  if (!first) return;
  ctx.beginPath();
  ctx.moveTo(first.x, first.y);
  if (stroke.points.length === 1) {
    ctx.lineTo(first.x + 0.01, first.y + 0.01);
  } else {
    for (let i = 1; i < stroke.points.length; i++) {
      const point = stroke.points[i];
      if (point) ctx.lineTo(point.x, point.y);
    }
  }
  ctx.strokeStyle = stroke.tool === Tool.TOOL_ERASER ? '#ffffff' : stroke.color;
  ctx.lineWidth = stroke.width || 1;
  ctx.stroke();
}

function paintShape(ctx: CanvasRenderingContext2D, shape: Shape): void {
  if (!shape.start || !shape.end) return;
  ctx.beginPath();
  if (shape.kind === ShapeKind.SHAPE_KIND_CIRCLE) drawCircle(ctx, shape.start, shape.end);
  else if (shape.kind === ShapeKind.SHAPE_KIND_LINE) {
    ctx.moveTo(shape.start.x, shape.start.y);
    ctx.lineTo(shape.end.x, shape.end.y);
  } else {
    drawRectangle(ctx, shape.start, shape.end);
  }
  ctx.strokeStyle = shape.color;
  ctx.lineWidth = shape.width || 1;
  ctx.stroke();
}

function paintLegacy(ctx: CanvasRenderingContext2D, shape: LegacyShape): void {
  const draw = LEGACY[shape.name];
  if (!draw) return;
  ctx.beginPath();
  draw(ctx, shape.start, shape.end);
  ctx.strokeStyle = shape.color;
  ctx.lineWidth = shape.width;
  ctx.stroke();
}

export function squareEnd(start: Point, end: Point): Point {
  const side = Math.max(Math.abs(end.x - start.x), Math.abs(end.y - start.y));
  const signX = end.x >= start.x ? 1 : -1;
  const signY = end.y >= start.y ? 1 : -1;
  return { x: start.x + signX * side, y: start.y + signY * side };
}

type Drawer = (ctx: CanvasRenderingContext2D, start: Point, end: Point) => void;

export function drawRectangle(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  ctx.rect(start.x, start.y, end.x - start.x, end.y - start.y);
}

export function drawCircle(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  ctx.ellipse(centerX, centerY, Math.abs(end.x - start.x) / 2, Math.abs(end.y - start.y) / 2, 0, 0, 2 * Math.PI);
}

function drawSquare(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const endPoint = squareEnd(start, end);
  ctx.rect(start.x, start.y, endPoint.x - start.x, endPoint.y - start.y);
}

function drawTriangle(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  ctx.moveTo(start.x, end.y);
  ctx.lineTo(end.x, end.y);
  ctx.lineTo((start.x + end.x) / 2, start.y);
  ctx.closePath();
}

function drawStar(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const outerRadius = Math.min(Math.abs(end.x - start.x), Math.abs(end.y - start.y)) / 2;
  const innerRadius = outerRadius / 2;
  ctx.moveTo(centerX + outerRadius, centerY);
  for (let i = 0; i < 10; i++) {
    const angle = (i * Math.PI) / 5;
    const radius = i % 2 === 0 ? outerRadius : innerRadius;
    ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
  }
  ctx.closePath();
}

function drawPolygon(ctx: CanvasRenderingContext2D, start: Point, end: Point, sides: number): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const radius = Math.min(Math.abs(end.x - start.x), Math.abs(end.y - start.y)) / 2;
  ctx.moveTo(centerX + radius, centerY);
  for (let i = 1; i <= sides; i++) {
    const angle = (i * 2 * Math.PI) / sides;
    ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
  }
  ctx.closePath();
}

function drawArrowUp(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const width = Math.abs(end.x - start.x);
  ctx.moveTo(centerX, start.y);
  ctx.lineTo(start.x, centerY);
  ctx.lineTo(centerX - width * 0.1, centerY);
  ctx.lineTo(centerX - width * 0.1, end.y);
  ctx.lineTo(centerX + width * 0.1, end.y);
  ctx.lineTo(centerX + width * 0.1, centerY);
  ctx.lineTo(end.x, centerY);
  ctx.closePath();
}

function drawArrowDown(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const width = Math.abs(end.x - start.x);
  ctx.moveTo(centerX, end.y);
  ctx.lineTo(start.x, centerY);
  ctx.lineTo(centerX - width * 0.1, centerY);
  ctx.lineTo(centerX - width * 0.1, start.y);
  ctx.lineTo(centerX + width * 0.1, start.y);
  ctx.lineTo(centerX + width * 0.1, centerY);
  ctx.lineTo(end.x, centerY);
  ctx.closePath();
}

function drawArrowLeft(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const height = Math.abs(end.y - start.y);
  ctx.moveTo(start.x, centerY);
  ctx.lineTo(centerX, start.y);
  ctx.lineTo(centerX, centerY - height * 0.1);
  ctx.lineTo(end.x, centerY - height * 0.1);
  ctx.lineTo(end.x, centerY + height * 0.1);
  ctx.lineTo(centerX, centerY + height * 0.1);
  ctx.lineTo(centerX, end.y);
  ctx.closePath();
}

function drawArrowRight(ctx: CanvasRenderingContext2D, start: Point, end: Point): void {
  const centerX = (start.x + end.x) / 2;
  const centerY = (start.y + end.y) / 2;
  const height = Math.abs(end.y - start.y);
  ctx.moveTo(end.x, centerY);
  ctx.lineTo(centerX, start.y);
  ctx.lineTo(centerX, centerY - height * 0.1);
  ctx.lineTo(start.x, centerY - height * 0.1);
  ctx.lineTo(start.x, centerY + height * 0.1);
  ctx.lineTo(centerX, centerY + height * 0.1);
  ctx.lineTo(centerX, end.y);
  ctx.closePath();
}

const LEGACY: Record<string, Drawer> = {
  rectangle: drawRectangle,
  square: drawSquare,
  circle: drawCircle,
  triangle: drawTriangle,
  star: drawStar,
  pentagon: (ctx, start, end) => drawPolygon(ctx, start, end, 5),
  hexagon: (ctx, start, end) => drawPolygon(ctx, start, end, 6),
  arrowUp: drawArrowUp,
  arrowDown: drawArrowDown,
  arrowLeft: drawArrowLeft,
  arrowRight: drawArrowRight,
};
