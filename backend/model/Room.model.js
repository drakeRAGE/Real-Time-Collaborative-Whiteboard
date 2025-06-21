import mongoose from "mongoose";

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

export const Room = mongoose.model('Room', roomSchema);