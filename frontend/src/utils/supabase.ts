import { createClient, type SupabaseClient } from '@supabase/supabase-js';

const url = import.meta.env.VITE_APP_SUPABASE_URL ?? '';
const anonKey = import.meta.env.VITE_APP_SUPABASE_ANON_KEY ?? '';

export const supabaseConfigured = url.length > 0 && anonKey.length > 0;

export const supabase: SupabaseClient | null = supabaseConfigured
  ? createClient(url, anonKey)
  : null;
