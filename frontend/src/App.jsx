import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import RoomSelection from './components/RoomSelection';
import Whiteboard from './components/Whiteboard';

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<RoomSelection />} />
        <Route path="/canva/:roomId" element={<Whiteboard />} />
      </Routes>
    </Router>
  );
}

export default App;