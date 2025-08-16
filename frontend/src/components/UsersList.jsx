import React, { useState, useEffect, useRef } from "react";
import { useAuth } from "../context/AuthContext";

function UsersList({ socket, roomId }) {
    const [open, setOpen] = useState(false);
    const [toasts, setToasts] = useState([]);
    const { users, setUsers, setAdminId, isAdmin, setIsAdmin } = useAuth();
    const uniqueUsers = Array.from(new Map(users.map(u => [u.userId, u])).values());

    // Track recent toast messages to prevent duplicates
    const recentToastMessages = useRef(new Set());

    const toggleDrawer = () => {
        setOpen(!open);
    };

    const addToast = (message) => {
        if (recentToastMessages.current.has(message)) return; // ignore duplicate toast

        recentToastMessages.current.add(message);
        const id = Date.now();
        setToasts(prev => [...prev, { id, message }]);

        // Auto remove toast after 3 seconds and remove from recent set
        setTimeout(() => {
            setToasts(prev => prev.filter(toast => toast.id !== id));
            recentToastMessages.current.delete(message);
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

    const getInitial = (name) =>
        name?.trim() ? name.charAt(0).toUpperCase() : "?";

    const getMutedColor = (name) => {
        const colors = [
            "#CBD5E1", // slate-300
            "#E2E8F0", // gray-200
            "#D1D5DB", // gray-300
            "#F3F4F6", // gray-100
            "#E5E7EB", // gray-200
        ];
        let sum = 0;
        for (let i = 0; i < name.length; i++) sum += name.charCodeAt(i);
        return colors[sum % colors.length];
    };

    return (
        <>
            {/* Toasts for sidebar */}
            <div className="absolute top-4 left-4 right-4 z-10">
                {toasts.map((toast) => (
                    <div
                        key={toast.id}
                        className="mb-2 px-3 py-2 rounded-md shadow-sm bg-green-50 border border-green-200 text-green-800 text-sm animate-slide-down"
                    >
                        {toast.message}
                    </div>
                ))}
            </div>

            <div className="h-full flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between p-4 border-b border-gray-200 bg-gray-50">
                    <h3 className="text-lg font-semibold text-gray-800">
                        Online Users
                    </h3>
                    <span className="px-2 py-1 bg-indigo-100 text-indigo-600 rounded-full text-sm font-medium">
                        {uniqueUsers.length}
                    </span>
                </div>

                {/* Users List */}
                <div className="flex-1 overflow-y-auto">
                    {uniqueUsers.length === 0 ? (
                        <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                            <div className="w-12 h-12 bg-gray-200 rounded-full flex items-center justify-center mb-2">
                                <span className="text-gray-400 text-xl">👥</span>
                            </div>
                            <p className="text-sm">No users online</p>
                        </div>
                    ) : (
                        <div className="p-2">
                            {uniqueUsers.map((user) => {
                                const isUserAdmin = isAdmin && user.userId === user.userId;
                                return (
                                    <div
                                        key={user.userId}
                                        className={`flex items-center gap-3 p-3 rounded-lg mb-2 transition-colors ${isUserAdmin
                                                ? "bg-indigo-50 border border-indigo-200"
                                                : "bg-white hover:bg-gray-50 border border-gray-100"
                                            }`}
                                    >
                                        {/* Avatar */}
                                        <div
                                            className={`w-10 h-10 flex items-center justify-center rounded-full font-semibold text-gray-700 text-sm ${isUserAdmin ? "ring-2 ring-indigo-300" : ""
                                                }`}
                                            style={{
                                                backgroundColor: getMutedColor(user.username || ""),
                                            }}
                                        >
                                            {getInitial(user.username)}
                                        </div>

                                        {/* User Info */}
                                        <div className="flex-1 min-w-0">
                                            <p className={`text-sm font-medium truncate ${isUserAdmin ? "text-indigo-700" : "text-gray-900"
                                                }`}>
                                                {user.username || "Anonymous"}
                                            </p>
                                            <p className="text-xs text-gray-500">
                                                {isUserAdmin ? "Administrator" : "Member"}
                                            </p>
                                        </div>

                                        {/* Status */}
                                        <div className="flex items-center gap-1">
                                            <div className="w-2 h-2 bg-green-400 rounded-full"></div>
                                            {isUserAdmin && (
                                                <span className="text-xs bg-indigo-100 text-indigo-700 px-2 py-0.5 rounded-full font-medium">
                                                    Admin
                                                </span>
                                            )}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </div>

            <style>{`
                    @keyframes slide-down {
                        from { opacity: 0; transform: translateY(-8px); }
                        to { opacity: 1; transform: translateY(0); }
                    }
                    .animate-slide-down {
                        animation: slide-down 0.3s ease-out;
                    }
                `}</style>
        </>
    );
}

export default UsersList;
