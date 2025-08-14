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
        const response = await fetch(`${import.meta.env.VITE_BACKEND_URL}/api/rooms/${roomId}`, {
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

export function drawRectangle(ctx, startPos, endPos) {
    ctx.rect(startPos.x, startPos.y, endPos.x - startPos.x, endPos.y - startPos.y);
}

export function drawCircle(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const radiusX = Math.abs(endPos.x - startPos.x) / 2;
    const radiusY = Math.abs(endPos.y - startPos.y) / 2;
    ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, 2 * Math.PI);
}

export function drawSquare(ctx, startPos, endPos) {
    const sideLength = Math.max(Math.abs(endPos.x - startPos.x), Math.abs(endPos.y - startPos.y));
    const signX = endPos.x > startPos.x ? 1 : -1;
    const signY = endPos.y > startPos.y ? 1 : -1;
    ctx.rect(startPos.x, startPos.y, signX * sideLength, signY * sideLength);
}

export function drawTriangle(ctx, startPos, endPos) {
    ctx.moveTo(startPos.x, endPos.y);
    ctx.lineTo(endPos.x, endPos.y);
    ctx.lineTo((startPos.x + endPos.x) / 2, startPos.y);
    ctx.closePath();
}

export function drawStar(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const outerRadius = Math.min(Math.abs(endPos.x - startPos.x), Math.abs(endPos.y - startPos.y)) / 2;
    const innerRadius = outerRadius / 2;

    ctx.moveTo(centerX + outerRadius * Math.cos(0), centerY + outerRadius * Math.sin(0));
    for (let i = 0; i < 10; i++) {
        const angle = i * Math.PI / 5;
        const radius = i % 2 === 0 ? outerRadius : innerRadius;
        ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
    }
    ctx.closePath();
}

export function drawPentagon(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const radius = Math.min(Math.abs(endPos.x - startPos.x), Math.abs(endPos.y - startPos.y)) / 2;

    ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
    for (let i = 1; i <= 5; i++) {
        const angle = i * 2 * Math.PI / 5;
        ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
    }
    ctx.closePath();
}

export function drawHexagon(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const radius = Math.min(Math.abs(endPos.x - startPos.x), Math.abs(endPos.y - startPos.y)) / 2;

    ctx.moveTo(centerX + radius * Math.cos(0), centerY + radius * Math.sin(0));
    for (let i = 1; i <= 6; i++) {
        const angle = i * 2 * Math.PI / 6;
        ctx.lineTo(centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle));
    }
    ctx.closePath();
}

export function drawArrowUp(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const width = Math.abs(endPos.x - startPos.x);
    const height = Math.abs(endPos.y - startPos.y);
    
    ctx.moveTo(centerX, startPos.y);
    ctx.lineTo(startPos.x, centerY);
    ctx.lineTo(centerX - width * 0.1, centerY);
    ctx.lineTo(centerX - width * 0.1, endPos.y);
    ctx.lineTo(centerX + width * 0.1, endPos.y);
    ctx.lineTo(centerX + width * 0.1, centerY);
    ctx.lineTo(endPos.x, centerY);
    ctx.closePath();
}

export function drawArrowDown(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const width = Math.abs(endPos.x - startPos.x);
    const height = Math.abs(endPos.y - startPos.y);
    
    ctx.moveTo(centerX, endPos.y);
    ctx.lineTo(startPos.x, centerY);
    ctx.lineTo(centerX - width * 0.1, centerY);
    ctx.lineTo(centerX - width * 0.1, startPos.y);
    ctx.lineTo(centerX + width * 0.1, startPos.y);
    ctx.lineTo(centerX + width * 0.1, centerY);
    ctx.lineTo(endPos.x, centerY);
    ctx.closePath();
}

export function drawArrowLeft(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const width = Math.abs(endPos.x - startPos.x);
    const height = Math.abs(endPos.y - startPos.y);
    
    ctx.moveTo(startPos.x, centerY);
    ctx.lineTo(centerX, startPos.y);
    ctx.lineTo(centerX, centerY - height * 0.1);
    ctx.lineTo(endPos.x, centerY - height * 0.1);
    ctx.lineTo(endPos.x, centerY + height * 0.1);
    ctx.lineTo(centerX, centerY + height * 0.1);
    ctx.lineTo(centerX, endPos.y);
    ctx.closePath();
}

export function drawArrowRight(ctx, startPos, endPos) {
    const centerX = (startPos.x + endPos.x) / 2;
    const centerY = (startPos.y + endPos.y) / 2;
    const width = Math.abs(endPos.x - startPos.x);
    const height = Math.abs(endPos.y - startPos.y);
    
    ctx.moveTo(endPos.x, centerY);
    ctx.lineTo(centerX, startPos.y);
    ctx.lineTo(centerX, centerY - height * 0.1);
    ctx.lineTo(startPos.x, centerY - height * 0.1);
    ctx.lineTo(startPos.x, centerY + height * 0.1);
    ctx.lineTo(centerX, centerY + height * 0.1);
    ctx.lineTo(centerX, endPos.y);
    ctx.closePath();
}