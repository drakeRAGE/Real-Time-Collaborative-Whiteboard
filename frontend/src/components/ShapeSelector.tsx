import { useState, type ReactNode } from 'react';
import { FaArrowDown, FaArrowLeft, FaArrowRight, FaArrowUp, FaCircle, FaPencilAlt, FaStar } from 'react-icons/fa';
import { BsFillPentagonFill, BsHexagonFill, BsTriangleFill } from 'react-icons/bs';
import { BiSolidRectangle } from 'react-icons/bi';
import { GoSquareFill } from 'react-icons/go';
import { GiArrowCursor } from 'react-icons/gi';

const OPTIONS: { label: string; shape: string | null; icon: ReactNode }[] = [
  { label: 'Cursor', shape: 'none', icon: <GiArrowCursor size={18} /> },
  { label: 'Pencil', shape: null, icon: <FaPencilAlt size={18} /> },
  { label: 'Square', shape: 'square', icon: <GoSquareFill size={18} /> },
  { label: 'Rectangle', shape: 'rectangle', icon: <BiSolidRectangle size={18} /> },
  { label: 'Circle', shape: 'circle', icon: <FaCircle size={18} /> },
  { label: 'Triangle', shape: 'triangle', icon: <BsTriangleFill size={18} /> },
  { label: 'Star', shape: 'star', icon: <FaStar size={18} /> },
  { label: 'Pentagon', shape: 'pentagon', icon: <BsFillPentagonFill size={18} /> },
  { label: 'Hexagon', shape: 'hexagon', icon: <BsHexagonFill size={18} /> },
  { label: 'Arrow up', shape: 'arrowUp', icon: <FaArrowUp size={18} /> },
  { label: 'Arrow down', shape: 'arrowDown', icon: <FaArrowDown size={18} /> },
  { label: 'Arrow left', shape: 'arrowLeft', icon: <FaArrowLeft size={18} /> },
  { label: 'Arrow right', shape: 'arrowRight', icon: <FaArrowRight size={18} /> },
];

export default function ShapeSelector({
  selectedShape,
  setSelectedShape,
  isEraserActive,
}: {
  selectedShape: string | null;
  setSelectedShape: (shape: string | null) => void;
  isEraserActive: boolean;
}) {
  const [notice, setNotice] = useState(false);

  return (
    <>
      <div className="fixed top-35 right-8 z-100 flex flex-col items-center gap-2">
        {OPTIONS.map(({ label, shape, icon }) => {
          const selected = selectedShape === shape || (shape === null && !selectedShape);
          return (
            <button
              key={label}
              type="button"
              aria-label={label}
              aria-pressed={selected}
              title={label}
              onClick={() => {
                if (isEraserActive) {
                  setNotice(true);
                  setTimeout(() => setNotice(false), 2500);
                  return;
                }
                setSelectedShape(shape);
              }}
              className={`rounded-lg border p-2 ${selected ? 'border-indigo-500 bg-indigo-500 text-white' : 'border-indigo-400 bg-transparent text-indigo-400'}`}
            >
              {icon}
            </button>
          );
        })}
      </div>
      {notice && (
        <div className="fixed top-5 left-1/2 z-50 -translate-x-1/2 rounded-lg border border-yellow-300 bg-yellow-100 px-4 py-2 text-sm text-yellow-800">
          Eraser is active. Turn it off to select a shape.
        </div>
      )}
    </>
  );
}
