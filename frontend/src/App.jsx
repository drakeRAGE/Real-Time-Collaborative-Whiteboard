import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import RoomSelection from './components/RoomSelection';
import Whiteboard from './components/Whiteboard';
import OfflineAssistance from './network/OfflineAssistance';
import { useEffect, useState } from 'react'
import { supabase } from './utils/supabase'
import { Auth } from './components/Auth'
import { initializeSocket } from './utils/socket'

function App() {
  const [session, setSession] = useState(null)
  const [socket, setSocket] = useState(null)

  useEffect(() => {
    // Set initial session
    supabase.auth.getSession().then(({ data: { session } }) => {
      setSession(session)
    })

    // Listen for auth changes
    const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
      setSession(session)
    })

    return () => subscription.unsubscribe()
  }, [])

  useEffect(() => {
    if (session) {
      initializeSocket()
        .then(setSocket)
        .catch(console.error)
    } else {
      if (socket) {
        socket.disconnect()
        setSocket(null)
      }
    }
  }, [session]);

  return (
    <Router>
      <Routes>
        {/* If no session, force /auth */}
        {!session ? (
          <>
            <Route path="/auth" element={<Auth />} />
            <Route path="*" element={<Navigate to="/auth" replace />} />
          </>
        ) : (
          <>
            <Route path="/" element={<RoomSelection />} />
            <Route path="/canva/:roomId" element={<Whiteboard />} />
            <Route path="/auth" element={<Auth />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </>
        )}
      </Routes>

      {session && <OfflineAssistance />}
    </Router>
  );
}

export default App;
