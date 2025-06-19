import React, { useState } from 'react';
import { FaShare } from 'react-icons/fa';

const CopyUrl = () => {
    const [copied, setCopied] = useState(false);

    return (
        <div style={{ position: 'fixed', top: '20px', left: '20px', zIndex: 1000 }}>
            <div style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                backgroundColor: 'white',
                padding: '8px 12px',
                borderRadius: '24px',
                boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                transition: 'all 0.2s ease'
            }}
                onMouseEnter={e => e.currentTarget.style.boxShadow = '0 4px 12px rgba(0,0,0,0.15)'}
                onMouseLeave={e => e.currentTarget.style.boxShadow = '0 2px 8px rgba(0,0,0,0.1)'}>
                <button
                    onClick={() => {
                        navigator.clipboard.writeText(window.location.href);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2000);
                    }}
                    style={{
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px'
                    }}
                >
                    <FaShare style={{ color: '#3b82f6', fontSize: '18px' }} />
                    <span style={{
                        color: '#3b82f6',
                        fontSize: '14px',
                        fontWeight: 500
                    }}>Share</span>
                </button>
                {copied && (
                    <div style={{
                        position: 'absolute',
                        top: '50px',
                        right: '0',
                        backgroundColor: '#10b981',
                        color: 'white',
                        padding: '8px 16px',
                        borderRadius: '6px',
                        fontSize: '14px',
                        boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
                        animation: 'fadeIn 0.3s ease'
                    }}>
                        Link copied!
                    </div>
                )}
            </div>
        </div>
    )
}

export default CopyUrl