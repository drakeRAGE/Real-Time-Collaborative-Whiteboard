import mongoose from 'mongoose';

const CallSessionSchema = new mongoose.Schema({
  callSessionId: { type: String, required: true, unique: true }, // Same as roomId
  status: { type: String, enum: ['active', 'ended'], default: 'active' },
  participants: [{
    userId: { type: String, required: true },
    username: { type: String },
    micEnabled: { type: Boolean, default: true },
    cameraEnabled: { type: Boolean, default: true },
    joinedAt: { type: Date, default: Date.now }
  }],
  startedAt: { type: Date, default: Date.now },
  endedAt: { type: Date, default: null }
});

export const CallSession = mongoose.model('CallSession', CallSessionSchema);