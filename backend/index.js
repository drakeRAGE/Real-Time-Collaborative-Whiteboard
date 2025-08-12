
import express from 'express';
import http from 'http';
import { Server } from 'socket.io';
import cors from 'cors';
import dotenv from 'dotenv';
import mongoose from 'mongoose';
import { Room } from './model/Room.model.js';
import { createRoomRouter } from './routes/room.routes.js';
import { socketAuthMiddleware } from './auth/socketAuthMiddleware.js';
import { loadOrCreateUser } from './controllers/user.controller.js';
import { User } from './model/User.model.js';
import rateLimit from 'express-rate-limit';
import { ChatMessage } from './model/ChatMessage.model.js';

// in-memory cache for quick lookups during runtime
const userCache = new Map(); // Map<userId, userObj>
// runtime redo stacks (roomId -> Array<drawing>)
const redoStacks = new Map(); // Map<roomId, Array<doc>>

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

// Connect to MongoDB
mongoose.connect(process.env.MONGO)
  .then(() => console.log('Connected to MongoDB'))
  .catch(err => console.error('MongoDB connection error:', err));

const connectedUsers = new Map();

io.use(socketAuthMiddleware);

// --- Add near top of file (where you declare maps) ---
const connectedSockets = new Map(); // socketId -> { userId, username, connectedAt }
const userToSockets = new Map();    // userId -> Set(socketId)

// very naive: 1 message / 500ms 
const MIN_MSG_INTERVAL_MS = 500; // adjust as needed

// --- Then replace your io.on('connection') with the block below ---
io.on('connection', async (socket) => {
  const userId = socket.user?.id;
  const email = socket.user?.email || '';

  if (!userId) {
    console.warn('No userId on socket, disconnecting');
    socket.disconnect(true);
    return;
  }

  // --- LOAD OR CREATE USER INTO CACHE ON FIRST CONNECTION THIS UPTIME ---
  let userObj = userCache.get(userId);
  if (!userObj) {
    // loadOrCreateUser does an indexed DB lookup + upsert if not exists
    userObj = await loadOrCreateUser({ userId, email });
    userCache.set(userId, userObj);
  }

  const username = userObj.username || (email ? email.split('@')[0] : (userId || socket.id).slice(0, 6));

  // Track this socket
  connectedSockets.set(socket.id, { userId, username, connectedAt: Date.now() });
  if (!userToSockets.has(userId)) userToSockets.set(userId, new Set());
  userToSockets.get(userId).add(socket.id);

  // helper to find the first real room this socket has joined (not the socket id)
  const getRoomIdFromSocket = (s) => {
    for (const r of s.rooms) {
      if (r !== s.id) return r;
    }
    return null;
  };

  // helper: compute connected users currently present in a room (returns array of { userId, username })
  const getConnectedUsersInRoom = (room) => {
    return room.users
      .filter(u => {
        const s = userToSockets.get(u.userId);
        return s && s.size > 0;
      })
      .map(u => ({ userId: u.userId, username: u.username || (userCache.get(u.userId)?.username || '') }));
  };

  // Rate limiting for chat messages
  socket.data.lastChatMessageAt = 0;

  // JOIN ROOM
  socket.on('joinRoom', async (roomId) => {
    try {
      socket.join(roomId);

      // get or create room
      let room = await Room.findOneAndUpdate(
        { roomId },
        { $setOnInsert: { users: [], drawings: [], adminId: null } },
        { new: true, upsert: true }
      );

      // use username from cache (guaranteed loaded earlier)
      const cachedUser = userCache.get(userId);
      const usernameFromCache = cachedUser?.username || (email ? email.split('@')[0] : (userId || socket.id).slice(0, 6));

      // Add user to room.users if not present (store canonical user object)
      if (!room.users.some(u => u.userId === userId)) {
        room.users.push({ userId, username: usernameFromCache });
      } else {
        // ensure username in room.users is up-to-date with cache
        room.users = room.users.map(u => u.userId === userId ? { ...u, username: usernameFromCache } : u);
      }

      // Compute connected users (those who have active sockets)
      const connectedUsersInRoom = getConnectedUsersInRoom(room);

      // Assign admin if none and this is the first connected user
      if (!room.adminId && connectedUsersInRoom.length === 1) {
        room.adminId = userId;
        console.log(`Assigned admin ${userId} for room ${roomId}`);
      }

      await room.save();

      // Send initialData only to joining socket (drawings + adminId)
      socket.emit('initialData', { drawings: room.drawings, adminId: room.adminId });

      // --- NEW: Send last 50 chat messages for this room ---
      const lastMessages = await ChatMessage.find({ roomId })
        .sort({ createdAt: -1 })
        .limit(50)
        .lean();
      socket.emit('chat:history', lastMessages.reverse());

      // Broadcast userJoined with canonical connectedUsersInRoom
      io.to(roomId).emit('userJoined', {
        userId,
        users: connectedUsersInRoom,
        username: usernameFromCache,
        adminId: room.adminId
      });

      console.log(`User ${userId} joined room ${roomId}`);
    } catch (err) {
      console.error('joinRoom error', err);
      socket.emit('errorMsg', 'Could not join room');
    }
  });

  // CURSOR MOVE
  socket.on('cursorMove', ({ roomId, x, y }) => {
    const socketInfo = connectedSockets.get(socket.id);
    const name = socketInfo?.username || username;
    socket.to(roomId).emit('cursorMove', {
      userId,
      x,
      y,
      username: name
    });
  });

  // --- CHAT SEND ---
  socket.on('chat:send', async (payload, ack) => {
    try {
      const now = Date.now();
      if (now - socket.data.lastChatMessageAt < MIN_MSG_INTERVAL_MS) {
        if (typeof ack === 'function') ack({ ok: false, error: 'You are sending messages too fast' });
        return;
      }
      socket.data.lastChatMessageAt = now;

      const { roomId, text } = payload || {};
      if (!roomId || !text || typeof text !== 'string' || text.trim().length === 0) {
        if (typeof ack === 'function') ack({ ok: false, error: 'roomId and text are required' });
        return;
      }

      // Sanitize & limit message length
      let cleanText = text.trim().substring(0, 2000);

      // Persist message in DB
      const messageDoc = new ChatMessage({
        roomId,
        userId,
        username,
        text: cleanText,
      });
      await messageDoc.save();

      // Broadcast to room
      const msgPayload = {
        _id: messageDoc._id,
        roomId,
        userId,
        username,
        text: cleanText,
        createdAt: messageDoc.createdAt
      };
      io.to(roomId).emit('chat:new', msgPayload);

      if (typeof ack === 'function') ack({ ok: true, message: msgPayload });
    } catch (err) {
      console.error('chat:send error', err);
      if (typeof ack === 'function') ack({ ok: false, error: 'Server error' });
    }
  });

  // DRAW (line)
  socket.on('draw', async (data) => {
    const roomId = getRoomIdFromSocket(socket);
    if (!roomId) return;
    try {
      // attach who created it
      const doc = { ...data, createdBy: userId };
      // push into DB
      await Room.updateOne({ roomId }, { $push: { drawings: doc } });

      // clear redo stack because new action invalidates redo history
      redoStacks.delete(roomId);

      // broadcast to other clients
      socket.to(roomId).emit('draw', doc);
    } catch (err) {
      console.error('draw error', err);
    }
  });

  // DRAW SHAPE
  socket.on('drawShape', async (data) => {
    const roomId = getRoomIdFromSocket(socket);
    if (!roomId) return;
    try {
      const doc = {
        x0: data.x0,
        y0: data.y0,
        x1: data.x1,
        y1: data.y1,
        color: data.color,
        size: data.size,
        shape: data.shape,
        createdBy: userId
      };
      await Room.updateOne({ roomId }, { $push: { drawings: doc } });

      // new action -> clear redo
      redoStacks.delete(roomId);

      socket.to(roomId).emit('drawShape', doc);
    } catch (err) {
      console.error('drawShape error', err);
    }
  });

  // CLEAR (anyone can clear — if you want admin-only, check room.adminId === userId)
  socket.on('clear', async () => {
    const roomId = getRoomIdFromSocket(socket);
    if (!roomId) return;
    try {
      await Room.updateOne({ roomId }, { $set: { drawings: [] } });

      // clearing board invalidates redo history
      redoStacks.delete(roomId);

      io.to(roomId).emit('clear');
    } catch (err) {
      console.error('clear error', err);
    }
  });

  // UNDO (only admin allowed)
  socket.on('undo', async (roomId) => {

    console.log("undoing");

    if (!roomId) return;
    try {
      const room = await Room.findOne({ roomId });
      if (!room) return socket.emit('errorMsg', 'Room not found');

      // server-side admin check (authoritative)
      if (room.adminId !== userId) {
        return socket.emit('errorMsg', 'Only the room admin can undo');
      }

      const drawings = room.drawings || [];
      if (drawings.length === 0) {
        return socket.emit('errorMsg', 'Nothing to undo');
      }

      // pop last drawing
      const last = drawings.pop();

      // save back to DB
      room.drawings = drawings;
      await room.save();

      // push popped drawing into redo stack
      const stack = redoStacks.get(roomId) || [];
      stack.push(last);
      redoStacks.set(roomId, stack);

      // notify everyone in room
      io.to(roomId).emit('undo', { removed: last, drawings });
    } catch (err) {
      console.error('undo error', err);
      socket.emit('errorMsg', 'Undo failed');
    }
  });

  // REDO (only admin allowed)
  socket.on('redo', async (roomId) => {
    if (!roomId) return;
    try {
      const room = await Room.findOne({ roomId });
      if (!room) return socket.emit('errorMsg', 'Room not found');

      if (room.adminId !== userId) {
        return socket.emit('errorMsg', 'Only the room admin can undo');
      }

      const stack = redoStacks.get(roomId) || [];
      if (stack.length === 0) {
        return socket.emit('errorMsg', 'Nothing to redo');
      }

      const doc = stack.pop();

      // append back into drawings and save
      room.drawings = room.drawings || [];
      room.drawings.push(doc);
      await room.save();

      // persist modified stack
      redoStacks.set(roomId, stack);

      io.to(roomId).emit('redo', { added: doc, drawings: room.drawings });
    } catch (err) {
      console.error('redo error', err);
      socket.emit('errorMsg', 'Redo failed');
    }
  });

  // DISCONNECT
  socket.on('disconnect', async () => {
    try {
      console.log(`Socket disconnected: socketId=${socket.id} userId=${userId}`);

      // remove this socket
      connectedSockets.delete(socket.id);
      const socketsSet = userToSockets.get(userId);
      if (socketsSet) {
        socketsSet.delete(socket.id);
        if (socketsSet.size === 0) {
          userToSockets.delete(userId);
        }
      }

      // Only if user has NO remaining sockets do we remove them from rooms
      const stillConnected = userToSockets.has(userId); // true if user still has other sockets
      if (!stillConnected) {
        // find rooms where this user is a member
        const rooms = await Room.find({ 'users.userId': userId });
        for (const room of rooms) {
          // remove user object from room.users
          room.users = room.users.filter(u => u.userId !== userId);

          // recompute connected users in that room (users with active sockets)
          const connectedUsersInRoom = getConnectedUsersInRoom(room);

          // if they were admin, transfer admin to first connected user or null
          if (room.adminId === userId) {
            room.adminId = connectedUsersInRoom.length > 0 ? connectedUsersInRoom[0].userId : null;
          }

          await room.save();

          io.to(room.roomId).emit('userLeft', {
            userId,
            users: connectedUsersInRoom,
            username
          });
        }
      } else {
        // user still has other sockets — optional: you could emit presence update if needed
      }
    } catch (err) {
      console.error('disconnect handler error', err);
    }
  });
});


const PORT = 5000;

app.use('/api', createRoomRouter(io));
server.listen(PORT, () => console.log(`Server running on port ${PORT}`));