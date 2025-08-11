import React, { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';

function UsersList({ socket, roomId }) {
    const [open, setOpen] = useState(false);
    const [toasts, setToasts] = useState([]);
    const { users, setUsers, setAdminId, isAdmin, setIsAdmin } = useAuth();

    const toggleDrawer = () => {
        setOpen(!open);
    };

    const addToast = (message) => {
        const id = Date.now();
        setToasts(prev => [...prev, { id, message }]);

        // Auto remove toast after 3 seconds
        setTimeout(() => {
            setToasts(prev => prev.filter(toast => toast.id !== id));
        }, 3000);
    };

    useEffect(() => {
        if (!socket) return;

        const handleUserJoined = ({ userId, users: updatedUsers, username, adminId }) => {
            setUsers(updatedUsers);
            setAdminId(adminId);
            setIsAdmin(userId === adminId);
            addToast(`${username} joined the room`);
        };

        const handleUserLeft = ({ userId, users: updatedUsers, username, adminId }) => {
            setUsers(updatedUsers);
            setAdminId(adminId);
            setIsAdmin(userId === adminId);
            addToast(`${username} left the room`);
        };

        socket.on('userJoined', handleUserJoined);
        socket.on('userLeft', handleUserLeft);

        return () => {
            socket.off('userJoined', handleUserJoined);
            socket.off('userLeft', handleUserLeft);
        };
    }, [socket]);

    

    return (
        <>
            {/* Toast container */}
            <div style={{
                position: 'fixed',
                top: '20px',
                right: '20px',
                zIndex: 1300,
                display: 'flex',
                flexDirection: 'column',
                gap: '10px'
            }}>
                {toasts.map(toast => (
                    <div key={toast.id} style={{
                        padding: '12px 16px',
                        background: '#9866ce',
                        color: 'white',
                        borderRadius: '4px',
                        boxShadow: '0 2px 10px rgba(0,0,0,0.1)',
                        animation: 'fadeIn 0.3s ease'
                    }}>
                        {toast.message}
                        {isAdmin && <h4>Welocome Admin!!</h4>}
                    </div>
                ))}
            </div>

            {/* Existing toggle button and user list */}
            <button
                onClick={toggleDrawer}
                style={{
                    position: 'fixed',
                    right: '16px',
                    top: '16px',
                    zIndex: 1200,
                    backgroundColor: '#9866ce',
                    color: 'white',
                    border: 'none',
                    borderRadius: '50%',
                    width: '40px',
                    height: '40px',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center'
                }}
            >
                <svg
                    xmlns="http://www.w3.org/2000/svg"
                    width="24"
                    height="24"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                >
                    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path>
                    <circle cx="9" cy="7" r="4"></circle>
                    <path d="M23 21v-2a4 4 0 0 0-3-3.87"></path>
                    <path d="M16 3.13a4 4 0 0 1 0 7.75"></path>
                </svg>
            </button>

            <div
                style={{
                    position: 'fixed',
                    top: 0,
                    right: open ? '0' : '-240px',
                    width: '240px',
                    height: '100vh',
                    background: 'white',
                    boxShadow: '-2px 0 5px rgba(0,0,0,0.1)',
                    transition: 'right 0.3s ease',
                    zIndex: 1000
                }}
            >
                <div style={{
                    padding: '16px',
                    fontWeight: 'bold',
                    color: '#9866ce',
                    fontFamily: 'serif',
                    fontSize: '1.2rem',
                    borderBottom: '1px solid #eee'
                }}>
                    Users in Room ({users.length})
                </div>
                <div style={{ overflow: 'auto' }}>
                    {users.map((user, index) => (
                        <div
                            key={user.userId}
                            style={{
                                padding: '8px 16px',
                                fontFamily: 'serif',
                                color: '#555',
                                borderBottom: '1px solid #f3f3f3'
                            }}
                        >
                            User {index + 1}: {user.username || 'Anonymous'}
                        </div>
                    ))}
                </div>
            </div>

            {/* Add CSS animation for toasts */}
            <style>
                {`
                    @keyframes fadeIn {
                        from { opacity: 0; transform: translateY(-10px); }
                        to { opacity: 1; transform: translateY(0); }
                    }
                `}
            </style>
        </>
    );
}

export default UsersList;