import React, { useState, useEffect } from "react";
import { useAuth } from "../context/AuthContext";
import { FaUsers } from "react-icons/fa";

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
            {/* Toasts */}
            <div className="fixed top-20 right-20 z-[9999] flex flex-col gap-2">
                {toasts.map((toast) => (
                    <div
                        key={toast.id}
                        className="px-3 py-2 rounded-md shadow-sm bg-white border border-gray-200 text-gray-800 text-sm animate-fade-in"
                    >
                        {toast.message}
                        {isAdmin && (
                            <span className="block text-xs text-gray-500 mt-1">
                                Welcome Admin
                            </span>
                        )}
                    </div>
                ))}
            </div>

            {/* Floating Toggle Button */}
            <button
                onClick={toggleDrawer}
                className="fixed top-19 right-5 z-[9998] w-11 h-11 flex items-center justify-center rounded-full bg-white border border-gray-300 text-gray-600 shadow-sm hover:bg-gray-50 transition"
            >
                <FaUsers className="text-lg" />
            </button>

            {/* Drawer */}
            <div
                className={`fixed top-15 right-0 h-full w-64 bg-white border-l border-gray-200 shadow-xl transform transition-transform duration-300 z-[9997] ${open ? "translate-x-0" : "translate-x-full"
                    }`}
            >
                <div className="p-7 font-medium text-gray-700 border-b border-gray-200">
                    Users ({users.length})
                </div>
                <div className="overflow-y-auto max-h-[calc(100%-48px)]">
                    {users.map((user) => {
                        const isUserAdmin = isAdmin;
                        return (
                            <div
                                key={user.userId}
                                className={`flex items-center gap-3 px-4 py-2 border-b border-gray-100 hover:bg-gray-50 transition relative ${isUserAdmin ? "bg-gray-50" : ""
                                    }`}
                            >
                                {/* Accent for admin */}
                                {isUserAdmin && (
                                    <div className="absolute left-0 top-0 bottom-0 w-1 bg-blue-400 rounded-tr-sm rounded-br-sm" />
                                )}

                                {/* Avatar */}
                                <div
                                    className={`w-9 h-9 flex items-center justify-center rounded-full font-semibold text-gray-700 ${isUserAdmin ? "ring-2 ring-blue-300" : ""
                                        }`}
                                    style={{
                                        backgroundColor: getMutedColor(user.username || "")
                                    }}
                                >
                                    {getInitial(user.username)}
                                </div>

                                {/* Username */}
                                <span
                                    className={`text-sm truncate ${isUserAdmin ? "text-gray-800 font-medium" : "text-gray-700"
                                        }`}
                                >
                                    {user.username || "Anonymous"}
                                </span>

                                {/* Admin Badge */}
                                {isUserAdmin && (
                                    <span className="ml-auto text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded-full border border-blue-200 font-medium">
                                        Admin
                                    </span>
                                )}
                            </div>
                        );
                    })}


                </div>
            </div>

            {/* Animations */}
            <style>{`
        @keyframes fade-in {
          from { opacity: 0; transform: translateY(-4px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .animate-fade-in {
          animation: fade-in 0.2s ease-out;
        }
      `}</style>
        </>
    );
}

export default UsersList;