import { useEffect } from 'react';

const SIZES = [4, 8, 12, 16, 20, 24, 28, 32];

export default function BrushSizeSelector({ brushSize, setBrushSize }: { brushSize: number; setBrushSize: (size: number) => void }) {
  useEffect(() => {
    if (!SIZES.includes(brushSize)) setBrushSize(SIZES[0] ?? 4);
  }, [brushSize, setBrushSize]);

  const label = `Brush size ${brushSize} pixels`;
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className="flex h-12 w-12 cursor-pointer items-center justify-center rounded-md"
      onClick={() => {
        const index = SIZES.indexOf(brushSize);
        const next = SIZES[(index === -1 ? 0 : index + 1) % SIZES.length] ?? 4;
        setBrushSize(next);
      }}
    >
      <span className="rounded-full bg-black" style={{ width: brushSize, height: brushSize }} />
    </button>
  );
}
