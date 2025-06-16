import { useRef, useEffect, useState } from "react";
import { io } from "socket.io-client";
import { useParams, useNavigate } from "react-router-dom";
import BrushSizeSelector from "./BrushSizeSelector";
import { clearCanvas, deleteRoom } from "../utils/canvasUtils";
import UsersList from './UsersList';

function Whiteboard() {
    const { roomId } = useParams();
    const [socket, setSocket] = useState(null);
    const [connectionError, setConnectionError] = useState(null);
    const canvasRef = useRef(null);
    const ctxRef = useRef(null);
    const [isDrawing, setIsDrawing] = useState(false);
    const [position, setPosition] = useState({ x: 0, y: 0 });
    const [color, setColor] = useState("#000000");
    const [brushSize, setBrushSize] = useState(3);

    const handleClearBoard = () => {
        if (!socket) return;
        clearCanvas(canvasRef);
        socket.emit('clear');
    };
    useEffect(() => {
        const newSocket = io("http://localhost:5000", {
            reconnection: true,
            reconnectionAttempts: 5,
            reconnectionDelay: 1000,
            transports: ['websocket', 'polling']
        });

        newSocket.on("connect_error", (err) => {
            console.error("Connection error:", err);
            setConnectionError("Failed to connect to server");
        });

        newSocket.on("connect", () => {
            setConnectionError(null);
            console.log("Connected to server");
        });

        newSocket.on("initialData", (drawings) => {
            const ctx = ctxRef.current;
            if (!ctx) return;

            // Clear canvas first
            clearCanvas(canvasRef);

            // Redraw all existing drawings
            drawings.forEach(({ x0, y0, x1, y1, color: lineColor, size }) => {
                drawLine(x0, y0, x1, y1, false, lineColor, size);
            });
        });

        setSocket(newSocket);

        return () => {
            newSocket.disconnect();
        };
    }, []);

    useEffect(() => {
        if (!socket || !roomId) return;

        socket.emit('joinRoom', roomId);

        const canvas = canvasRef.current;
        const parent = canvas.parentElement;

        canvas.width = parent.clientWidth;
        canvas.height = parent.clientHeight;
        clearCanvas(canvasRef);

        const ctx = canvas.getContext("2d");
        ctx.lineCap = "round";
        ctx.strokeStyle = color;
        ctx.lineWidth = brushSize;
        ctxRef.current = ctx;

        socket.on("draw", ({ x0, y0, x1, y1, color: lineColor, size }) => {
            drawLine(x0, y0, x1, y1, false, lineColor, size);
        });

        // Add clear event listener
        socket.on("clear", () => {
            clearCanvas(canvasRef);
        });

        return () => {
            socket.off("draw");
            socket.off("clear");
        };
    }, [socket, roomId]);

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
        if (!ctx) return; // Add null check

        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.strokeStyle = lineColor;
        ctx.lineWidth = size;
        ctx.stroke();
        ctx.closePath();

        if (!emit || !socket) return; // Also check socket
        socket.emit("draw", { x0, y0, x1, y1, color: lineColor, size });
    };

    const navigate = useNavigate();
    const [showDeleteModal, setShowDeleteModal] = useState(false);

    const handleDeleteRoom = async () => {
        const success = await deleteRoom(socket, roomId);
        if (success) {
            navigate('/'); // Redirect to home after deletion
        }
    };

    // Add this useEffect to handle room deletion from other clients
    useEffect(() => {
        if (!socket) return;

        socket.on('roomDeleted', () => {
            navigate('/');
        });

        return () => {
            socket.off('roomDeleted');
        };
    }, [socket, navigate]);

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
                <button
                    onClick={handleClearBoard}
                    style={{
                        padding: '0.5rem 1rem',
                        fontSize: '1.2rem',
                        fontFamily: 'serif',
                        background: '#9866ce',
                        color: 'white',
                        border: 'none',
                        borderRadius: '4px',
                        cursor: 'pointer'
                    }}
                >
                    Clear Board
                </button>

                <button
                    onClick={() => setShowDeleteModal(true)}
                    style={{
                        padding: '0.5rem 1rem',
                        fontSize: '1.2rem',
                        fontFamily: 'serif',
                        background: '#ef4444',
                        color: 'white',
                        border: 'none',
                        borderRadius: '4px',
                        cursor: 'pointer'
                    }}
                >
                    Delete Room
                </button>
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
                <UsersList socket={socket} roomId={roomId} />
            </div>

            {/* Delete Confirmation Modal */}
            {showDeleteModal && (
                <div style={{
                    position: 'fixed',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    backgroundColor: 'rgba(0,0,0,0.5)',
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    zIndex: 1000
                }}>
                    <div style={{
                        backgroundColor: 'white',
                        padding: '2rem',
                        borderRadius: '8px',
                        maxWidth: '400px'
                    }}>
                        <h2 style={{ marginBottom: '1rem' }}>Delete Room</h2>
                        <p>Are you sure you want to delete this room? This action cannot be undone.</p>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '1rem', marginTop: '1.5rem' }}>
                            <button
                                onClick={() => setShowDeleteModal(false)}
                                style={{
                                    padding: '0.5rem 1rem',
                                    background: '#e5e7eb',
                                    border: 'none',
                                    borderRadius: '4px',
                                    cursor: 'pointer'
                                }}
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleDeleteRoom}
                                style={{
                                    padding: '0.5rem 1rem',
                                    background: '#ef4444',
                                    color: 'white',
                                    border: 'none',
                                    borderRadius: '4px',
                                    cursor: 'pointer'
                                }}
                            >
                                Delete
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default Whiteboard;
