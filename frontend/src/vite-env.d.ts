/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_BACKEND_URL: string;
  readonly VITE_APP_SUPABASE_URL: string;
  readonly VITE_APP_SUPABASE_ANON_KEY: string;
  readonly VITE_ICE_SERVERS: string;
  readonly VITE_NETWORK_ONLINE_HASH: string;
  readonly VITE_NETWORK_OFFLINE_HASH: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
