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