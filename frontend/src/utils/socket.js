import { io } from 'socket.io-client'
import { supabase } from './supabase'

export const initializeSocket = async () => {
  const { data: { session } } = await supabase.auth.getSession();

  if (!session) {
    throw new Error('No authenticated session');
  }

  const socket = io(import.meta.env.VITE_BACKEND_URL, {
    auth: { token: session.access_token },
    autoConnect: false // don’t connect automatically
  });

  socket.connect();
  return socket;
};
