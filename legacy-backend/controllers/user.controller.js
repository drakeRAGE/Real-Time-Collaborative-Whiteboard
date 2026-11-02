// controllers/user.controller.js
import { User } from '../model/User.model.js'; // adjust path

/**
 * Load user from in-memory cache if present, otherwise from MongoDB.
 * If not in DB, create (upsert) a new user doc.
 * Returns the mongoose user doc.
 */
export async function loadOrCreateUser({ userId, email }) {
    if (!userId) throw new Error('userId required');

    // IMPORTANT: do NOT query whole collection. Use indexed field userId.
    // Atomic upsert: create if missing, otherwise return existing.
    const usernameFallback = email ? email.split('@')[0] : '';

    const user = await User.findOneAndUpdate(
        { userId },
        {
            $setOnInsert: {
                userId,
                email,
                username: usernameFallback
            }
        },
        { new: true, upsert: true, setDefaultsOnInsert: true }
    ).lean();

    return user;
}
