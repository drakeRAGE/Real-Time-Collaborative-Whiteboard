import React from 'react';
import { FaSquare, FaCircle, FaPencilAlt } from 'react-icons/fa';

function ShapeSelector({ selectedShape, setSelectedShape }) {
    return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
            <button
                onClick={() => setSelectedShape(null)}
                style={{
                    background: !selectedShape ? '#9866ce' : 'transparent',
                    color: !selectedShape ? 'white' : '#9866ce',
                    border: '2px solid #9866ce',
                    borderRadius: '4px',
                    padding: '8px',
                    cursor: 'pointer'
                }}
            >
                <FaPencilAlt size={24} />
            </button>
            <button
                onClick={() => setSelectedShape('rectangle')}
                style={{
                    background: selectedShape === 'rectangle' ? '#9866ce' : 'transparent',
                    color: selectedShape === 'rectangle' ? 'white' : '#9866ce',
                    border: '2px solid #9866ce',
                    borderRadius: '4px',
                    padding: '8px',
                    cursor: 'pointer'
                }}
            >
                <FaSquare size={24} />
            </button>
            <button
                onClick={() => setSelectedShape('circle')}
                style={{
                    background: selectedShape === 'circle' ? '#9866ce' : 'transparent',
                    color: selectedShape === 'circle' ? 'white' : '#9866ce',
                    border: '2px solid #9866ce',
                    borderRadius: '4px',
                    padding: '8px',
                    cursor: 'pointer'
                }}
            >
                <FaCircle size={24} />
            </button>
        </div>
    );
}

export default ShapeSelector;