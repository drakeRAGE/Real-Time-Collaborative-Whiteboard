import { useRef, useEffect, useState } from "react";
import { io } from "socket.io-client";

const socket = io("http://localhost:5000");

function Whiteboard() {
    const canvasRef = useRef(null);
    const [isDrawing, setIsDrawing] = useState(false);

    console.log("canvas ref", canvasRef.current);

    useEffect(() => {
        const canvas = canvasRef.current;
        const ctx = canvas.getContext("2d");

        const drawLine = ({ x0, y0, x1, y1 }) => {
            ctx.beginPath();
            ctx.moveTo(x0, y0);
            ctx.lineTo(x1, y1);
            ctx.stroke();
        };

        socket.on("draw", drawLine);
    }, []);

    const handleMouseDown = e => setIsDrawing(true);
    const handleMouseUp = e => setIsDrawing(false);
    const handleMouseMove = e => {
        if (!isDrawing) return;
        const canvas = canvasRef.current;
        const rect = canvas.getBoundingClientRect();

        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        socket.emit("draw", { x0: x, y0: y, x1: x + 1, y1: y + 1 });
    };

    return (
        <canvas
            ref={canvasRef}
            width={800}
            height={600}
            style={{ border: "1px solid black" }}
            onMouseDown={handleMouseDown}
            onMouseUp={handleMouseUp}
            onMouseMove={handleMouseMove}
        />
    );
}

export default Whiteboard;
