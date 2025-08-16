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
        <div className="">
            {/* Button */}
            <button
                onClick={handleCopy}
                className={`rounded-lg transition bg-transparent cursor-pointer text-gray-400 border-indigo-400 hover:text-indigo-500`}
            >
                <FaShare className="text-md" />
            </button>

            {/* Copied Toast */}
            {copied && (
                <div className="absolute mt-5 left-0 bg-emerald-500 text-white text-sm font-medium px-4 py-2 rounded-lg shadow-md animate-fade-in">
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
