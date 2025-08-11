import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import RoomSelection from './components/RoomSelection';
import Whiteboard from './components/Whiteboard';
import OfflineAssistance from './network/OfflineAssistance';
import { useEffect, useState } from 'react';
import { supabase } from './utils/supabase';
import { Auth } from './components/Auth';
import { initializeSocket } from './utils/socket';

function ProtectedRoute({ session, children }) {
  if (!session) {
    return <Navigate to="/auth" replace />;
  }
  return children;
}

function App() {
  const [session, setSession] = useState(null);
  const [socket, setSocket] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const getSession = async () => {
      const { data: { session } } = await supabase.auth.getSession();
      setSession(session);
      setLoading(false);
    };

    getSession();

    const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
      setSession(session);
      setLoading(false);
    });

    return () => subscription.unsubscribe();
  }, []);

  useEffect(() => {
    if (session) {
      initializeSocket()
        .then(setSocket)
        .catch(console.error);
    } else {
      if (socket) {
        socket.disconnect();
        setSocket(null);
      }
    }
  }, [session]);

  if (loading) {
    return <div>Loading...</div>;
  }

  return (
    <Router>
      <Routes>
        {/* Public Route */}
        <Route path="/auth" element={<Auth />} />

        {/* Protected Routes */}
        <Route
          path="/"
          element={
            <ProtectedRoute session={session}>
              <RoomSelection />
            </ProtectedRoute>
          }
        />
        <Route
          path="/canva/:roomId"
          element={
            <ProtectedRoute session={session}>
              <Whiteboard />
            </ProtectedRoute>
          }
        />

        {/* Fallback */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>

      {session && <OfflineAssistance />}
    </Router>
  );
}

export default App;
