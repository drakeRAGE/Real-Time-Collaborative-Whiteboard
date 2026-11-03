import { Navigate, Route, BrowserRouter as Router, Routes, useLocation } from 'react-router-dom';
import { ErrorBoundary } from 'react-error-boundary';
import { useParams } from 'react-router-dom';
import { Auth } from './components/Auth';
import ConnectionBanner from './components/ConnectionBanner';
import Navbar from './components/Navbar';
import RoomSelection from './components/RoomSelection';
import RouteFallback from './components/RouteFallback';
import Whiteboard from './components/Whiteboard';
import { useAuth } from './context/AuthContext';
import { RoomProvider } from './context/RoomProvider';
import { SocketProvider } from './context/SocketProvider';
import { ToastProvider } from './context/ToastContext';
import OfflineAssistance from './network/OfflineAssistance';
import type { ReactNode } from 'react';

function RouteBoundary({ children }: { children: ReactNode }) {
  const location = useLocation();
  return (
    <ErrorBoundary FallbackComponent={RouteFallback} resetKeys={[location.pathname]}>
      {children}
    </ErrorBoundary>
  );
}

function Protected({ children }: { children: ReactNode }) {
  const { session } = useAuth();
  if (!session) return <Navigate to="/auth" replace />;
  return children;
}

function RoomRoute() {
  const { roomId = '' } = useParams();
  return (
    <RoomProvider roomId={roomId}>
      <Whiteboard />
    </RoomProvider>
  );
}

export default function App() {
  const { session, loading } = useAuth();
  if (loading) return <div className="p-6 text-gray-700">Loading...</div>;

  return (
    <SocketProvider>
      <ToastProvider>
        <Router>
          <ConnectionBanner />
          <Navbar />
          <Routes>
            <Route path="/auth" element={<RouteBoundary><Auth /></RouteBoundary>} />
            <Route path="/" element={<RouteBoundary><Protected><RoomSelection /></Protected></RouteBoundary>} />
            <Route path="/canva/:roomId" element={<RouteBoundary><Protected><RoomRoute /></Protected></RouteBoundary>} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          {session && <OfflineAssistance />}
        </Router>
      </ToastProvider>
    </SocketProvider>
  );
}
