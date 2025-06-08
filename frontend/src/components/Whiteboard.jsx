import { useRef, useEffect, useState } from "react";
import { io } from "socket.io-client";
import BrushSizeSelector from "./BrushSizeSelector";

const socket = io("http://localhost:5000");

function Whiteboard() {
    const canvasRef = useRef(null);
    const ctxRef = useRef(null);
    const [isDrawing, setIsDrawing] = useState(false);
    const [position, setPosition] = useState({ x: 0, y: 0 });
    const [color, setColor] = useState("#000000");
    const [brushSize, setBrushSize] = useState(3);

    useEffect(() => {
        const canvas = canvasRef.current;
        const parent = canvas.parentElement;

        canvas.width = parent.clientWidth;
        canvas.height = parent.clientHeight;

        const ctx = canvas.getContext("2d");
        ctx.lineCap = "round";
        ctx.strokeStyle = color;
        ctx.lineWidth = brushSize;

        ctxRef.current = ctx;

        socket.on("draw", ({ x0, y0, x1, y1, color: lineColor, size }) => {
            drawLine(x0, y0, x1, y1, false, lineColor, size);
        });

        return () => socket.off("draw");
    }, []);

    const startDrawing = ({ nativeEvent }) => {
        const { offsetX, offsetY } = nativeEvent;
        setPosition({ x: offsetX, y: offsetY });
        setIsDrawing(true);
    };

    const endDrawing = () => setIsDrawing(false);

    const draw = ({ nativeEvent }) => {
        if (!isDrawing) return;

        const { offsetX, offsetY } = nativeEvent;
        drawLine(position.x, position.y, offsetX, offsetY, true, color);
        setPosition({ x: offsetX, y: offsetY });
    };

    const drawLine = (x0, y0, x1, y1, emit, lineColor = color, size = brushSize) => {
        const ctx = ctxRef.current;
        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.strokeStyle = lineColor;
        ctx.lineWidth = size;
        ctx.stroke();
        ctx.closePath();

        if (!emit) return;
        socket.emit("draw", { x0, y0, x1, y1, color: lineColor, size });
    };

    return (
        <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
            <header
                style={{
                    padding: "1rem",
                    textAlign: "center",
                    background: "linear-gradient(to right, #f472b6, #d8b4fe)",
                    color: "#9866ce",
                    fontSize: "2.5rem",
                    fontWeight: "bold",
                    fontFamily: "serif",
                }}
            >
                Collaborative Whiteboard
            </header>

            <div style={{
                display: "flex",
                justifyContent: "center",
                alignItems: "center",
                gap: "2rem",
                padding: "0.5rem",
                background: "#f3f4f6"
            }}>
                <label htmlFor="color" style={{ fontSize: '2rem', fontFamily: 'serif' }}>
                    Choose Color:
                </label>
                <input
                    type="color"
                    value={color}
                    onChange={(e) => setColor(e.target.value)}
                    style={{ width: "50px", height: "50px" }}
                />
                <BrushSizeSelector brushSize={brushSize} setBrushSize={setBrushSize} />
            </div>

            <div style={{ flexGrow: 1, position: "relative" }}>
                <canvas
                    ref={canvasRef}
                    onMouseDown={startDrawing}
                    onMouseUp={endDrawing}
                    onMouseOut={endDrawing}
                    onMouseMove={draw}
                    style={{ display: "block", width: "100%", height: "100%", cursor: "crosshair" }}
                />
            </div>
        </div>
    );
}

export default Whiteboard;
