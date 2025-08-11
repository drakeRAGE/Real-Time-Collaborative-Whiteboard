import React, { useState } from 'react';
import { FaShare } from 'react-icons/fa';

const CopyUrl = () => {
    const [copied, setCopied] = useState(false);

    const handleCopy = () => {
        navigator.clipboard.writeText(window.location.href);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    return (
        <div className="fixed top-20 left-5 z-50">
            {/* Button */}
            <button
                onClick={handleCopy}
                className="flex items-center gap-2 px-4 py-2 rounded-full bg-white/80 backdrop-blur-md shadow-sm border border-gray-200 
                   text-blue-600 font-medium transition-all duration-200 hover:shadow-md hover:scale-105"
            >
                <FaShare className="text-lg" />
                <span>Share</span>
            </button>

            {/* Copied Toast */}
            {copied && (
                <div className="absolute mt-3 left-0 bg-emerald-500 text-white text-sm font-medium px-4 py-2 rounded-lg shadow-md animate-fade-in">
                    Link copied!
                </div>
            )}

            {/* Animation */}
            <style>{`
        @keyframes fade-in {
          from { opacity: 0; transform: translateY(-5px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .animate-fade-in {
          animation: fade-in 0.25s ease-out;
        }
      `}</style>
        </div>
    );
};

export default CopyUrl;
