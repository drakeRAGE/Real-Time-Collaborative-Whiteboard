import React, { useEffect, useState, useRef } from 'react';
import { FiSend } from 'react-icons/fi';

export default function ChatMessage({ socket, roomId, userId, username }) {
  const [message, setMessage] = useState('');
  const [messages, setMessages] = useState([]);
  const messagesEndRef = useRef(null);

  // Scroll chat to bottom on new messages
  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    if (!socket || !roomId) return;

    // Join room and listen for chat events
    socket.emit('joinRoom', roomId);

    socket.on('chat:history', (history) => {
      setMessages(history);
    });

    socket.on('chat:new', (msg) => {
      setMessages((prev) => [...prev, msg]);
    });

    // Cleanup listeners and leave room on unmount or room change
    return () => {
      socket.off('chat:history');
      socket.off('chat:new');
      if (socket.connected) {
        socket.emit('leaveRoom', roomId);
      }
      setMessages([]);
    };
  }, [socket, roomId]);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Optimistic send: add message immediately, rollback on failure
  const sendMessage = () => {
    const trimmed = message.trim();
    if (!trimmed) return;

    // Create a temporary local message ID
    const tempId = `temp-${Date.now()}`;

    // Add optimistic message to UI
    const optimisticMsg = {
      _id: tempId,
      userId,
      username,
      text: trimmed,
      createdAt: new Date().toISOString(),
      pending: true,
    };
    setMessages((prev) => [...prev, optimisticMsg]);
    setMessage('');

    socket.emit('chat:send', { roomId, text: trimmed }, (ack) => {
      if (!ack.ok) {
        alert('Message failed: ' + ack.error);
        // Remove optimistic message if send failed
        setMessages((prev) => prev.filter((m) => m._id !== tempId));
      } else {
        // Replace optimistic message with confirmed message from server
        setMessages((prev) =>
          prev.map((m) => (m._id === tempId ? ack.message : m))
        );
      }
    });
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div
      className="chat-container"
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        maxHeight: '100%',
        fontFamily: "'Segoe UI', Tahoma, Geneva, Verdana, sans-serif",
        border: '1px solid #ddd',
        borderRadius: 8,
        backgroundColor: '#fff',
        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
      }}
    >
      <div
        className="chat-messages"
        style={{
          flexGrow: 1,
          overflowY: 'auto',
          padding: 16,
          backgroundColor: '#f9f9f9',
          borderBottom: '1px solid #ddd',
        }}
      >
        {messages.map((msg) => (
          <div
            key={msg._id || `${msg.userId}-${msg.createdAt}`}
            style={{
              marginBottom: 12,
              display: 'flex',
              justifyContent: msg.userId === userId ? 'flex-end' : 'flex-start',
              opacity: msg.pending ? 0.6 : 1,
            }}
          >
            <div
              style={{
                maxWidth: '75%',
                backgroundColor: msg.userId === userId ? '#0B93F6' : '#e5e5ea',
                color: msg.userId === userId ? '#fff' : '#000',
                borderRadius: 20,
                padding: '10px 14px',
                boxShadow: '0 1px 2px rgba(0,0,0,0.1)',
                fontSize: 14,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                position: 'relative',
              }}
              title={new Date(msg.createdAt).toLocaleString()}
            >
              {msg.userId !== userId && (
                <div
                  style={{
                    fontWeight: 600,
                    marginBottom: 4,
                    fontSize: 12,
                    color: '#555',
                  }}
                >
                  {msg.username || 'Unknown'}
                </div>
              )}
              <div>{msg.text}</div>
              {msg.pending && (
                <span
                  style={{
                    position: 'absolute',
                    bottom: 4,
                    right: 8,
                    fontSize: 10,
                    color: 'rgba(255,255,255,0.7)',
                    fontStyle: 'italic',
                  }}
                >
                  Sending...
                </span>
              )}
            </div>
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          sendMessage();
        }}
        style={{
          display: 'flex',
          padding: 12,
          backgroundColor: '#fff',
          alignItems: 'center',
          gap: 12,
          borderTop: '1px solid #ddd',
          borderBottomLeftRadius: 8,
          borderBottomRightRadius: 8,
        }}
      >
        <textarea
          rows={1}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type a message..."
          style={{
            flexGrow: 1,
            resize: 'none',
            padding: 10,
            fontSize: 14,
            borderRadius: 20,
            border: '1px solid #ccc',
            outline: 'none',
            fontFamily: 'inherit',
            boxShadow: 'inset 0 1px 3px rgb(0 0 0 / 0.1)',
          }}
          maxLength={2000}
        />
        <button
          type="submit"
          disabled={!message.trim()}
          style={{
            border: 'none',
            backgroundColor: message.trim() ? '#0B93F6' : '#a0cfff',
            color: '#fff',
            padding: '10px 16px',
            borderRadius: '50%',
            cursor: message.trim() ? 'pointer' : 'not-allowed',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 2px 6px rgba(11, 147, 246, 0.4)',
            transition: 'background-color 0.2s ease',
          }}
          aria-label="Send message"
        >
          <FiSend size={20} />
        </button>
      </form>
    </div>
  );
}
