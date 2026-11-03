import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { RealtimeClient, type ConnectionState } from '../realtime/client';
import { websocketUrl } from '../realtime/url';
import { useAuth } from './AuthContext';
import { supabase } from '../utils/supabase';

interface SocketValue {
  client: RealtimeClient | null;
  state: ConnectionState;
  retry: () => void;
}

const SocketContext = createContext<SocketValue | null>(null);

async function freshAccessToken(): Promise<string> {
  if (!supabase) throw new Error('Supabase is not configured');
  const refreshed = await supabase.auth.refreshSession();
  const fresh = refreshed.data.session?.access_token;
  if (fresh) return fresh;
  const current = await supabase.auth.getSession();
  const token = current.data.session?.access_token;
  if (!token) throw refreshed.error ?? new Error('no session');
  return token;
}

export function SocketProvider({ children }: { children: ReactNode }) {
  const { session } = useAuth();
  const userId = session?.user.id ?? null;
  const [client, setClient] = useState<RealtimeClient | null>(null);
  const [state, setState] = useState<ConnectionState>('closed');

  useEffect(() => {
    if (!userId || !supabase) return;
    const httpBase = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080';
    const rt = new RealtimeClient({
      url: websocketUrl(httpBase),
      getToken: freshAccessToken,
    });
    setClient(rt);
    setState('connecting');
    const off = rt.onState(setState);
    rt.connect();
    return () => {
      off();
      rt.close();
      setClient(null);
      setState('closed');
    };
  }, [userId]);

  const retry = useCallback(() => {
    client?.retry();
  }, [client]);

  const value = useMemo(() => ({ client, state, retry }), [client, state, retry]);
  return <SocketContext.Provider value={value}>{children}</SocketContext.Provider>;
}

export function useSocket(): SocketValue {
  const value = useContext(SocketContext);
  if (!value) throw new Error('useSocket requires SocketProvider');
  return value;
}
