import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";

export default function Navbar() {
    const { userId, email, logout } = useAuth();
    const navigate = useNavigate();

    const handleLogout = () => {
        logout();
        navigate("/auth");
    };

    const getUserName = (email) => {
        if (!email) return "";
        const namePart = email.split("@")[0];
        return namePart.charAt(0).toUpperCase() + namePart.slice(1);
    };

    return (
        <nav className="w-full h-[8vh] border-b border-gray-200 px-6 flex items-center justify-between bg-gray-100 shadow-sm">
            {/* Left: Logo / Brand */}
            <div className="flex items-center gap-2 cursor-pointer" onClick={() => navigate("/")}>
                <div className="w-8 h-8 bg-indigo-500 rounded-md flex items-center justify-center font-bold text-white text-lg">
                    W
                </div>
                <span className="font-semibold text-gray-800 text-lg">Whiteboard</span>
            </div>

            {/* Middle: Links */}
            <div className="hidden md:flex items-center gap-6">
                <Link
                    to="/room"
                    className="text-gray-600 hover:text-indigo-500 uppercase transition font-medium text-sm"
                >
                    Room
                </Link>
                <Link
                    to="/auth"
                    className="text-gray-600 hover:text-indigo-500 uppercase transition font-medium text-sm"
                >
                    Login
                </Link>
            </div>

            {/* Right: Auth Button */}
            <div className="flex items-center gap-4">
                {userId ? (
                    <>
                        <div className="flex items-center gap-2 px-3 py-1 rounded-full ">
                            <span className="w-7 h-7 rounded-full bg-indigo-500 text-white flex items-center justify-center text-sm font-medium">
                                {getUserName(email).charAt(0).toUpperCase()}
                            </span>
                            <span className="text-gray-700 text-sm font-medium">
                                {getUserName(email)}
                            </span>
                        </div>
                        <button
                            onClick={handleLogout}
                            className="px-4 py-1.5 rounded-md border border-gray-300 text-gray-700 text-sm font-medium hover:bg-gray-100 transition"
                        >
                            Logout
                        </button>
                    </>
                ) : (
                    <button
                        onClick={() => navigate("/auth")}
                        className="px-4 py-1.5 rounded-md bg-indigo-500 text-white text-sm font-medium hover:bg-indigo-600 transition"
                    >
                        Login
                    </button>
                )}
            </div>
        </nav>
    );
}
