import React, { useEffect, useState } from 'react';
import { IoCloudOfflineOutline } from "react-icons/io5";
import { IoCloudDoneOutline } from "react-icons/io5";
import { updateStatus } from '../utils/networkUtils';

const ONLINE_HASH = import.meta.env.VITE_NETWORK_ONLINE_HASH;
const OFFLINE_HASH = import.meta.env.VITE_NETWORK_OFFLINE_HASH;

function OfflineAssistance() {
    const [isOnline, setIsOnline] = useState(navigator.onLine);
    const [showBackOnline, setShowBackOnline] = useState(false);

    useEffect(() => {
        //Passing `navigator.onLine` as `status` to show current network state.
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
                    <IoCloudOfflineOutline size={22} style={{ flexShrink: 0 }} />
                    <span style={{ lineHeight: 1.4 }}>
                        You are offline. Changes may not be synced.
                    </span>
                </div>
            )}

            {showBackOnline && (
                <div style={toastStyle('#f6ffed', '#389e0d', '#b7eb8f')}>
                    <IoCloudDoneOutline size={22} style={{ flexShrink: 0 }} />
                    <span style={{ lineHeight: 1.4 }}>
                        You are back online.
                    </span>
                </div>
            )}
        </>
    );
}

// Common toast style generator
function toastStyle(bgColor, textColor, borderColor) {
    return {
        position: 'fixed',
        top: '20px',
        left: '50%',
        transform: 'translateX(-50%)',
        backgroundColor: bgColor,
        color: textColor,
        padding: '10px 16px',
        border: `1px solid ${borderColor}`,
        borderRadius: '10px',
        boxShadow: '0 8px 20px rgba(0,0,0,0.15)',
        fontWeight: 500,
        zIndex: 10000,
        fontFamily: 'system-ui, sans-serif',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        minWidth: '300px',
        maxWidth: '90%',
        animation: 'fadeIn 0.3s ease-in-out'
    };
}

export default OfflineAssistance;
