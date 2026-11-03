declare module 'simple-peer' {
  interface Options {
    initiator?: boolean;
    trickle?: boolean;
    stream?: MediaStream;
    config?: { iceServers?: Array<{ urls: string | string[]; username?: string; credential?: string }> };
  }

  export interface Instance {
    signal(data: object): void;
    destroy(): void;
    on(event: 'signal', cb: (data: object) => void): void;
    on(event: 'stream', cb: (stream: MediaStream) => void): void;
    on(event: 'connect', cb: () => void): void;
    on(event: 'close', cb: () => void): void;
    on(event: 'error', cb: (err: Error) => void): void;
  }

  interface PeerConstructor {
    new (opts: Options): Instance;
  }

  const Peer: PeerConstructor;
  export default Peer;
}
