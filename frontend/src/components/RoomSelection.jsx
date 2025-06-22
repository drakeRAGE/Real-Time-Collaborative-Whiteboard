import React, { useState } from 'react';
import { CheckOline } from '../utils/networkUtils';
import { useNavigate } from 'react-router-dom';
import { v4 as uuidv4 } from 'uuid';

function RoomSelection() {
    const [roomId, setRoomId] = useState('');
    const navigate = useNavigate();

    const handleJoinRoom = (e) => {
        e.preventDefault();

        // Check if user is online
        if (!CheckOline()) return;

        // Proceed only if roomId is not empty
        if (roomId.trim()) {
            navigate(`/canva/${roomId}`);
        }
    };

    const handleCreateRoom = () => {
        // Check if user is online
        if (!CheckOline()) return;

        const newRoomId = uuidv4();
        navigate(`/canva/${newRoomId}`);
    };

    return (
        <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100vh',
            gap: '2rem'
        }}>
            <h1 style={{ fontSize: '2.5rem', fontFamily: 'serif', color: '#9866ce' }}>
                Collaborative Whiteboard
            </h1>
            <form onSubmit={handleJoinRoom} style={{ display: 'flex', gap: '1rem' }}>
                <input
                    type="text"
                    value={roomId}
                    onChange={(e) => setRoomId(e.target.value)}
                    placeholder="Enter Room ID"
                    style={{
                        padding: '0.5rem',
                        fontSize: '1rem',
                        borderRadius: '4px',
                        border: '1px solid #9866ce'
                    }}
                />
                <button
                    type="submit"
                    style={{
                        padding: '0.5rem 1rem',
                        background: '#9866ce',
                        color: 'white',
                        border: 'none',
                        borderRadius: '4px',
                        cursor: 'pointer'
                    }}
                >
                    Join Room
                </button>
            </form>
            <p style={{ fontSize: '1.2rem' }}>or</p>
            <button
                onClick={handleCreateRoom}
                style={{
                    padding: '0.5rem 1rem',
                    background: '#f472b6',
                    color: 'white',
                    border: 'none',
                    borderRadius: '4px',
                    cursor: 'pointer'
                }}
            >
                Create New Room
            </button>
        </div>
    );
}

export default RoomSelection;