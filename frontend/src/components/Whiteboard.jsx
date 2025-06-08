import { useRef, useEffect, useState } from "react";
import { io } from "socket.io-client";

const socket = io("http://localhost:5000");

function Whiteboard() {
    const canvasRef = useRef(null);
    const ctxRef = useRef(null);
    const [isDrawing, setIsDrawing] = useState(false);
    const [position, setPosition] = useState({ x: 0, y: 0 });

    useEffect(() => {
        const canvas = canvasRef.current;
        const parent = canvas.parentElement;

        canvas.width = parent.clientWidth;
        canvas.height = parent.clientHeight;

        const ctx = canvas.getContext("2d");
        ctx.lineCap = "round";
        ctx.strokeStyle = "black";
        ctx.lineWidth = 3;

        ctxRef.current = ctx;

        socket.on("draw", ({ x0, y0, x1, y1 }) => {
            drawLine(x0, y0, x1, y1, false);
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
        drawLine(position.x, position.y, offsetX, offsetY, true);
        setPosition({ x: offsetX, y: offsetY });
    };

    const drawLine = (x0, y0, x1, y1, emit) => {
        const ctx = ctxRef.current;
        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.stroke();
        ctx.closePath();

        if (!emit) return;
        socket.emit("draw", { x0, y0, x1, y1 });
    };

    return (
        <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
            <header
                style={{
                    padding: "1rem",
                    textAlign: "center",
                    background: "linear-gradient(to right, #f472b6, #d8b4fe)", // pink-400 to purple-300
                    color: "#9866ce",
                    fontSize: "1.5rem",
                    fontWeight: "bold"
                }}
            >
                Collaborative Whiteboard
            </header>


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
