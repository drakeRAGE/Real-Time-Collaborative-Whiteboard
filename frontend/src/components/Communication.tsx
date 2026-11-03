import { useState } from 'react';
import { FaChevronRight, FaComments, FaTimes, FaUsers, FaVideo } from 'react-icons/fa';
import ChatMessage from './ChatMessage';
import UsersList from './UsersList';
import VideoCallSession from './VideoCallSession';

const TABS = [
  { id: 'users', label: 'Users', icon: FaUsers },
  { id: 'chat', label: 'Chat', icon: FaComments },
  { id: 'video', label: 'Video', icon: FaVideo },
] as const;

export default function CommunicationPanel({ roomId, userId, username }: { roomId: string; userId: string | null; username: string }) {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<(typeof TABS)[number]['id']>('users');

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className={`fixed top-25 z-[9999] -translate-y-1/2 rounded-full p-3 text-white shadow-lg ${open ? 'right-82 bg-gray-700' : 'right-7 bg-indigo-600'}`}
        aria-label={open ? 'Close sidebar' : 'Open sidebar'}
      >
        {open ? <FaTimes className="text-lg" /> : <FaChevronRight className="text-lg" />}
      </button>
      <div className={`fixed top-0 right-0 z-[9998] h-full w-80 border-l border-gray-200 bg-white shadow-2xl transition-transform ${open ? 'translate-x-0' : 'translate-x-full'}`}>
        <div className="bg-indigo-700 p-4 text-white">
          <div className="flex rounded-lg bg-white/10 p-1">
            {TABS.map((item) => {
              const Icon = item.icon;
              return (
                <button
                  key={item.id}
                  type="button"
                  aria-label={item.label}
                  onClick={() => setTab(item.id)}
                  className={`flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm ${tab === item.id ? 'bg-white text-indigo-600' : 'text-white/80'}`}
                >
                  <Icon className="text-sm" />
                  <span className="hidden sm:inline">{item.label}</span>
                </button>
              );
            })}
          </div>
        </div>
        <div className="h-[88vh] overflow-hidden">
          {tab === 'users' && <UsersList />}
          {tab === 'chat' && <ChatMessage roomId={roomId} userId={userId} username={username} />}
          {tab === 'video' && <VideoCallSession roomId={roomId} userId={userId} username={username} />}
        </div>
      </div>
      {open && <div className="fixed inset-0 z-[9997] bg-black/20" onClick={() => setOpen(false)} />}
    </>
  );
}
