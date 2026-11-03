import { useEffect, useRef, useState } from 'react';
import { FiMessageSquare, FiSend } from 'react-icons/fi';
import { loadChatHistory, type ChatRecord } from '../api/gateway';
import { useSocket } from '../context/SocketProvider';
import { useToast } from '../context/ToastContext';
import type { Message as ChatWire } from '../realtime/gen/whiteboard/chat/v1/chat';
import { ulid } from '../realtime/ulid';
import { supabase } from '../utils/supabase';

interface ChatItem extends ChatRecord {
  pending: boolean;
}

function fromWire(msg: ChatWire): ChatItem {
  return {
    id: msg.id,
    userId: msg.userId,
    username: msg.username,
    body: msg.body,
    createdAt: msg.createdAt ? msg.createdAt.toISOString() : new Date().toISOString(),
    pending: false,
  };
}

export default function ChatMessage({ roomId, userId, username }: { roomId: string; userId: string | null; username: string }) {
  const { client } = useSocket();
  const toast = useToast();
  const [draft, setDraft] = useState('');
  const [messages, setMessages] = useState<ChatItem[]>([]);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    if (!client || !roomId) return;
    let cancel = false;
    const off = client.on('chatMessage', (msg) => {
      if (msg.roomId !== roomId) return;
      const item = fromWire(msg);
      setMessages((prev) => {
        const index = prev.findIndex((entry) => entry.id === item.id);
        if (index === -1) return [...prev, item];
        const next = prev.slice();
        next[index] = item;
        return next;
      });
    });
    void (async () => {
      if (!supabase) return;
      const { data } = await supabase.auth.getSession();
      const token = data.session?.access_token;
      if (!token || cancel) return;
      const history = await loadChatHistory(roomId, token);
      if (cancel) return;
      setMessages((prev) => {
        const ids = new Set(history.map((item) => item.id));
        return [...history.map((item) => ({ ...item, pending: false })), ...prev.filter((item) => !ids.has(item.id))];
      });
    })();
    return () => {
      cancel = true;
      off();
    };
  }, [client, roomId]);

  const sendMessage = () => {
    const body = draft.trim();
    if (!body || !client || !roomId) return;
    const id = ulid();
    const optimistic: ChatItem = {
      id,
      userId: userId ?? '',
      username,
      body,
      createdAt: new Date().toISOString(),
      pending: true,
    };
    setMessages((prev) => [...prev, optimistic]);
    setDraft('');
    void client.send({ chatSend: { messageId: id, roomId, body } }).catch((err: unknown) => {
      setMessages((prev) => prev.filter((item) => item.id !== id));
      toast(err instanceof Error ? err.message : 'Message failed');
    });
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-gray-200 bg-gray-50 p-4">
        <h3 className="text-lg font-semibold text-gray-800">Chat</h3>
        <span className="rounded-full bg-green-100 px-2 py-1 text-sm font-medium text-green-600">{messages.length}</span>
      </div>
      <div className="flex-1 space-y-3 overflow-y-auto bg-gray-50 p-4" aria-live="polite">
        {messages.length === 0 ? (
          <div className="flex h-32 flex-col items-center justify-center text-gray-500">
            <FiMessageSquare className="text-xl text-gray-400" />
            <p className="mt-2 text-center text-sm">No messages yet. Start the conversation.</p>
          </div>
        ) : messages.map((msg) => {
          const mine = msg.userId === userId;
          return (
            <div key={msg.id} className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
              <div className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${mine ? 'bg-indigo-600 text-white' : 'border border-gray-200 bg-white text-gray-900'}`} style={{ opacity: msg.pending ? 0.6 : 1 }}>
                {!mine && <p className="mb-1 text-xs text-gray-500">{msg.username || 'Unknown'}</p>}
                {msg.body}
              </div>
            </div>
          );
        })}
        <div ref={endRef} />
      </div>
      <form
        className="flex gap-2 border-t border-gray-200 bg-white px-4 py-4"
        onSubmit={(event) => {
          event.preventDefault();
          sendMessage();
        }}
      >
        <label htmlFor="chat-body" className="sr-only">Message</label>
        <input
          id="chat-body"
          value={draft}
          maxLength={2000}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="Type a message..."
          className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm text-black"
        />
        <button type="submit" aria-label="Send message" disabled={!draft.trim()} className="rounded-lg bg-indigo-600 px-3 py-2 text-white disabled:bg-gray-200 disabled:text-gray-400">
          <FiSend size={16} />
        </button>
      </form>
    </div>
  );
}
