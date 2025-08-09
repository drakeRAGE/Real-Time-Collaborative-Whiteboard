import { useEffect, useState } from 'react';

function LiveCursors({ socket, roomId }) {
    const [cursors, setCursors] = useState({});
    const [currentUserId, setCurrentUserId] = useState(null);

    useEffect(() => {
        if (!socket) return;

        // Store current user's socket ID
        setCurrentUserId(socket.id);

        const handleCursorMove = ({ userId, x, y, username }) => {
            // Skip if this is the current user's cursor
            if (userId === socket.id) return;
            
            setCursors(prev => ({
                ...prev,
                [userId]: { x, y, username }
            }));
        };

        const handleUserJoined = ({ userId, username }) => {
            setCursors(prev => ({
                ...prev,
                [userId]: { ...prev[userId], username }
            }));
        };

        const handleUserLeft = ({ userId }) => {
            setCursors(prev => {
                const newCursors = { ...prev };
                delete newCursors[userId];
                return newCursors;
            });
        };

        socket.on('cursorMove', handleCursorMove);
        socket.on('userJoined', handleUserJoined);
        socket.on('userLeft', handleUserLeft);

        // Send cursor position updates
        const sendCursorPosition = (e) => {
            if (!socket || !roomId) return;

            const canvas = document.querySelector('canvas');
            if (!canvas) return;

            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;

            socket.emit('cursorMove', {
                roomId,
                x,
                y
            });
        };

        window.addEventListener('mousemove', sendCursorPosition);

        return () => {
            socket.off('cursorMove', handleCursorMove);
            socket.off('userJoined', handleUserJoined);
            socket.off('userLeft', handleUserLeft);
            window.removeEventListener('mousemove', sendCursorPosition);
        };
    }, [socket, roomId]);

    return (
        <div style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: '100%',
            height: '100%',
            pointerEvents: 'none',
            zIndex: 100
        }}>
            {Object.entries(cursors)
                .filter(([userId]) => userId !== currentUserId) // Filter out current user
                .map(([userId, { x, y, username }]) => (
                <div 
                    key={JSON.stringify(userId)}
                    style={{
                        position: 'absolute',
                        left: `${x}px`,
                        top: `${y}px`,
                        transform: 'translate(-50%, -50%)',
                        pointerEvents: 'none',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center'
                    }}
                >
                    <div style={{
                        width: '12px',
                        height: '12px',
                        borderRadius: '50%',
                        backgroundColor: '#9866ce',
                        border: '2px solid white',
                        boxShadow: '0 0 4px rgba(0,0,0,0.2)'
                    }} />
                    {username && (
                        <div style={{
                            marginTop: '4px',
                            padding: '2px 6px',
                            backgroundColor: 'rgba(152, 102, 206, 0.8)',
                            color: 'white',
                            borderRadius: '12px',
                            fontSize: '12px',
                            fontFamily: 'serif',
                            whiteSpace: 'nowrap'
                        }}>
                            {username} 
                            {/* {JSON.stringify(userId)} */}
                        </div>
                    )}
                </div>
            ))}
        </div>
    );
}

export default LiveCursors;