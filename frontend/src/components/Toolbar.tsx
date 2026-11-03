import { BsEraserFill } from 'react-icons/bs';
import { FaRedo, FaUndo } from 'react-icons/fa';
import { BiReset } from 'react-icons/bi';
import { HiOutlineTrash } from 'react-icons/hi';
import BrushSizeSelector from './BrushSizeSelector';
import ColorPicker from './ColorPicker';
import CopyUrl from '../UI/CopyUrl';

interface ToolbarProps {
  color: string;
  setColor: (color: string) => void;
  eraser: boolean;
  setEraser: (eraser: boolean) => void;
  brushSize: number;
  setBrushSize: (size: number) => void;
  isAdmin: boolean;
  onUndo: () => void;
  onRedo: () => void;
  onClear: () => void;
  onDelete: () => void;
}

export default function Toolbar({
  color,
  setColor,
  eraser,
  setEraser,
  brushSize,
  setBrushSize,
  isAdmin,
  onUndo,
  onRedo,
  onClear,
  onDelete,
}: ToolbarProps) {
  return (
    <div className="fixed left-2 top-50 z-50 flex flex-col items-center justify-center gap-5 rounded-2xl border border-indigo-500 bg-gray-200 px-0 py-4 shadow-inner">
      <ColorPicker color={color} setColor={setColor} isEraserActive={eraser} setIsEraserActive={setEraser} />
      {isAdmin && (
        <>
          <button type="button" aria-label="Undo" title="Undo" className="cursor-pointer rounded-lg text-indigo-500" onClick={onUndo}>
            <FaUndo className="text-sm" />
          </button>
          <button type="button" aria-label="Redo" title="Redo" className="cursor-pointer rounded-lg text-indigo-500" onClick={onRedo}>
            <FaRedo className="text-sm" />
          </button>
          <button type="button" aria-label="Clear board" title="Clear board" className="cursor-pointer text-gray-400 hover:text-indigo-500" onClick={onClear}>
            <BiReset className="text-xl" />
          </button>
          <button type="button" aria-label="Delete room" title="Delete room" className="cursor-pointer text-gray-400 hover:text-red-500" onClick={onDelete}>
            <HiOutlineTrash className="text-xl" />
          </button>
        </>
      )}
      <BrushSizeSelector brushSize={brushSize} setBrushSize={setBrushSize} />
      <button
        type="button"
        aria-label={eraser ? 'Turn eraser off' : 'Eraser'}
        aria-pressed={eraser}
        title="Eraser"
        onClick={() => setEraser(!eraser)}
      >
        <BsEraserFill className={`text-lg ${eraser ? 'text-indigo-600' : 'text-gray-400'}`} />
      </button>
      <CopyUrl />
    </div>
  );
}
