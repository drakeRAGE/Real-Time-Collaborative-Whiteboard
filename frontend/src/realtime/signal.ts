export interface IceServer {
  urls: string | string[];
  username?: string;
  credential?: string;
}

const FALLBACK_ICE: IceServer[] = [{ urls: 'stun:stun.l.google.com:19302' }];

/** simple-peer signal JSON, carried in CallSignal.payload. */
export function callSignalPayload(signal: object): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(signal));
}

export function parseCallSignal(payload: Uint8Array): Record<string, unknown> {
  const value: unknown = JSON.parse(new TextDecoder().decode(payload));
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('invalid call signal');
  }
  return value as Record<string, unknown>;
}

/** `VITE_ICE_SERVERS` is a JSON array of RTCIceServer objects. */
export function readIceServers(raw: string | undefined): IceServer[] {
  if (!raw) return FALLBACK_ICE;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length === 0) return FALLBACK_ICE;
    const servers: IceServer[] = [];
    for (const item of parsed) {
      if (!item || typeof item !== 'object' || !('urls' in item)) return FALLBACK_ICE;
      const urls = (item as { urls?: unknown }).urls;
      if (typeof urls !== 'string' && !(Array.isArray(urls) && urls.every((u) => typeof u === 'string'))) {
        return FALLBACK_ICE;
      }
      const server: IceServer = { urls };
      const username = (item as { username?: unknown }).username;
      const credential = (item as { credential?: unknown }).credential;
      if (typeof username === 'string') server.username = username;
      if (typeof credential === 'string') server.credential = credential;
      servers.push(server);
    }
    return servers;
  } catch {
    return FALLBACK_ICE;
  }
}
