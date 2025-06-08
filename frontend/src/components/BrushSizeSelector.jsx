import React from 'react';
import Slider from '@mui/material/Slider';
import { styled } from '@mui/material/styles';

const CustomSlider = styled(Slider)({
    width: 150,
    color: '#9866ce',
    '& .MuiSlider-thumb': {
        height: 24,
        width: 24,
        backgroundColor: '#fff',
        border: '2px solid currentColor',
    },
});

function BrushSizeSelector({ brushSize, setBrushSize }) {
    return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
            <label style={{ fontSize: '2rem', fontFamily: 'serif' }}>Brush Size:</label>
            <CustomSlider
                value={brushSize}
                onChange={(e, newValue) => setBrushSize(newValue)}
                min={1}
                max={50}
                aria-labelledby="brush-size-slider"
            />
            <span style={{ fontSize: '1.5rem', fontFamily: 'serif' }}>{brushSize}</span>
        </div>
    );
}

export default BrushSizeSelector;