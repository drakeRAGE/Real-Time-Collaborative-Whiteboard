import { useState } from 'react';
import { FaShare } from 'react-icons/fa';
import { useToast } from '../context/ToastContext';

export default function CopyUrl() {
  const [copied, setCopied] = useState(false);
  const toast = useToast();

  return (
    <div>
      <button
        type="button"
        aria-label="Copy room link"
        title="Copy room link"
        className="cursor-pointer text-gray-400 hover:text-indigo-500"
        onClick={() => {
          void navigator.clipboard.writeText(window.location.href).then(() => {
            setCopied(true);
            toast('Link copied');
            setTimeout(() => setCopied(false), 2000);
          }).catch(() => toast('Could not copy the link'));
        }}
      >
        <FaShare />
      </button>
      {copied && <div className="absolute mt-5 rounded-lg bg-emerald-500 px-4 py-2 text-sm text-white">Link copied</div>}
    </div>
  );
}
