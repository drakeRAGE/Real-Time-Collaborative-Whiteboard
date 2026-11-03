import { useEffect, useState, type CSSProperties } from 'react';
import { IoCloudDoneOutline, IoCloudOfflineOutline } from 'react-icons/io5';
import { updateStatus } from '../utils/networkUtils';

const ONLINE_HASH = import.meta.env.VITE_NETWORK_ONLINE_HASH ?? '';
const OFFLINE_HASH = import.meta.env.VITE_NETWORK_OFFLINE_HASH ?? '';

export default function OfflineAssistance() {
  const [isOnline, setIsOnline] = useState(navigator.onLine);
  const [showBackOnline, setShowBackOnline] = useState(false);

  useEffect(() => {
    updateStatus(navigator.onLine, setIsOnline, setShowBackOnline, ONLINE_HASH, OFFLINE_HASH);
    const handleOnline = () => updateStatus(true, setIsOnline, setShowBackOnline, ONLINE_HASH, OFFLINE_HASH);
    const handleOffline = () => updateStatus(false, setIsOnline, setShowBackOnline, ONLINE_HASH, OFFLINE_HASH);
    window.addEventListener('online', handleOnline);
    window.addEventListener('offline', handleOffline);
    return () => {
      window.removeEventListener('online', handleOnline);
      window.removeEventListener('offline', handleOffline);
    };
  }, []);

  return (
    <>
      {!isOnline && (
        <div style={toastStyle('#fff1f0', '#cf1322', '#ffa39e')}>
          <IoCloudOfflineOutline size={22} />
          <span>You are offline. Changes may not be synced.</span>
        </div>
      )}
      {showBackOnline && (
        <div style={toastStyle('#f6ffed', '#389e0d', '#b7eb8f')}>
          <IoCloudDoneOutline size={22} />
          <span>You are back online.</span>
        </div>
      )}
    </>
  );
}

function toastStyle(bg: string, color: string, border: string): CSSProperties {
  return {
    position: 'fixed',
    top: '20px',
    left: '50%',
    transform: 'translateX(-50%)',
    backgroundColor: bg,
    color,
    padding: '10px 16px',
    border: `1px solid ${border}`,
    borderRadius: '10px',
    zIndex: 10000,
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  };
}
