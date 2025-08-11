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
        <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-900 via-gray-800 to-black p-4">
            <div className="bg-white/10 backdrop-blur-lg border border-white/20 rounded-2xl shadow-xl w-full max-w-md p-8">
                <h1 className="text-3xl font-bold text-white text-center mb-6">
                    Collaborative Whiteboard
                </h1>
                <p className="text-gray-300 text-center mb-8">
                    Join an existing room or create a new one to start collaborating.
                </p>

                {/* Join Room Form */}
                <form onSubmit={handleJoinRoom} className="space-y-5">
                    <div>
                        <label className="block text-gray-200 text-sm mb-1">Room ID</label>
                        <input
                            type="text"
                            value={roomId}
                            onChange={(e) => setRoomId(e.target.value)}
                            placeholder="Enter Room ID"
                            className="w-full px-4 py-2 rounded-lg bg-gray-900/60 text-white placeholder-gray-400 border border-gray-700 focus:ring-2 focus:ring-indigo-500 outline-none"
                        />
                    </div>
                    <button
                        type="submit"
                        className="w-full bg-indigo-600 hover:bg-indigo-700 text-white font-semibold py-2 rounded-lg transition-all duration-300"
                    >
                        Join Room
                    </button>
                </form>

                {/* Divider */}
                <div className="my-6 flex items-center">
                    <div className="flex-1 h-px bg-gray-700"></div>
                    <span className="px-4 text-gray-400 text-sm">OR</span>
                    <div className="flex-1 h-px bg-gray-700"></div>
                </div>

                {/* Create Room Button */}
                <button
                    onClick={handleCreateRoom}
                    className="w-full bg-pink-500 hover:bg-pink-600 text-white font-semibold py-2 rounded-lg transition-all duration-300"
                >
                    Create New Room
                </button>
            </div>
        </div>
    );
}

export default RoomSelection;
