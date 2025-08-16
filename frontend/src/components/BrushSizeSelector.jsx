import React, { useEffect } from "react";

const sizes = [4, 8, 12, 16, 20, 24, 28, 32]; // can adjust

function BrushSizeToggle({ brushSize, setBrushSize }) {
    // ensure there's a default smallest size on mount if parent didn't set it
    useEffect(() => {
        if (brushSize == null) setBrushSize(sizes[0]);
    }, [brushSize, setBrushSize]);

    const handleClick = () => {
        const currentIndex = sizes.indexOf(brushSize);
        const nextIndex = currentIndex === -1 ? 0 : (currentIndex + 1) % sizes.length;
        setBrushSize(sizes[nextIndex]);
    };

    const label = `Brush size ${brushSize}px — click to change`;

    return (
        <button
            type="button"
            onClick={handleClick}
            aria-label={label}
            title={label}
            className="flex items-center h-12 w-12 cursor-pointer justify-center p-2 rounded-md focus:outline-nonea"
        >
            {/* The round "brush" visual */}
            <div
                className={`rounded-full shadow-md transition-all duration-150 ease-out transform`}
                style={{
                    width: brushSize,
                    height: brushSize,
                    // subtle visual tweaks
                    boxShadow: "0 2px 6px rgba(0,0,0,0.12)",
                    background: "black", // you can change
                }}
            />
        </button>
    );
}

export default BrushSizeToggle;
