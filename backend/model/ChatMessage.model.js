// models/ChatMessage.js
import mongoose from 'mongoose';

const ChatMessageSchema = new mongoose.Schema({
    roomId: { type: String, required: true, index: true },
    userId: { type: String, required: true, index: true },
    username: { type: String, required: true },
    text: { type: String, required: true, maxlength: 2000 },
    createdAt: { type: Date, default: Date.now, index: true }
});

export const ChatMessage = mongoose.model('ChatMessage', ChatMessageSchema);
