import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import RoomSelection from './components/RoomSelection';
import Whiteboard from './components/Whiteboard';
import OfflineAssistance from './network/OfflineAssistance';

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<RoomSelection />} />
        <Route path="/canva/:roomId" element={<Whiteboard />} />
      </Routes>
      <OfflineAssistance />
    </Router>
  );
}

export default App;