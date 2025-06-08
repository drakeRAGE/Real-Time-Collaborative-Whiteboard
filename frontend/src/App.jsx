import './App.css'
import Whiteboard from './components/Whiteboard'

function App() {
  return (
    <div className='h-screen bg-gradient-to-r from-pink-400 to-purple-300 flex items-center justify-center'>
      <h1 className='text-purple-600 text-4xl font-serif'>Collaborative Whiteboard</h1>
      <Whiteboard />
    </div>
  )
}

export default App
