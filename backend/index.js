
import express from 'express';
import http from 'http';
import { Server } from 'socket.io';
import cors from 'cors';
import dotenv from 'dotenv';
import mongoose from 'mongoose';

const app = express();
const server = http.createServer(app);
const io = new Server(server, {
  cors: {
    origin: '*',
  },
  pingTimeout: 60000, // Increase timeout
  pingInterval: 25000,
  transports: ['websocket', 'polling'] // Explicitly set transports
});
dotenv.config();

app.use(cors());

const rooms = new Map();

// Connect to MongoDB
mongoose.connect(process.env.MONGO)
  .then(() => console.log('Connected to MongoDB'))
  .catch(err => console.error('MongoDB connection error:', err));

// Define schemas
const roomSchema = new mongoose.Schema({
  roomId: String,
  users: [String],
  drawings: [{
    x0: Number,
    y0: Number,
    x1: Number,
    y1: Number,
    color: String,
    size: Number,
    shape: String // Add shape type
  }]
});

const Room = mongoose.model('Room', roomSchema);

io.on('connection', (socket) => {
  // Add user to connected users map
  connectedUsers.set(socket.id, { id: socket.id, timestamp: Date.now() });

  socket.on('joinRoom', async (roomId) => {
    socket.join(roomId);
    
    let room = await Room.findOne({ roomId });
    if (!room) {
        room = new Room({ 
            roomId,
            users: [],
            drawings: []
        });
    }

    const username = `User ${room.users.length + 1}`;
    
    // Store username in connectedUsers
    connectedUsers.set(socket.id, { 
        id: socket.id, 
        timestamp: Date.now(),
        username 
    });
    
    if (!room.users.includes(socket.id)) {
        room.users.push(socket.id);
        await room.save();
    }

    socket.emit('initialData', room.drawings);
    
    const connectedUsersInRoom = room.users.filter(userId => connectedUsers.has(userId));
    
    io.to(roomId).emit('userJoined', { 
        userId: socket.id, 
        users: connectedUsersInRoom,
        username
    });
    
    console.log(`User ${socket.id} joined room ${roomId}`);
});

  socket.on('cursorMove', ({ roomId, x, y }) => {
      // Get the username from connectedUsers map instead
      const userData = connectedUsers.get(socket.id);
      const username = userData?.username || `User ${socket.id.slice(0, 4)}`;
      
      socket.to(roomId).emit('cursorMove', { 
          userId: socket.id, 
          x, 
          y,
          username
      });
  });
  socket.on('draw', async (data) => {
    const roomId = [...socket.rooms].find(room => room !== socket.id);
    if (roomId) {
      // Save drawing to database
      await Room.updateOne(
        { roomId },
        { $push: { drawings: data } }
      );
      socket.to(roomId).emit('draw', data);
    }
  });

  socket.on('drawShape', async (data) => {
    const roomId = [...socket.rooms].find(room => room !== socket.id);
    if (roomId) {
        // Ensure shape data is included when saving to database
        await Room.updateOne(
            { roomId },
            { $push: { 
                drawings: {
                    x0: data.x0,
                    y0: data.y0,
                    x1: data.x1,
                    y1: data.y1,
                    color: data.color,
                    size: data.size,
                    shape: data.shape // Make sure shape is included
                }
            } }
        );
        socket.to(roomId).emit('drawShape', data);
    }
});

  socket.on('clear', async () => {
    const roomId = [...socket.rooms].find(room => room !== socket.id);
    if (roomId) {
        // Clear drawings from database
        await Room.updateOne(
            { roomId },
            { $set: { drawings: [] } }
        );
        // Emit to all clients in the room including the sender
        io.to(roomId).emit('clear');
    }
});

  socket.on('disconnect', async () => {
    console.log('User disconnected:', socket.id);
    connectedUsers.delete(socket.id);
    
    const rooms = await Room.find({ users: socket.id });
    for (const room of rooms) {
      room.users = room.users.filter(userId => userId !== socket.id);
      await room.save();
      
      // Get remaining connected users
      const remainingUsers = room.users.filter(userId => connectedUsers.has(userId));
      
      io.to(room.roomId).emit('userLeft', { 
        userId: socket.id, 
        users: remainingUsers,
        username: `User ${remainingUsers.length + 1}` // Add username for toast
      });
    }
  });
});

// Add this at the top with other declarations
const connectedUsers = new Map();

const PORT = 5000;
// Add this route before server.listen
app.delete('/api/rooms/:roomId', async (req, res) => {
    try {
        const { roomId } = req.params;
        await Room.deleteOne({ roomId });
        io.to(roomId).emit('roomDeleted');
        res.status(200).json({ success: true });
    } catch (error) {
        console.error('Error deleting room:', error);
        res.status(500).json({ success: false });
    }
});

server.listen(PORT, () => console.log(`Server running on port ${PORT}`));