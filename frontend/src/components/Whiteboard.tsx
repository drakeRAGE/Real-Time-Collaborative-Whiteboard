import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { deleteRoom } from '../api/gateway';
import { useAuth } from '../context/AuthContext';
import { useRoom } from '../context/RoomProvider';
import { useSocket } from '../context/SocketProvider';
import { useToast } from '../context/ToastContext';
import { useBoardOps } from '../hooks/useBoardOps';
import { useStrokeBatcher } from '../hooks/useStrokeBatcher';
import { ShapeKind, type Point, type Shape } from '../realtime/gen/whiteboard/board/v1/board';
import { ulid } from '../realtime/ulid';
import { squareEnd } from '../utils/canvasUtils';
import { usernameFromEmail } from '../utils/username';
import CanvasBoard from './CanvasBoard';
import CommunicationPanel from './Communication';
import Modal from '../UI/Modal';
import LiveCursors from './LiveCursors';
import ShapeSelector from './ShapeSelector';
import Toolbar from './Toolbar';

function syncedKind(name: string): ShapeKind | null {
  if (name === 'circle') return ShapeKind.SHAPE_KIND_CIRCLE;
  if (name === 'rectangle' || name === 'square') return ShapeKind.SHAPE_KIND_RECT;
  return null;
}

export default function Whiteboard() {
  const { roomId = '' } = useParams();
  const navigate = useNavigate();
  const { email, session } = useAuth();
  const { client } = useSocket();
  const { isAdmin } = useRoom();
  const toast = useToast();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [color, setColor] = useState('#000000');
  const prevColorRef = useRef(color);
  const [brushSize, setBrushSize] = useState(4);
  const [eraser, setEraser] = useState(false);
  const [shape, setShape] = useState<string | null>(null);
  const [modal, setModal] = useState<'clear' | 'delete' | null>(null);
  const { commit, repaint } = useBoardOps(roomId, canvasRef);
  const { addPoint, endStroke } = useStrokeBatcher({
    roomId,
    color,
    width: brushSize,
    eraser,
    onStroke: (stroke) => commit({ type: 'stroke', stroke }),
  });
  const mode = shape === 'none' ? 'cursor' : shape ? 'shape' : 'pen';

  const sendAdmin = useCallback((kind: 'undo' | 'redo' | 'clear') => {
    if (!client || !roomId) return;
    const payload = kind === 'undo'
      ? { undo: { roomId } }
      : kind === 'redo'
        ? { redo: { roomId } }
        : { clear: { roomId } };
    void client.send(payload).catch((err: unknown) => {
      toast(err instanceof Error ? err.message : 'Request failed');
    });
  }, [client, roomId, toast]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (!isAdmin || !(event.ctrlKey || event.metaKey)) return;
      if (event.key.toLowerCase() === 'z') {
        event.preventDefault();
        sendAdmin('undo');
      } else if (event.key.toLowerCase() === 'y') {
        event.preventDefault();
        sendAdmin('redo');
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isAdmin, sendAdmin]);

  const onShape = (start: Point, end: Point) => {
    if (!shape || shape === 'none' || !roomId) return;
    const endPoint = shape === 'square' ? squareEnd(start, end) : end;
    const kind = syncedKind(shape);
    if (!kind) {
      commit({
        type: 'legacy',
        id: ulid(),
        legacy: { name: shape, start, end, color, width: brushSize },
      });
      toast('That shape stays on this canvas. Rectangles and circles sync to the room.');
      return;
    }
    const id = ulid();
    const drawn: Shape = { id, roomId, userId: '', kind, start, end: endPoint, color, width: brushSize };
    commit({ type: 'shape', shape: drawn });
    if (!client) return;
    void client.send({
      shapeInput: { shapeId: id, roomId, kind, start, end: endPoint, color, width: brushSize },
    }).catch((err: unknown) => {
      toast(err instanceof Error ? err.message : 'Shape was not saved');
    });
  };

  const confirm = () => {
    if (modal === 'clear') {
      commit({ type: 'clear' });
      sendAdmin('clear');
      return;
    }
    const token = session?.access_token;
    if (!token) {
      toast('You are not signed in');
      return;
    }
    void deleteRoom(token, roomId).then(() => navigate('/')).catch((err: unknown) => {
      toast(err instanceof Error ? err.message : 'Could not delete the room');
    });
  };

  return (
    <div className="flex h-[92vh] flex-col text-white">
      <ShapeSelector selectedShape={shape} setSelectedShape={setShape} isEraserActive={eraser} />
      <Toolbar
        color={color}
        setColor={(next) => {
          setColor(next);
          if (eraser && next !== '#ffffff') setEraser(false);
        }}
        eraser={eraser}
        setEraser={(next) => {
          if (next) {
            prevColorRef.current = color === '#ffffff' ? prevColorRef.current : color;
            setColor('#ffffff');
            setShape(null);
          } else {
            setColor(prevColorRef.current === '#ffffff' ? '#000000' : prevColorRef.current);
          }
          setEraser(next);
        }}
        brushSize={brushSize}
        setBrushSize={setBrushSize}
        isAdmin={isAdmin}
        onUndo={() => sendAdmin('undo')}
        onRedo={() => sendAdmin('redo')}
        onClear={() => setModal('clear')}
        onDelete={() => setModal('delete')}
      />
      <CommunicationPanel roomId={roomId} userId={session?.user.id ?? null} username={usernameFromEmail(email)} />
      <div className="relative flex-grow">
        <CanvasBoard
          canvasRef={canvasRef}
          mode={mode}
          eraser={eraser}
          onPoint={addPoint}
          onStrokeEnd={endStroke}
          onShape={onShape}
          onResize={repaint}
        />
        <LiveCursors roomId={roomId} userId={session?.user.id ?? null} />
      </div>
      {modal && (
        <Modal
          title={modal === 'clear' ? 'Clear' : 'Delete'}
          confirmLabel={modal === 'clear' ? 'Clear' : 'Delete'}
          message={modal === 'clear'
            ? 'Are you sure you want to clear the canvas? This action cannot be undone.'
            : 'Are you sure you want to delete this room? This action cannot be undone.'}
          onConfirm={confirm}
          onClose={() => setModal(null)}
        />
      )}
    </div>
  );
}
