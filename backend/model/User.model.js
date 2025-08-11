import mongoose from "mongoose";

const UserSchema = new mongoose.Schema({
    userId: { type: String, required: true, unique: true }, // Supabase UUID
    email: { type: String, required: true },
    username: {
        type: String, required: true,
    }
});

export const User = mongoose.model('User', UserSchema);
