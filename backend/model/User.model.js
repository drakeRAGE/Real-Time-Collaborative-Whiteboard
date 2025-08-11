import mongoose from "mongoose";

const UserSchema = new mongoose.Schema({
    userId: { type: String, required: true, unique: true }, // Supabase UUID
    email: { type: String, required: true },
    username: {
        type: String,
        default: function () {
            // `this` refers to the document being created
            return this.email ? this.email.split('@')[0] : '';
        }
    }
});

export const User = mongoose.model('User', UserSchema);
