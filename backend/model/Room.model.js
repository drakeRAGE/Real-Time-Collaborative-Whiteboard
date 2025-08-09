import mongoose from "mongoose";

const roomSchema = new mongoose.Schema({
  roomId: { type: String, required: true, unique: true },
  users: [
    {
      userId: { type: String, required: true },     // Supabase UUID
      username: { type: String }
    }
  ],
  drawings: [{
    x0: Number,
    y0: Number,
    x1: Number,
    y1: Number,
    color: String,
    size: Number,
    shape: String,
    createdBy: String   // Supabase user ID of the drawer
  }],
  adminId: { type: String, default: null } // Supabase UUID
});

export const Room = mongoose.model('Room', roomSchema);