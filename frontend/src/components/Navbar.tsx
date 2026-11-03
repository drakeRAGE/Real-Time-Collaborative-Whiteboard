import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { usernameFromEmail } from '../utils/username';

export default function Navbar() {
  const { userId, email, logout } = useAuth();
  const navigate = useNavigate();
  const name = usernameFromEmail(email);

  return (
    <nav className="flex h-[8vh] w-full items-center justify-between border-b border-gray-200 bg-gray-100 px-6 shadow-sm">
      <button type="button" className="flex items-center gap-2" onClick={() => navigate('/')}>
        <span className="flex h-8 w-8 items-center justify-center rounded-md bg-indigo-500 text-lg font-bold text-white">W</span>
        <span className="text-lg font-semibold text-gray-800">Whiteboard</span>
      </button>
      <div className="hidden items-center gap-6 md:flex">
        <Link to="/" className="text-sm font-medium text-gray-600 hover:text-indigo-500">Room</Link>
        <Link to="/auth" className="text-sm font-medium text-gray-600 hover:text-indigo-500">Login</Link>
      </div>
      <div className="flex items-center gap-4">
        {userId ? (
          <>
            <span className="text-sm font-medium text-gray-700">{name}</span>
            <button
              type="button"
              className="rounded-md border border-gray-300 px-4 py-1.5 text-sm text-gray-700"
              onClick={() => {
                void logout().then(() => navigate('/auth'));
              }}
            >
              Logout
            </button>
          </>
        ) : (
          <button type="button" className="rounded-md bg-indigo-500 px-4 py-1.5 text-sm text-white" onClick={() => navigate('/auth')}>
            Login
          </button>
        )}
      </div>
    </nav>
  );
}
