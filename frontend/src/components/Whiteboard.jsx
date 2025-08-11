import { useRef, useEffect, useState } from "react";
import { io } from "socket.io-client";
import { useParams, useNavigate } from "react-router-dom";
import BrushSizeSelector from "./BrushSizeSelector";
import { clearCanvas, deleteRoom, startDrawingUtils, endDrawingUtils } from "../utils/canvasUtils";

// All drawshape fncs from utils
import { drawRectangle, drawCircle, drawSquare, drawTriangle, drawStar, drawPentagon, drawHexagon, drawArrowUp, drawArrowDown, drawArrowLeft, drawArrowRight } from '../utils/canvasUtils';
import UsersList from './UsersList';
import LiveCursors from './LiveCursors';
import Modal from "../UI/Modal";
import CopyUrl from "../UI/CopyUrl";
import ShapeSelector from './ShapeSelector';
import { MdDelete } from "react-icons/md";
import { GrClear } from "react-icons/gr";
import { BsEraserFill } from "react-icons/bs";
import { supabase } from "../utils/supabase";
import { initializeSocket } from "../utils/socket";
import { useAuth } from "../context/AuthContext";

function Whiteboard() {
    const { roomId } = useParams();
    const [socket, setSocket] = useState(null);
    const [connectionError, setConnectionError] = useState(null);
    const canvasRef = useRef(null);
    const ctxRef = useRef(null);
    const [isDrawing, setIsDrawing] = useState(false);
    const [position, setPosition] = useState({ x: 0, y: 0 });
    const [color, setColor] = useState("#000000");
    const [brushSize, setBrushSize] = useState(4);
    const [showClearModal, setShowClearModal] = useState(false);
    const [selectedShape, setSelectedShape] = useState(null);
    const [startPos, setStartPos] = useState(null);
    const navigate = useNavigate();
    const [showDeleteModal, setShowDeleteModal] = useState(false);
    const [isEraserActive, setIsEraserActive] = useState(false);

    const handleClearBoard = () => {
        if (!socket) return;
        clearCanvas(canvasRef);
        socket.emit('clear');
    };
    useEffect(() => {
        if (canvasRef.current) {
            clearCanvas(canvasRef);
        }

        let newSocket;

        const setupSocket = async () => {
            const { data: { session } } = await supabase.auth.getSession();
            if (!session) {
                console.error("No session found");
                setConnectionError("You are not logged in");
                return;
            }

            newSocket = io("http://localhost:5000", {
                auth: {
                    token: session.access_token // Send JWT to backend
                },
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

            newSocket.on("initialData", (data) => {
                const ctx = ctxRef.current;
                if (!ctx) return;

                clearCanvas(canvasRef);

                // Safely get drawings
                const drawingsArray = Array.isArray(data.drawings) ? data.drawings : [];
                drawingsArray.forEach(({ x0, y0, x1, y1, color: lineColor, size, shape }) => {
                    ctx.beginPath();

                    if (shape) {
                        if (shape === 'rectangle') {
                            ctx.rect(x0, y0, x1 - x0, y1 - y0);
                        } else if (shape === 'circle') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const radiusX = Math.abs(x1 - x0) / 2;
                            const radiusY = Math.abs(y1 - y0) / 2;
                            ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, 2 * Math.PI);
                        } else if (shape === 'square') {
                            const sideLength = Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0));
                            const signX = x1 > x0 ? 1 : -1;
                            const signY = y1 > y0 ? 1 : -1;
                            ctx.rect(x0, y0, signX * sideLength, signY * sideLength);
                        } else if (shape === 'triangle') {
                            ctx.moveTo(x0, y1);
                            ctx.lineTo(x1, y1);
                            ctx.lineTo((x0 + x1) / 2, y0);
                            ctx.closePath();
                        } else if (shape === 'star') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const outerRadius = Math.min(Math.abs(x1 - x0), Math.abs(y1 - y0)) / 2;
                            const innerRadius = outerRadius / 2;

                            ctx.moveTo(centerX + outerRadius * Math.cos(0), centerY + outerRadius * Math.sin(0));
                            for (let i = 0; i < 10; i++) {
                                const angle = i * Math.PI / 5;
                                const radius = i % 2 === 0 ? outerRadius : innerRadius;
                                ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                            }
                            ctx.closePath();
                        } else if (shape === 'pentagon') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const radius = Math.min(Math.abs(x1 - x0), Math.abs(y1 - y0)) / 2;

                            ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
                            for (let i = 1; i <= 5; i++) {
                                const angle = i * 2 * Math.PI / 5;
                                ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                            }
                            ctx.closePath();
                        } else if (shape === 'hexagon') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const radius = Math.min(Math.abs(x1 - x0), Math.abs(y1 - y0)) / 2;

                            ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
                            for (let i = 1; i <= 6; i++) {
                                const angle = i * 2 * Math.PI / 6;
                                ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                            }
                            ctx.closePath();
                        } else if (shape === 'arrowUp') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const width = Math.abs(x1 - x0);
                            ctx.moveTo(centerX, y0);
                            ctx.lineTo(x0, centerY);
                            ctx.lineTo(centerX - width * 0.1, centerY);
                            ctx.lineTo(centerX - width * 0.1, y1);
                            ctx.lineTo(centerX + width * 0.1, y1);
                            ctx.lineTo(centerX + width * 0.1, centerY);
                            ctx.lineTo(x1, centerY);
                            ctx.closePath();

                        } else if (shape === 'arrowDown') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const width = Math.abs(x1 - x0);
                            ctx.moveTo(centerX, y1);
                            ctx.lineTo(x0, centerY);
                            ctx.lineTo(centerX - width * 0.1, centerY);
                            ctx.lineTo(centerX - width * 0.1, y0);
                            ctx.lineTo(centerX + width * 0.1, y0);
                            ctx.lineTo(centerX + width * 0.1, centerY);
                            ctx.lineTo(x1, centerY);
                            ctx.closePath();

                        } else if (shape === 'arrowLeft') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const height = Math.abs(y1 - y0);
                            ctx.moveTo(x0, centerY);
                            ctx.lineTo(centerX, y0);
                            ctx.lineTo(centerX, centerY - height * 0.1);
                            ctx.lineTo(x1, centerY - height * 0.1);
                            ctx.lineTo(x1, centerY + height * 0.1);
                            ctx.lineTo(centerX, centerY + height * 0.1);
                            ctx.lineTo(centerX, y1);
                            ctx.closePath();

                        } else if (shape === 'arrowRight') {
                            const centerX = (x0 + x1) / 2;
                            const centerY = (y0 + y1) / 2;
                            const height = Math.abs(y1 - y0);
                            ctx.moveTo(x1, centerY);
                            ctx.lineTo(centerX, y0);
                            ctx.lineTo(centerX, centerY - height * 0.1);
                            ctx.lineTo(x0, centerY - height * 0.1);
                            ctx.lineTo(x0, centerY + height * 0.1);
                            ctx.lineTo(centerX, centerY + height * 0.1);
                            ctx.lineTo(centerX, y1);
                            ctx.closePath();
                        }
                    } else {
                        // Handle regular line drawings
                        ctx.moveTo(x0, y0);
                        ctx.lineTo(x1, y1);
                    }

                    ctx.strokeStyle = lineColor;
                    ctx.lineWidth = size;
                    ctx.stroke();
                });
            });



            setSocket(newSocket);
        }

        setupSocket();

        return () => {
            if (newSocket) {
                newSocket.disconnect();
            }
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

    // Modify startDrawing to handle shapes
    const startDrawing = ({ nativeEvent }) => {
        startDrawingUtils(nativeEvent, setPosition, setIsDrawing, selectedShape, setStartPos);
    };

    const endDrawing = ({ nativeEvent }) => {
        endDrawingUtils(nativeEvent, isDrawing, position, color, selectedShape, drawShape, setStartPos, setIsDrawing, drawLine);
    };

    const draw = ({ nativeEvent }) => {
        if (!isDrawing || selectedShape) return; // Only draw if no shape is selected

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

        socket.on('drawShape', (data) => {
            const ctx = ctxRef.current;
            if (!ctx) return;

            ctx.beginPath();

            if (data.shape === 'rectangle') {
                ctx.rect(data.x0, data.y0, data.x1 - data.x0, data.y1 - data.y0);
            } else if (data.shape === 'circle') {
                // Calculate center point and radii
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const radiusX = Math.abs(data.x1 - data.x0) / 2;
                const radiusY = Math.abs(data.y1 - data.y0) / 2;

                // Draw ellipse/oval
                ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, 2 * Math.PI);
            } else if (data.shape === 'square') {
                const sideLength = Math.max(Math.abs(data.x1 - data.x0), Math.abs(data.y1 - data.y0));
                const signX = data.x1 > data.x0 ? 1 : -1;
                const signY = data.y1 > data.y0 ? 1 : -1;
                ctx.rect(data.x0, data.y0, signX * sideLength, signY * sideLength);
            } else if (data.shape === 'triangle') {
                ctx.moveTo(data.x0, data.y1);
                ctx.lineTo(data.x1, data.y1);
                ctx.lineTo((data.x0 + data.x1) / 2, data.y0);
                ctx.closePath();
            } else if (data.shape === 'star') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const outerRadius = Math.min(Math.abs(data.x1 - data.x0), Math.abs(data.y1 - data.y0)) / 2;
                const innerRadius = outerRadius / 2;

                ctx.moveTo(centerX + outerRadius * Math.cos(0), centerY + outerRadius * Math.sin(0));
                for (let i = 0; i < 10; i++) {
                    const angle = i * Math.PI / 5;
                    const radius = i % 2 === 0 ? outerRadius : innerRadius;
                    ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                }
                ctx.closePath();
            } else if (data.shape === 'pentagon') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const radius = Math.min(Math.abs(data.x1 - data.x0), Math.abs(data.y1 - data.y0)) / 2;

                ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
                for (let i = 1; i <= 5; i++) {
                    const angle = i * 2 * Math.PI / 5;
                    ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                }
                ctx.closePath();
            } else if (data.shape === 'hexagon') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const radius = Math.min(Math.abs(data.x1 - data.x0), Math.abs(data.y1 - data.y0)) / 2;

                ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
                for (let i = 1; i <= 6; i++) {
                    const angle = i * 2 * Math.PI / 6;
                    ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
                }
                ctx.closePath();
            } else if (data.shape === 'arrowUp') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const width = Math.abs(data.x1 - data.x0);
                ctx.moveTo(centerX, data.y0);
                ctx.lineTo(data.x0, centerY);
                ctx.lineTo(centerX - width * 0.1, centerY);
                ctx.lineTo(centerX - width * 0.1, data.y1);
                ctx.lineTo(centerX + width * 0.1, data.y1);
                ctx.lineTo(centerX + width * 0.1, centerY);
                ctx.lineTo(data.x1, centerY);
                ctx.closePath();

            } else if (data.shape === 'arrowDown') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const width = Math.abs(data.x1 - data.x0);
                ctx.moveTo(centerX, data.y1);
                ctx.lineTo(data.x0, centerY);
                ctx.lineTo(centerX - width * 0.1, centerY);
                ctx.lineTo(centerX - width * 0.1, data.y0);
                ctx.lineTo(centerX + width * 0.1, data.y0);
                ctx.lineTo(centerX + width * 0.1, centerY);
                ctx.lineTo(data.x1, centerY);
                ctx.closePath();

            } else if (data.shape === 'arrowLeft') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const height = Math.abs(data.y1 - data.y0);
                ctx.moveTo(data.x0, centerY);
                ctx.lineTo(centerX, data.y0);
                ctx.lineTo(centerX, centerY - height * 0.1);
                ctx.lineTo(data.x1, centerY - height * 0.1);
                ctx.lineTo(data.x1, centerY + height * 0.1);
                ctx.lineTo(centerX, centerY + height * 0.1);
                ctx.lineTo(centerX, data.y1);
                ctx.closePath();

            } else if (data.shape === 'arrowRight') {
                const centerX = (data.x0 + data.x1) / 2;
                const centerY = (data.y0 + data.y1) / 2;
                const height = Math.abs(data.y1 - data.y0);
                ctx.moveTo(data.x1, centerY);
                ctx.lineTo(centerX, data.y0);
                ctx.lineTo(centerX, centerY - height * 0.1);
                ctx.lineTo(data.x0, centerY - height * 0.1);
                ctx.lineTo(data.x0, centerY + height * 0.1);
                ctx.lineTo(centerX, centerY + height * 0.1);
                ctx.lineTo(centerX, data.y1);
                ctx.closePath();
            }

            ctx.strokeStyle = data.color;
            ctx.lineWidth = data.size;
            ctx.stroke();
        });

        return () => {
            socket.off('roomDeleted');
            socket.off('drawShape');
        };
    }, [socket, navigate]);


    const drawShape = (endPos) => {
        if (!selectedShape || !startPos) return;

        const ctx = ctxRef.current;
        if (!ctx) return;

        ctx.beginPath();

        switch (selectedShape) {
            case 'rectangle':
                drawRectangle(ctx, startPos, endPos);
                break;
            case 'circle':
                drawCircle(ctx, startPos, endPos);
                break;
            case 'square':
                drawSquare(ctx, startPos, endPos);
                break;
            case 'triangle':
                drawTriangle(ctx, startPos, endPos);
                break;
            case 'star':
                drawStar(ctx, startPos, endPos);
                break;
            case 'pentagon':
                drawPentagon(ctx, startPos, endPos);
                break;
            case 'hexagon':
                drawHexagon(ctx, startPos, endPos);
                break;
            case 'arrowUp':
                drawArrowUp(ctx, startPos, endPos);
                break;
            case 'arrowDown':
                drawArrowDown(ctx, startPos, endPos);
                break;
            case 'arrowLeft':
                drawArrowLeft(ctx, startPos, endPos);
                break;
            case 'arrowRight':
                drawArrowRight(ctx, startPos, endPos);
                break;
        }

        ctx.strokeStyle = color;
        ctx.lineWidth = brushSize;
        ctx.stroke();

        if (socket) {
            socket.emit('drawShape', {
                x0: startPos.x,
                y0: startPos.y,
                x1: endPos.x,
                y1: endPos.y,
                color,
                size: brushSize,
                shape: selectedShape
            });
        }
    };

    const toggleEraser = () => {
        setIsEraserActive(!isEraserActive);
        if (!isEraserActive) {
            // set shape to be pencil which is null and color to be white
            setSelectedShape(null);
            setColor("#ffffff");
        }
    };

    return (
        <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
            <header style={{ padding: "1.5rem", textAlign: "center", background: "linear-gradient(135deg, #667eea 0%, #764ba2 100%)", color: "white", fontSize: "2rem", fontWeight: "600", fontFamily: "'Inter', sans-serif", boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1)" }}>
                Collaborative Whiteboard
            </header>

            <div style={{ display: "flex", justifyContent: "center", alignItems: "center", gap: "1.5rem", padding: "1rem", background: "white", boxShadow: "0 1px 3px rgba(0, 0, 0, 0.1)", marginBottom: "1rem" }}>
                <ShapeSelector selectedShape={selectedShape} setSelectedShape={setSelectedShape} isEraserActive={isEraserActive} />
                <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                    <input
                        type="color"
                        value={color}
                        onChange={(e) => {
                            // if ther eraser is active and user selects color other than white then disable the eraser
                            if (isEraserActive && e.target.value !== "#ffffff") {
                                setIsEraserActive(false);
                            }
                            setColor(e.target.value);
                        }}
                        style={{
                            width: "52px",
                            height: "52px",
                        }}
                    />
                </div>
                <BrushSizeSelector brushSize={brushSize} setBrushSize={setBrushSize} />

                <GrClear
                    onClick={() => setShowClearModal(true)}
                    style={{ fontSize: '1.75rem', cursor: 'pointer', transition: 'all 0.2s', '&:hover': { background: '#e2e8f0' } }}
                    title="Clear Board"
                />
                <MdDelete
                    onClick={() => setShowDeleteModal(true)}
                    style={{ fontSize: '2rem', cursor: 'pointer', color: '#ef4444', transition: 'all 0.2s', '&:hover': { background: '#fee2e2' } }}
                    title="Delete Room"
                />
                <button
                    className={`tool-button ${isEraserActive ? 'active' : ''}`}
                    title="Erase"
                    onClick={toggleEraser}
                    style={{ background: isEraserActive ? '#e0e7ff' : 'transparent', border: isEraserActive ? '2px solid #4f46e5' : '1px solid #e5e7eb', padding: '0.5rem', borderRadius: '0.375rem', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', transition: 'all 0.2s ease-in-out' }}
                >
                    <BsEraserFill
                        style={{ color: isEraserActive ? '#4f46e5' : '#6b7280', fontSize: '1.25rem' }}
                    />
                </button>
            </div>

            <div style={{ flexGrow: 1, position: "relative" }}>
                <canvas
                    ref={canvasRef}
                    onMouseDown={startDrawing}
                    onMouseUp={endDrawing}
                    onMouseOut={endDrawing}
                    onMouseMove={draw}
                    style={{ display: "block", width: "100%", height: "100%", cursor: isEraserActive ? `url('/eraser.png'), auto` : "crosshair" }}
                />
                <LiveCursors socket={socket} roomId={roomId} />
                <UsersList socket={socket} roomId={roomId} />
            </div>

            {showDeleteModal && <Modal handleClick={handleDeleteRoom} setShowModal={setShowDeleteModal} name={"Delete"} message={"Are you sure you want to delete this room? This action cannot be undone."} />}
            {showClearModal && <Modal handleClick={handleClearBoard} setShowModal={setShowClearModal} name={"Clear"} message={"Are you sure you want to clear the canvas? This action cannot be undo."} />}
            <CopyUrl />
        </div>
    );
}

export default Whiteboard;
