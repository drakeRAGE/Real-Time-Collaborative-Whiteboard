
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
    size: Number
  }]
});

const Room = mongoose.model('Room', roomSchema);

io.on('connection', (socket) => {
  // console.log('User connected:', socket.id);

  socket.on('joinRoom', async (roomId) => {
    socket.join(roomId);
    
    // Find or create room
    let room = await Room.findOne({ roomId });
    if (!room) {
      room = new Room({ 
        roomId,
        users: [],
        drawings: []
      });
    }

    // Add user if not already in room
    if (!room.users.includes(socket.id)) {
      room.users.push(socket.id);
      await room.save();
    }

    // Send existing drawings to new user
    socket.emit('initialData', room.drawings);
    
    console.log(`User ${socket.id} joined room ${roomId}`);
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

  socket.on('clear', async () => {
    const roomId = [...socket.rooms].find(room => room !== socket.id);
    if (roomId) {
        // Clear drawings from database
        await Room.updateOne(
            { roomId },
            { $set: { drawings: [] } }
        );
        socket.to(roomId).emit('clear');
    }
});

  socket.on('disconnect', () => {
    console.log('User disconnected:', socket.id);
    rooms.forEach((users, roomId) => {
      if (users.has(socket.id)) {
        users.delete(socket.id);
        if (users.size === 0) {
          rooms.delete(roomId);
        }
      }
    });
  });
});

const PORT = 5000;
server.listen(PORT, () => console.log(`Server running on port ${PORT}`));