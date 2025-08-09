// auth/supabaseAuth.js
import jwt from 'jsonwebtoken';
import dotenv from 'dotenv';
dotenv.config();

/**
 * Verify Supabase JWT Access Token from frontend.
 * Returns decoded user object if valid, otherwise throws an error.
 */
export const verifySupabaseToken = (token) => {
    console.log('Verifying Supabase token:', token);
    if (!token) throw new Error('No token provided');

    try {
        // Supabase signs access tokens with your JWT secret (from API settings in Supabase)
        const decoded = jwt.verify(token, process.env.SUPABASE_JWT_SECRET);
        return decoded; // contains user id (sub), email, etc.
    } catch (err) {
        throw new Error('Invalid or expired token');
    }
};
