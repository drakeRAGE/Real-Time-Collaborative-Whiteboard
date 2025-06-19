import React from 'react';
import {
    FaSquare, FaCircle, FaPencilAlt, FaArrowUp, FaArrowDown,
    FaArrowLeft, FaArrowRight, FaStar, FaShapes,
} from 'react-icons/fa';

const shapeOptions = [
    { label: 'pencil', shape: null, icon: <FaPencilAlt size={24} /> },
    { label: 'rectangle', shape: 'rectangle', icon: <FaSquare size={24} /> },
    { label: 'circle', shape: 'circle', icon: <FaCircle size={24} /> },
    { label: 'triangle', shape: 'triangle', icon: <FaShapes size={24} /> },
    { label: 'rightTriangle', shape: 'rightTriangle', icon: <FaShapes size={24} /> },
    { label: 'star', shape: 'star', icon: <FaStar size={24} /> },
    { label: 'arrowUp', shape: 'arrowUp', icon: <FaArrowUp size={24} /> },
    { label: 'arrowDown', shape: 'arrowDown', icon: <FaArrowDown size={24} /> },
    { label: 'arrowLeft', shape: 'arrowLeft', icon: <FaArrowLeft size={24} /> },
    { label: 'arrowRight', shape: 'arrowRight', icon: <FaArrowRight size={24} /> },
];

function ShapeSelector({ selectedShape, setSelectedShape }) {
    return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', flexWrap: 'wrap' }}>
            {shapeOptions.map(({ label, shape, icon }) => {
                const isSelected = selectedShape === shape;
                return (
                    <button
                        key={label}
                        onClick={() => setSelectedShape(shape)}
                        style={{
                            background: isSelected || (shape === null && !selectedShape) ? '#9866ce' : 'transparent',
                            color: isSelected || (shape === null && !selectedShape) ? 'white' : '#9866ce',
                            border: '2px solid #9866ce',
                            borderRadius: '4px',
                            padding: '8px',
                            cursor: 'pointer'
                        }}
                        title={label}
                    >
                        {icon}
                    </button>
                );
            })}
        </div>
    );
}

export default ShapeSelector;
