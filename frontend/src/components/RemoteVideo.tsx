import { useEffect, useRef } from 'react';

export default function RemoteVideo({ stream, muted = false, className = '' }: { stream: MediaStream | null; muted?: boolean; className?: string }) {
  const ref = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    node.srcObject = stream;
  }, [stream]);
  return <video ref={ref} autoPlay playsInline muted={muted} className={className} />;
}
