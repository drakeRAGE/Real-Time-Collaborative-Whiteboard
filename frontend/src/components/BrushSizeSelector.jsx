import React from 'react';

const sizes = [4, 8, 12, 16, 20, 24, 28, 32]; // You can adjust or add more sizes

function BrushSizeSelector({ brushSize, setBrushSize }) {
    return (
        <div className="flex flex-col items-start">
            <div className="flex gap-3 items-center flex-wrap">
                {sizes.map((size, index) => (
                    <div
                        key={index}
                        onClick={() => setBrushSize(size)}
                        className={`rounded-full cursor-pointer transition-transform transform hover:scale-110 ${brushSize === size
                                ? "bg-purple-400 border-2 border-blue-500"
                                : "bg-gray-300 border-2 border-transparent"
                            }`}
                        style={{
                            width: size,
                            height: size,
                        }}
                        title={`Size ${size}`}
                    />
                ))}
            </div>
        </div>
    );
}

export default BrushSizeSelector;
