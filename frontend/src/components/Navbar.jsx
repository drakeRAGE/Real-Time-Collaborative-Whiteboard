import React from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";

export default function Navbar() {
    const { userId, email, logout } = useAuth();
    const navigate = useNavigate();

    const handleLogout = () => {
        logout();
        navigate("/auth");
    };

    // Extract clean name from email
    const getUserName = (email) => {
        if (!email) return "";
        const namePart = email.split("@")[0];
        return namePart.replace(/\.(com|net|org|xyz|io|co)$/i, ""); // strip common extensions if present
    };

    return (
        <nav className="w-full bg-white border-b border-gray-200 shadow-sm px-6 py-3 flex items-center justify-between">
            {/* Left: Logo / Brand */}
            <div className="flex items-center gap-2">
                <div className="w-8 h-8 bg-gray-200 rounded-md flex items-center justify-center font-bold text-gray-700">
                    W
                </div>
                <span className="font-medium text-gray-700">Whiteboard</span>
            </div>

            {/* Middle: Links */}
            <div className="hidden md:flex items-center gap-6">
                <Link
                    to="/room"
                    className="text-gray-600 hover:text-gray-900 transition text-sm"
                >
                    Room
                </Link>
                <Link
                    to="/auth"
                    className="text-gray-600 hover:text-gray-900 transition text-sm"
                >
                    Auth
                </Link>
            </div>

            {/* Right: Auth Button */}
            <div className="flex items-center gap-4">
                {userId ? (
                    <>
                        <span className="text-gray-600 text-sm">
                            👋 Hi, <span className="font-medium">{getUserName(email)}</span>
                        </span>
                        <button
                            onClick={handleLogout}
                            className="px-3 py-1.5 rounded-md bg-gray-100 border border-gray-300 text-gray-700 text-sm hover:bg-gray-200 transition"
                        >
                            Logout
                        </button>
                    </>
                ) : (
                    <button
                        onClick={() => navigate("/auth")}
                        className="px-3 py-1.5 rounded-md bg-indigo-500 text-white text-sm hover:bg-indigo-600 transition"
                    >
                        Login
                    </button>
                )}
            </div>
        </nav>
    );
}
