// auth/socketAuthMiddleware.js
import { verifySupabaseToken } from './supabaseAuth.js';

export const socketAuthMiddleware = (socket, next) => {
    try {
        const token = socket.handshake.auth?.token || socket.handshake.headers?.authorization?.split(' ')[1];
        const user = verifySupabaseToken(token);

        // Attach verified user to socket for later use
        socket.user = {
            id: user.sub, // Supabase user id
            email: user.email
        };

        next();
    } catch (err) {
        next(new Error('Authentication error: ' + err.message));
    }
};
