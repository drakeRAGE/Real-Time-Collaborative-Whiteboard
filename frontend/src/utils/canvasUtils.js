export function clearCanvas(canvasRef, color = '#ffffff') {
    const canvas = canvasRef.current;
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    return canvas;
}

export const deleteRoom = async (socket, roomId) => {
    if (!socket || !roomId) return false;
    
    try {
        const response = await fetch(`http://localhost:5000/api/rooms/${roomId}`, {
            method: 'DELETE'
        });
        
        if (!response.ok) {
            throw new Error('Failed to delete room');
        }
        
        return true;
    } catch (error) {
        console.error('Error deleting room:', error);
        return false;
    }
};

export const startDrawingUtils = (nativeEvent, setPosition, setIsDrawing, selectedShape, setStartPos) => {
    const { offsetX, offsetY } = nativeEvent;
    setPosition({ x: offsetX, y: offsetY });
    setIsDrawing(true);
    if (selectedShape) {
        setStartPos({ x: offsetX, y: offsetY });
    }
};

export const endDrawingUtils = (nativeEvent, isDrawing, position, color, selectedShape, drawShape, setStartPos, setIsDrawing, drawLine) => {
    if (!isDrawing) return;

    const { offsetX, offsetY } = nativeEvent;
    if (selectedShape) {
        drawShape({ x: offsetX, y: offsetY });
        setStartPos(null);
    } else {
        drawLine(position.x, position.y, offsetX, offsetY, true, color);
    }

    setIsDrawing(false);
};