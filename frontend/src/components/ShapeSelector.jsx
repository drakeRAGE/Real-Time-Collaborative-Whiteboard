import React, { useState } from 'react';
import {
    FaCircle, FaPencilAlt, FaArrowUp, FaArrowDown,
    FaArrowLeft, FaArrowRight, FaStar,
} from 'react-icons/fa';
import { BsTriangleFill, BsFillPentagonFill, BsHexagonFill } from "react-icons/bs";
import { BiSolidRectangle } from "react-icons/bi";
import { GoSquareFill } from "react-icons/go";

const shapeOptions = [
    { label: 'pencil', shape: null, icon: <FaPencilAlt size={24} /> },
    { label: 'square', shape: 'square', icon: <GoSquareFill size={24} /> },
    { label: 'rectangle', shape: 'rectangle', icon: <BiSolidRectangle size={24} /> },
    { label: 'circle', shape: 'circle', icon: <FaCircle size={24} /> },
    { label: 'triangle', shape: 'triangle', icon: <BsTriangleFill size={24} /> },
    { label: 'star', shape: 'star', icon: <FaStar size={24} /> },
    { label: 'Pentagon', shape: 'pentagon', icon: <BsFillPentagonFill size={24} /> },
    { label: 'Hexagon', shape: 'hexagon', icon: <BsHexagonFill size={24} /> },
    { label: 'arrowUp', shape: 'arrowUp', icon: <FaArrowUp size={24} /> },
    { label: 'arrowDown', shape: 'arrowDown', icon: <FaArrowDown size={24} /> },
    { label: 'arrowLeft', shape: 'arrowLeft', icon: <FaArrowLeft size={24} /> },
    { label: 'arrowRight', shape: 'arrowRight', icon: <FaArrowRight size={24} /> },
];

function ShapeSelector({ selectedShape, setSelectedShape, isEraserActive }) {
    const [showEraserToast, setShowEraserToast] = useState(false);

    const handleShapeClick = (shape) => {
        if (isEraserActive) {
            setShowEraserToast(true);
            setTimeout(() => setShowEraserToast(false), 2500);
            return;
        }
        setSelectedShape(shape);
    };

    return (
        <>
            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', flexWrap: 'wrap' }}>
                {shapeOptions.map(({ label, shape, icon }) => {
                    const isSelected = selectedShape === shape;
                    return (
                        <button
                            key={label}
                            onClick={() => handleShapeClick(shape)}
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

            {showEraserToast && (
                <div style={toastStyle('#fffbe6', '#d48806', '#ffe58f')}>
                    <span>🧽 Eraser is active. Please deactivate it to select a shape.</span>
                </div>
            )}
        </>
    );
}

// Reusable toast style
function toastStyle(bgColor, textColor, borderColor) {
    return {
        position: 'fixed',
        top: '20px',
        left: '50%',
        transform: 'translateX(-50%)',
        backgroundColor: bgColor,
        color: textColor,
        padding: '10px 16px',
        border: `1px solid ${borderColor}`,
        borderRadius: '10px',
        boxShadow: '0 8px 20px rgba(0,0,0,0.15)',
        fontWeight: 500,
        zIndex: 10000,
        fontFamily: 'system-ui, sans-serif',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        minWidth: '300px',
        maxWidth: '90%',
        animation: 'fadeIn 0.3s ease-in-out'
    };
}

export default ShapeSelector;
