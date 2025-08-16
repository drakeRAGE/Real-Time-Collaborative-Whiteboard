import { BsPaletteFill } from "react-icons/bs";

function ColorPicker({ color, setColor, isEraserActive, setIsEraserActive }) {
    return (
        <div className="flex items-center">
            <label className="relative cursor-pointer">
                {/* Hidden color input */}
                <input
                    type="color"
                    value={color}
                    onChange={(e) => {
                        if (isEraserActive && e.target.value !== "#ffffff") {
                            setIsEraserActive(false);
                        }
                        setColor(e.target.value);
                    }}
                    className="absolute inset-0 opacity-0 cursor-pointer"
                />

                {/* Palette Icon */}
                <BsPaletteFill
                    className="w-5 h-5 text-indigo-500 hover:scale-110 transition-transform"
                    style={{ color }}
                    title="Choose a color"
                />
            </label>
        </div>
    );
}

export default ColorPicker;
