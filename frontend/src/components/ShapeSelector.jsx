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
            <div className="flex items-center gap-2 flex-wrap">
                {shapeOptions.map(({ label, shape, icon }) => {
                    const isSelected = selectedShape === shape || (shape === null && !selectedShape);
                    return (
                        <button
                            key={label}
                            onClick={() => handleShapeClick(shape)}
                            className={`p-2 rounded-lg border transition ${isSelected
                                    ? "bg-indigo-500 text-white border-indigo-500"
                                    : "bg-transparent text-indigo-400 border-indigo-400 hover:bg-indigo-100 hover:text-indigo-600"
                                }`}
                            title={label}
                        >
                            {icon}
                        </button>
                    );
                })}
            </div>

            {showEraserToast && (
                <div className="fixed top-5 left-1/2 -translate-x-1/2 bg-yellow-100 text-yellow-800 px-4 py-2 rounded-lg shadow-lg border border-yellow-300 z-50 text-sm">
                    🧽 Eraser is active. Please deactivate it to select a shape.
                </div>
            )}
        </>
    );
}

export default ShapeSelector;
