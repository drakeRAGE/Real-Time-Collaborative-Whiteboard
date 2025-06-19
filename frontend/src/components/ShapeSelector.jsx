import React from 'react';
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
