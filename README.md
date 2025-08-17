# Whiteboard - Real Time Collaboration

Collaborative Whiteboard is a real-time interactive platform that enables teams to brainstorm, design, and communicate seamlessly through a shared digital workspace. Combining whiteboard drawing tools with video conferencing and chat functionality, it provides a comprehensive solution for remote collaboration, online education, and creative teamwork.

## 🚀 Live Demo

[https://real-time-collaborative-whiteboard-xkh3.onrender.com](https://real-time-collaborative-whiteboard-xkh3.onrender.com)

## ✨ Features

- **User Authentication**:
  - Secure registration with email/password
  - JWT token generation and validation
  - Supabase integration for auth services
  - Socket authentication middleware

- **Room Management**:
  - Room creation with unique IDs
  - Join room functionality
  - Room persistence in database
  - Room participant tracking

- **Real-time Collaboration**:
  - WebSocket connections for live updates
  - Drawing synchronization between users
  - Cursor position sharing
  - Presence indicators for online users

- **Whiteboard Drawing**:
  - Freehand drawing tools
  - Shape creation (lines, rectangles, circles)
  - Color selection palette
  - Eraser functionality
  - Layer management

- **Chat Functionality**:
  - Real-time message broadcasting
  - Message history persistence
  - Typing indicators
  - Message timestamps

- **Video Calling**:
  - Peer-to-peer WebRTC connections
  - Video session tracking
  - Mute/unmute controls
  - Camera on/off toggles

- **User Profiles**:
  - Profile information storage
  - Activity history
  - Connection status tracking

- **Database Integration**:
  - Room model with metadata
  - User model with credentials
  - Chat message model with timestamps
  - Call session model with participants

- **Frontend UI**:
  - Responsive whiteboard canvas
  - Toolbar with drawing options
  - Sidebar for room participants
  - Chat message display area
  - Video call interface

- **Asset Management**:
  - Static asset serving
  - Icon storage for UI elements
  - Theme customization options
        
## 🛠️ Tech Stack

### **Backend Tech Stack**
- **Runtime**: Node.js
- **Framework**: Express.js
- **Database**: 
  - MongoDB (via Mongoose)
  - PostgreSQL (via pg for Supabase)
- **Authentication**: 
  - Supabase (via @supabase/supabase-js)
  - JWT (jsonwebtoken)
- **Real-time Communication**: Socket.io
- **Other Libraries**:
  - CORS for cross-origin requests
  - dotenv for environment variables
  - express-rate-limit for API rate limiting
  - UUID for unique ID generation

### **Frontend Tech Stack**
- **Framework**: React 19
- **Bundler**: Vite
- **UI Components**: 
  - Material-UI (MUI)
  - Emotion for CSS-in-JS
- **State Management**: React hooks
- **Routing**: React Router DOM
- **Real-time Communication**: 
  - Socket.io-client
  - Simple-peer for WebRTC
- **Styling**: Tailwind CSS
- **Authentication**: Supabase
- **Utility Libraries**:
  - React Icons
  - UUID

### **Shared Technologies**
- **Authentication**: Supabase (@supabase/supabase-js)
- **Utilities**: UUID for ID generation
- **Real-time**: Socket.io (server) and socket.io-client (frontend)
        
## 📦 Installation & Setup

### Prerequisites
- Node.js (v14.0.0 or higher)
- MongoDB instance (local or cloud)
- npm or yarn package manager

### Local Development

1. **Clone the repository**
   ```bash
   git clone https://github.com/drakeRAGE/Real-Time-Collaborative-Whiteboard.git
   cd Real-Time-Collaborative-Whiteboard
   ```

2. **Install dependencies**
   ```bash
   # Install server dependencies
   cd backend && npm install

   # Install client dependencies
   cd ../frontend && npm install
   ```

3. **Environment Setup**
   
   Create a `.env` file in the `backend` directory:
   ```env
    PORT=5000
    MONGO = 'mongodb://localhost:27017/whiteboard'
    SUPABASE_JWT_SECRET=Your_Secret
    NODE_ENV=production
   ```


   Create a `.env` file in the `frontend` directory:
   ```env
    VITE_NETWORK_ONLINE_HASH=create_hash_value
    VITE_NETWORK_OFFLINE_HASH=create_hash_value
    VITE_APP_SUPABASE_URL=Your_supabase_url
    VITE_APP_SUPABASE_ANON_KEY=Your_supabase_key
    VITE_BACKEND_URL=http://localhost:5000
   ```

4. **Database Setup**
   Add MONGO_URI variable in the .env file in backend directory.

5. **Start Development Servers**
   ```bash
   # Terminal 1 - Backend
   cd backend && npm run dev

   # Terminal 2 - Frontend
   cd frontend && npm run dev
   ```

### Production Deployment

#### Render Deployment (Recommended)

1. **Push your code to GitHub**

2. **Create a new Web Service on Render**
   - Connect your GitHub repository
   - Set the build command: `npm run build`
   - Set the start command: `npm start`
   - Add environment variables in the Render dashboard

3. **Environment Variables for Production**
   ```env
    VITE_NETWORK_ONLINE_HASH=
    VITE_NETWORK_OFFLINE_HASH=
    VITE_APP_SUPABASE_URL=
    VITE_APP_SUPABASE_ANON_KEY=
    VITE_BACKEND_URL=http://localhost:5000
    PORT=5000
    MONGO = 'mongodb://localhost:27017/whiteboard'
    SUPABASE_JWT_SECRET=
    NODE_ENV=production
   ```

## 🎯 Key Usage Scenarios

- **Remote Team Brainstorming**
  - Simultaneous drawing and annotation
  - Real-time idea sharing with visual elements
  - Persistent workspace for ongoing projects

- **Online Education & Tutoring**
  - Interactive lessons with shared whiteboard
  - Teacher-student collaboration
  - Visual explanations of complex concepts

- **Design Collaboration**
  - Wireframing and prototyping
  - UI/UX design feedback sessions
  - Creative concept development

- **Agile Development**
  - Sprint planning with visual task boards
  - Architecture diagramming
  - Team stand-ups with shared notes

- **Customer Support**
  - Visual troubleshooting guides
  - Product demonstrations
  - Real-time technical explanations

- **Creative Workshops**
  - Design thinking sessions
  - Mind mapping exercises
  - Visual storytelling collaborations

- **Remote Interviews**
  - Technical problem-solving on shared space
  - Coding interview whiteboarding
  - Collaborative skill assessment

## 🏗️ Project Structure

```
Real-Time-Collaborative-Whiteboard/
├── frontend/              # React frontend
│   ├── src/
│   │   ├── components/    # Reusable components and event pages
│   │   ├── context/       # React context providers for Supabase Auth
│   │   ├── network/       # To handle Offline Assistance
│   │   ├── UI/            # Custom UI components
│   │   └── utils/         # Utility functions for canvas, network, socket and supabsae connection
│   │   ├── App.jsx        # App routes
│   ├── public/            # Static assets
│   └── package.json
├── backend/               # Express backend
│   ├── auth/              # Authentication middlewares and Supabase verification
│   ├── controllers/       # Route controllers for room and user models
│   ├── model/             # MongoDB models for callsession, chatmessage, room and user
│   ├── routes/            # API routes for room
│   ├── index.js           # Main backend file (also responsible for handling websockets connection)
│   └── package.json
└── package.json           # Root package.json
```

## 📱 Screenshots

### Room Page
<img width="1896" height="900" alt="image" src="https://github.com/user-attachments/assets/944fdd54-bd9e-42b7-86cd-600599552257" />

### Whiteboard
<img width="1897" height="904" alt="image" src="https://github.com/user-attachments/assets/db776949-bf6a-4d26-8763-ef9de42ecde2" />

### Real-Time Chat
<img width="1899" height="877" alt="image" src="https://github.com/user-attachments/assets/9f613ebc-dfcc-4a7e-ae8e-cddb2720c735" />

### Users List
<img width="1899" height="900" alt="image" src="https://github.com/user-attachments/assets/9f377ff2-716c-490f-b21b-e9bef13d88a1" />

### Video Call
<img width="402" height="907" alt="image" src="https://github.com/user-attachments/assets/f320879f-52bb-4bc8-a3aa-4063ad3d7c3c" />

<img width="1901" height="899" alt="image" src="https://github.com/user-attachments/assets/ec29375b-e0ee-46b3-a98e-4f1ae2149312" />

### **Socket Events and Functionalities**

1. **connection**
   - Establishes socket connection
   - Handles user authentication via middleware
   - Initializes user tracking in memory

2. **joinRoom**
   - Joins a user to a specific room
   - Creates room if it doesn't exist
   - Maintains room participant list
   - Handles admin assignment
   - Sends initial room data (drawings, admin status)
   - Broadcasts user join notification

3. **cursorMove**
   - Tracks and broadcasts cursor position
   - Used for showing other users' cursor locations

4. **chat:send**
   - Handles chat message sending
   - Includes rate limiting
   - Persists messages to database
   - Broadcasts messages to room

5. **draw**
   - Handles freehand drawing
   - Persists drawings to database
   - Broadcasts drawing data to room
   - Manages undo/redo stacks

6. **drawShape**
   - Handles predefined shape drawing (lines, rectangles, circles)
   - Similar persistence and broadcasting as freehand drawing

7. **clear**
   - Clears the whiteboard
   - Resets drawings in database
   - Broadcasts clear command to all room participants

8. **undo**
   - Removes last drawing action
   - Maintains redo stack
   - Updates database and broadcasts changes

9. **redo**
   - Reapplies last undone action
   - Updates from redo stack
   - Updates database and broadcasts changes

10. **call:join**
    - Handles video call initiation
    - Manages call participants
    - Broadcasts participant join notifications

11. **call:leave**
    - Handles participant leaving call
    - Updates participant list
    - Ends call if last participant leaves

12. **call:toggleMic**
    - Handles microphone mute/unmute
    - Broadcasts state to other participants

13. **call:toggleCamera**
    - Handles camera on/off
    - Broadcasts state to other participants

14. **call:signal**
    - WebRTC signaling for peer connections
    - Routes signaling data between participants

15. **disconnect**
    - Handles socket disconnection
    - Cleans up user tracking
    - Handles room participant updates
    - Manages admin reassignment

### **Special Socket Rooms**
- Regular rooms (for whiteboard collaboration)
- `call:[roomId]` rooms (for video call sessions)

All socket events are integrated with MongoDB for persistence and include proper error handling and state management.

## 🚀 Available Scripts

### Root Level
- `npm start` - Start production server
- `npm run dev` - Start development mode (both client & server)
- `npm run build` - Build for production
- `npm run install` - Install all dependencies

### Client
- `npm run dev` - Start development server
- `npm run build` - Build for production

### Server
- `npm start` - Start production server
- `npm run dev` - Start development server with nodemon

## 🧪 Testing

```bash
# Run client tests
npm test

# Run server tests
npm test
```

## 🐛 Troubleshooting

### Common Issues

1. **Build fails on Render**
   - Ensure all dependencies are properly listed in package.json
   - Check Node.js version compatibility
   - Use `--legacy-peer-deps` flag for dependency conflicts (especially if you are using swc with vite+js)

2. **MongoDB connection issues**
   - Verify your MongoDB connection string
   - Ensure your IP is whitelisted in MongoDB Atlas
   - Check if MongoDB service is running locally

3. **CORS issues**
   - Ensure CORS is properly configured in the server
   - Check if frontend API URL matches backend URL

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request


## 👥 Contact

**Deepak Joshi** - [@drakeRAGE](https://github.com/drakeRAGE)

**Project Link**: [https://github.com/drakeRAGE/Real-Time-Collaborative-Whiteboard](https://github.com/drakeRAGE/Real-Time-Collaborative-Whiteboard)

## 🙏 Acknowledgments

- [React](https://reactjs.org/) for the amazing frontend framework
- [Express.js](https://expressjs.com/) for the robust backend framework
- [Tailwind CSS](https://tailwindcss.com/) for the beautiful styling
- [MongoDB](https://www.mongodb.com/) for the flexible database
- [Render](https://render.com/) for the free hosting service
- [Socket.io](https://www.npmjs.com/package/socket.io) for providing websockers connection.

<!-- Support Section -->
<div style="background: linear-gradient(135deg, #f5f7fa 0%, #c3cfe2 100%); padding: 2rem; border-radius: 10px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); margin: 2rem 0;">
  <h2 style="color: #2d3748; margin-bottom: 1rem;">☕ Support My Work</h2>
  
  <p style="color: #4a5568; margin-bottom: 1.5rem;">
    If you find this project useful and would like to support my development efforts, 
    consider buying me a coffee! Your support helps me continue maintaining and 
    improving this project.
  </p>
  
  <a href="https://buymeacoffee.com/drakeRAGE" target="_blank" 
     style="display: inline-block; background: #FF813F; color: white; 
            padding: 0.8rem 1.5rem; border-radius: 5px; text-decoration: none; 
            font-weight: bold; transition: all 0.3s ease;">
    🚀 Buy Me A Coffee
  </a>
  
  <p style="color: #718096; font-size: 0.9rem; margin-top: 1.5rem;">
    Every contribution, no matter how small, is greatly appreciated!
  </p>
</div>

---

<div align="center">
  <p>Made with ❤️ by <a href="https://github.com/drakeRAGE">Deepak Joshi</a></p>
  <p>⭐ Don't forget to star this repository if you find it helpful!</p>
</div>


        
