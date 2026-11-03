import { BsPaletteFill } from 'react-icons/bs';

interface ColorPickerProps {
  color: string;
  setColor: (color: string) => void;
  isEraserActive: boolean;
  setIsEraserActive: (active: boolean) => void;
}

export default function ColorPicker({ color, setColor, isEraserActive, setIsEraserActive }: ColorPickerProps) {
  return (
    <div className="flex items-center">
      <label htmlFor="brush-color" className="relative cursor-pointer">
        <span className="sr-only">Choose a color</span>
        <input
          id="brush-color"
          type="color"
          value={color}
          aria-label="Choose a color"
          onChange={(event) => {
            if (isEraserActive && event.target.value !== '#ffffff') setIsEraserActive(false);
            setColor(event.target.value);
          }}
          className="absolute inset-0 cursor-pointer opacity-0"
        />
        <BsPaletteFill className="h-5 w-5 text-indigo-500" style={{ color }} />
      </label>
    </div>
  );
}
