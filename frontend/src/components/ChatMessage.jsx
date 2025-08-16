import React, { useEffect, useState, useRef } from "react";
import { FiSend, FiX, FiMessageSquare } from "react-icons/fi";

export default function ChatMessage({ socket, roomId, userId, username }) {
    const [message, setMessage] = useState("");
    const [messages, setMessages] = useState([]);
    const [open, setOpen] = useState(false);
    const messagesEndRef = useRef(null);

    // Scroll chat to bottom on new messages
    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        if (!socket || !roomId) return;

        socket.emit("joinRoom", roomId);

        socket.on("chat:history", (history) => {
            setMessages(history);
        });

        socket.on('chat:new', (msg) => {
            setMessages((prev) => {
                // Check if message with same _id already exists (including optimistic temp ID)
                if (prev.some((m) => m._id === msg._id)) {
                    return prev; // already have this message, skip adding duplicate
                }
                // If message text and userId matches a pending message, replace that optimistic message
                if (msg.userId === userId) {
                    const index = prev.findIndex((m) => m.pending && m.text === msg.text);
                    if (index !== -1) {
                        const newMessages = [...prev];
                        newMessages[index] = msg; // replace optimistic with real msg
                        return newMessages;
                    }
                }
                // Otherwise add message normally
                return [...prev, msg];
            });
        });


        return () => {
            socket.off("chat:history");
            socket.off("chat:new");
            if (socket.connected) {
                socket.emit("leaveRoom", roomId);
            }
            setMessages([]);
        };
    }, [socket, roomId]);

    useEffect(() => {
        if (open) scrollToBottom();
    }, [messages, open]);

    const sendMessage = () => {
        const trimmed = message.trim();
        if (!trimmed) return;

        const tempId = `temp-${Date.now()}`;
        const optimisticMsg = {
            _id: tempId,
            userId,
            username,
            text: trimmed,
            createdAt: new Date().toISOString(),
            pending: true,
        };
        setMessages((prev) => [...prev, optimisticMsg]);
        setMessage("");

        socket.emit("chat:send", { roomId, text: trimmed }, (ack) => {
            if (!ack.ok) {
                alert("Message failed: " + ack.error);
                setMessages((prev) => prev.filter((m) => m._id !== tempId));
            } else {
                setMessages((prev) =>
                    prev.map((m) => (m._id === tempId ? ack.message : m))
                );
            }
        });
    };

    const handleKeyDown = (e) => {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    };

    return (
        <>
            <div className="h-full flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between p-4 border-b border-gray-200 bg-gray-50">
                    <h3 className="text-lg font-semibold text-gray-800">Chat</h3>
                    <span className="px-2 py-1 bg-green-100 text-green-600 rounded-full text-sm font-medium">
                        {messages.length}
                    </span>
                </div>

                {/* Messages */}
                <div className="flex-1 overflow-y-auto p-4 space-y-3 bg-gray-50">
                    {messages.length === 0 ? (
                        <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                            <div className="w-12 h-12 bg-gray-200 rounded-full flex items-center justify-center mb-2">
                                <FiMessageSquare className="text-xl text-gray-400" />
                            </div>
                            <p className="text-sm text-center">No messages yet.<br />Start the conversation!</p>
                        </div>
                    ) : (
                        messages.map((msg) => {
                            const isSelf = msg.userId === userId;
                            return (
                                <div
                                    key={msg._id || `${msg.userId}-${msg.createdAt}`}
                                    className={`flex ${isSelf ? "justify-end" : "justify-start"}`}
                                >
                                    <div className={`max-w-[85%] ${isSelf ? "order-2" : "order-1"}`}>
                                        {!isSelf && (
                                            <div className="flex items-center gap-2 mb-1">
                                                <div className="w-6 h-6 bg-indigo-100 rounded-full flex items-center justify-center">
                                                    <span className="text-xs font-semibold text-indigo-600">
                                                        {(msg.username || "U").charAt(0).toUpperCase()}
                                                    </span>
                                                </div>
                                                <span className="text-xs font-medium text-gray-600">
                                                    {msg.username || "Unknown"}
                                                </span>
                                                <span className="text-xs text-gray-400">
                                                    {new Date(msg.createdAt).toLocaleTimeString([], {
                                                        hour: '2-digit',
                                                        minute: '2-digit'
                                                    })}
                                                </span>
                                            </div>
                                        )}
                                        <div
                                            className={`px-3 py-2 rounded-lg text-sm break-words ${isSelf
                                                    ? "bg-indigo-600 text-white ml-auto"
                                                    : "bg-white text-gray-900 border border-gray-200"
                                                }`}
                                            style={{ opacity: msg.pending ? 0.6 : 1 }}
                                        >
                                            {msg.text}
                                            {msg.pending && (
                                                <div className="text-xs italic text-white/70 mt-1">
                                                    Sending...
                                                </div>
                                            )}
                                        </div>
                                        {isSelf && (
                                            <div className="text-xs text-gray-400 text-right mt-1">
                                                {new Date(msg.createdAt).toLocaleTimeString([], {
                                                    hour: '2-digit',
                                                    minute: '2-digit'
                                                })}
                                            </div>
                                        )}
                                    </div>
                                </div>
                            );
                        })
                    )}
                    <div ref={messagesEndRef} />
                </div>

                {/* Input */}
                <div className="px-4 pt-4 border-t border-gray-200 bg-white">
                    <div className="flex gap-2">
                        <input
                            type="text"
                            value={message}
                            onChange={(e) => setMessage(e.target.value)}
                            onKeyDown={handleKeyDown}
                            placeholder="Type a message..."
                            maxLength={2000}
                            className="flex-1 px-3 py-2 text-black border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                        />
                        <button
                            onClick={sendMessage}
                            disabled={!message.trim()}
                            className={`px-3 py-2 rounded-lg flex items-center justify-center transition-colors ${message.trim()
                                    ? "bg-indigo-600 text-white hover:bg-indigo-700"
                                    : "bg-gray-200 text-gray-400 cursor-not-allowed"
                                }`}
                        >
                            <FiSend size={16} />
                        </button>
                    </div>
                </div>
            </div>
        </>
    );
}
