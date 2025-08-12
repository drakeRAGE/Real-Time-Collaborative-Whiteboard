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
import { useAuth } from "../context/AuthContext";
import { FaRedo, FaUndo } from "react-icons/fa";
import ChatMessage from "./ChatMessage";

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
    const [undo, setUndo] = useState(false);
    const { userId,
        email,
        users,
        adminId,
        setUsers,
        setAdminId,
        isAdmin,
        setIsAdmin } = useAuth();
    console.log(email, users, adminId, isAdmin);

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

        const handleKeyDown = (e) => {
            if (!socket || !roomId) return;

            if ((e.ctrlKey || e.metaKey) && e.key === 'z') {
                e.preventDefault();
                socket.emit('undo', roomId);
            } else if ((e.ctrlKey || e.metaKey) && e.key === 'y') {
                e.preventDefault();
                socket.emit('redo', roomId);
            }
        };

        window.addEventListener('keydown', handleKeyDown);

        return () => {
            socket.off("draw");
            socket.off("clear");
            socket.off("joinRoom");
            window.removeEventListener('keydown', handleKeyDown);
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

        const drawShapeHandler = (data) => {
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
        }
        socket.on('drawShape', drawShapeHandler);
        socket.on('undo', drawShapeHandler);

        const updateCanvasHandler = (drawings) => {
            const canvas = canvasRef.current;
            const ctx = ctxRef.current;
            if (!canvas || !ctx) return;

            ctx.clearRect(0, 0, canvas.width, canvas.height);
            drawings.forEach(drawing => {
                if (drawing.shape) {
                    drawShapeHandler(drawing);
                } else {
                    // It's a freehand line, so use drawLine
                    drawLine(drawing.x0, drawing.y0, drawing.x1, drawing.y1, false, drawing.color, drawing.size);
                }
            });
        };

        socket.on('drawShape', drawShapeHandler);

        socket.on('undo', ({ drawings }) => {
            updateCanvasHandler(drawings);
        });

        socket.on('redo', ({ drawings }) => {
            updateCanvasHandler(drawings);
        });

        socket.on('updateCanvas', (drawings) => {
            updateCanvasHandler(drawings);
        });

        return () => {
            socket.off('drawShape');
            socket.off('undo');
            socket.off('redo');
            socket.off('updateCanvas');
        }
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

    // Extract clean name from email
    const getUserName = (email) => {
        if (!email) return "";
        const namePart = email.split("@")[0];
        return namePart.replace(/\.(com|net|org|xyz|io|co)$/i, ""); // strip common extensions if present
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
        <div className="h-screen flex flex-col text-white">
            {/* Header */}
            <header className="p-6 text-center bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500 shadow-md text-2xl font-bold font-inter tracking-wide">
                Collaborative Whiteboard
            </header>

            {/* Toolbar */}
            <div className="flex flex-wrap items-center justify-center gap-4 p-4 shadow-inner">
                <ShapeSelector
                    selectedShape={selectedShape}
                    setSelectedShape={setSelectedShape}
                    isEraserActive={isEraserActive}
                />

                {/* Color Picker */}
                <div className="flex items-center">
                    <label className="relative">
                        <input
                            type="color"
                            value={color}
                            onChange={(e) => {
                                if (isEraserActive && e.target.value !== "#ffffff") {
                                    setIsEraserActive(false);
                                }
                                setColor(e.target.value);
                            }}
                            className="absolute inset-0 opacity-0 cursor-pointer"
                        />
                        <div
                            className="w-12 h-12 rounded-full border border-gray-300 shadow-md transition-transform duration-200 hover:scale-105 hover:shadow-lg"
                            style={{ backgroundColor: color }}
                            title="Choose a color"
                        />
                    </label>
                </div>


                <BrushSizeSelector brushSize={brushSize} setBrushSize={setBrushSize} />

                {/* Clear Board */}
                <GrClear
                    onClick={() => setShowClearModal(true)}
                    className="text-2xl cursor-pointer text-gray-300 hover:text-indigo-400 transition"
                    title="Clear Board"
                />

                {/* Delete Room */}
                <MdDelete
                    onClick={() => setShowDeleteModal(true)}
                    className="text-3xl cursor-pointer text-red-500 hover:text-red-400 transition"
                    title="Delete Room"
                />

                {/* Eraser */}
                <button
                    className={`p-2 rounded-lg border transition flex items-center justify-center ${isEraserActive
                            ? "bg-indigo-100 border-indigo-500"
                            : "bg-transparent border-gray-600"
                        }`}
                    title="Erase"
                    onClick={toggleEraser}
                >
                    <BsEraserFill
                        className={`text-lg ${isEraserActive ? "text-indigo-600" : "text-gray-400"
                            }`}
                    />
                </button>

                {/* Undo */}
                <button
                    onClick={() => socket.emit("undo", roomId)}
                    className={`flex items-center justify-center gap-2 px-4 py-2 rounded-full font-medium transition-all duration-200 bg-blue-500 hover:bg-blue-600 text-white shadow-sm hover:shadow-md hover:scale-105`}
                >
                    <FaUndo className="text-lg" />
                    <span className="hidden sm:inline">Undo</span>
                </button>

                {/* Redo */}
                <button
                    onClick={() => socket.emit("redo", roomId)}
                    className={`flex items-center justify-center gap-2 px-4 py-2 rounded-full font-medium transition-all duration-200 bg-green-500 hover:bg-green-600 text-white shadow-sm hover:shadow-md hover:scale-105`}
                >
                    <FaRedo className="text-lg" />
                    <span className="hidden sm:inline">Redo</span>
                </button>
            </div>

            {/* Canvas Area */}
            <div className="flex-grow relative">
                <canvas
                    ref={canvasRef}
                    onMouseDown={startDrawing}
                    onMouseUp={endDrawing}
                    onMouseOut={endDrawing}
                    onMouseMove={draw}
                    className={`block w-full h-full ${isEraserActive
                            ? "cursor-[url('/eraser.png'),_auto]"
                            : "cursor-crosshair"
                        }`}
                />
                <LiveCursors socket={socket} roomId={roomId} />
                <UsersList socket={socket} roomId={roomId} />
            </div>

            <div style={{ height: 'auto', width: 'auto' }}>
                <ChatMessage
                    socket={socket}
                    roomId={roomId}
                    userId={userId}
                    username={getUserName(email)}
                />
            </div>

            {/* Modals */}
            {showDeleteModal && (
                <Modal
                    handleClick={handleDeleteRoom}
                    setShowModal={setShowDeleteModal}
                    name={"Delete"}
                    message={
                        "Are you sure you want to delete this room? This action cannot be undone."
                    }
                />
            )}
            {showClearModal && (
                <Modal
                    handleClick={handleClearBoard}
                    setShowModal={setShowClearModal}
                    name={"Clear"}
                    message={
                        "Are you sure you want to clear the canvas? This action cannot be undone."
                    }
                />
            )}
            <CopyUrl />
        </div>
    );
}

export default Whiteboard;
