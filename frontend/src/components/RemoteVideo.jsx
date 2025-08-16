import { useRef } from "react";
import { useEffect } from "react";

function RemoteVideo({ stream, muted = false, className = '' }) {
    const ref = useRef(null);
    useEffect(() => {
        if (!ref.current) return;
        if (stream) {
            try { ref.current.srcObject = stream; }
            catch (e) { console.warn('set srcObject failed', e); }
        } else {
            ref.current.srcObject = null;
        }
    }, [stream]);

    return (
        <video
            ref={ref}
            autoPlay
            playsInline
            muted={muted}
            className={className}
        />
    );
}

export default RemoteVideo;