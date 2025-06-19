import React from 'react';

const sizes = [4, 8, 12, 16, 20, 24, 28, 32]; // You can adjust or add more sizes

function BrushSizeSelector({ brushSize, setBrushSize }) {
    return (
        <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-start',
            fontFamily: 'sans-serif',
        }}>

            <div style={{
                display: 'flex',
                gap: '1rem',
                alignItems: 'center',
                flexWrap: 'wrap',
            }}>
                {sizes.map((size, index) => (
                    <div
                        key={index}
                        onClick={() => setBrushSize(size)}
                        style={{
                            width: size,
                            height: size,
                            borderRadius: '50%',
                            backgroundColor: brushSize === size ? '#c084fc' : '#E5e7eb',
                            border: brushSize === size ? '2px solid #1976d2' : '2px solid transparent',
                            cursor: 'pointer',
                            transition: 'transform 0.2s ease',
                        }}
                        title={`Size ${size}`}
                    />
                ))}
            </div>
        </div>
    );
}

export default BrushSizeSelector;
