import { useEffect, useState } from 'react';

function LiveCursors({ socket, roomId }) {
    const [cursors, setCursors] = useState({});
    const [mySocketId, setMySocketId] = useState(null);

    useEffect(() => {
        if (!socket) return;

        // capture the client's socket id (may be undefined briefly)
        setMySocketId(socket.id);

        const handleCursorMove = ({ userId, x, y, username, color, socketId }) => {
            // Ignore cursor events originating from this client's socket
            if (socketId && socketId === socket.id) return;

            // If coordinates are invalid, ignore
            if (typeof x !== 'number' || typeof y !== 'number') return;

            setCursors(prev => ({
                ...prev,
                [userId]: { x, y, username, color }
            }));
        };

        const handleUserJoined = ({ userId, username, users, adminId }) => {
            // update username/color if we already have a cursor for that user
            if (users && Array.isArray(users)) {
                // apply colors/usernames from users list
                setCursors(prev => {
                    const next = { ...prev };
                    for (const u of users) {
                        if (next[u.userId]) {
                            next[u.userId] = { ...next[u.userId], username: u.username, color: u.color };
                        }
                    }
                    return next;
                });
            } else {
                setCursors(prev => ({
                    ...prev,
                    [userId]: { ...prev[userId], username }
                }));
            }
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

            // emit to server; server will attach userId & socketId & color when rebroadcasting
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
            {Object.entries(cursors).map(([userId, { x, y, username, color = '#9866ce' }]) => {
                // If for any reason we somehow got our own userId mapped (double check via mySocketId),
                // we can still filter by comparing userId->none. Primary filter is via socketId check on events.
                return (
                    <div
                        key={userId}
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
                            backgroundColor: color,
                            border: '2px solid white',
                            boxShadow: '0 0 4px rgba(0,0,0,0.2)'
                        }} />
                        {username && (
                            <div style={{
                                marginTop: '4px',
                                padding: '2px 6px',
                                backgroundColor: color ? `${color}CC` : 'rgba(152,102,206,0.8)',
                                color: 'white',
                                borderRadius: '12px',
                                fontSize: '12px',
                                fontFamily: 'serif',
                                whiteSpace: 'nowrap'
                            }}>
                                {username}
                            </div>
                        )}
                    </div>
                );
            })}
        </div>
    );
}

export default LiveCursors;