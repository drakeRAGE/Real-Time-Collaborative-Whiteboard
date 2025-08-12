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

    const toggleOpen = () => setOpen((prev) => !prev);

    return (
        <>
            {/* Toggle button */}
            {!open && (
                <button
                    onClick={toggleOpen}
                    aria-label="Open chat"
                    className="fixed bottom-5 right-5 z-50 flex items-center justify-center w-14 h-14 rounded-full shadow-lg bg-blue-600 hover:bg-blue-700 text-white transition-colors"
                >
                    <FiMessageSquare size={28} />
                </button>
            )}

            {/* Chat panel */}
            <aside
                className={`fixed bottom-5 right-6 z-40 w-[320px] max-h-[480px] bg-white rounded-lg shadow-2xl flex flex-col overflow-hidden transform transition-transform duration-300 ${open ? "translate-y-0 opacity-100" : "translate-y-12 opacity-0 pointer-events-none"
                    }`}
                role="region"
                aria-label="Chat messages"
            >
                {/* Header */}
                <header className="flex items-center justify-between bg-blue-600 px-4 py-3">
                    <h2 className="text-white font-semibold text-lg select-none">Chat</h2>
                    <button
                        onClick={toggleOpen}
                        aria-label="Close chat"
                        className="text-white hover:text-gray-200"
                    >
                        <FiX size={20} />
                    </button>
                </header>

                {/* Messages list */}
                <div
                    className="flex-1 overflow-y-auto px-4 py-3 space-y-3 scrollbar-thin scrollbar-thumb-gray-300 scrollbar-track-gray-100"
                    style={{ fontFamily: "'Segoe UI', Tahoma, Geneva, Verdana, sans-serif" }}
                >
                    {messages.length === 0 && (
                        <p className="text-center text-gray-400 text-sm mt-8 select-none">
                            No messages yet. Start the conversation!
                        </p>
                    )}
                    {messages.map((msg) => {
                        const isSelf = msg.userId === userId;
                        return (
                            <div
                                key={msg._id || `${msg.userId}-${msg.createdAt}`}
                                className={`flex flex-col max-w-[80%] ${isSelf ? "ml-auto items-end" : "mr-auto items-start"
                                    }`}
                                title={new Date(msg.createdAt).toLocaleString()}
                            >
                                {!isSelf && (
                                    <span className="text-xs font-semibold text-gray-600 mb-0.5 select-none">
                                        {msg.username || "Unknown"}
                                    </span>
                                )}
                                <div
                                    className={`relative px-4 py-2 rounded-2xl break-words whitespace-pre-wrap shadow-md ${isSelf
                                            ? "bg-blue-600 text-white rounded-br-none"
                                            : "bg-gray-200 text-gray-900 rounded-bl-none"
                                        }`}
                                    style={{
                                        opacity: msg.pending ? 0.6 : 1,
                                    }}
                                >
                                    {msg.text}
                                    {msg.pending && (
                                        <span className="absolute bottom-1 right-3 text-xs italic text-white/70 select-none">
                                            Sending...
                                        </span>
                                    )}
                                    {/* Chat bubble tail */}
                                    <span
                                        className={`absolute bottom-0 ${isSelf ? "right-0" : "left-0"
                                            } w-3 h-3 bg-inherit rounded-tr-[6px] rounded-tl-[6px]`}
                                        style={{
                                            clipPath: isSelf
                                                ? "polygon(100% 0, 0% 100%, 100% 100%)"
                                                : "polygon(0 0, 100% 100%, 0 100%)",
                                            backgroundColor: isSelf ? "#2563eb" : "#e5e7eb",
                                        }}
                                    />
                                </div>
                            </div>
                        );
                    })}
                    <div ref={messagesEndRef} />
                </div>

                {/* Input area */}
                <form
                    onSubmit={(e) => {
                        e.preventDefault();
                        sendMessage();
                    }}
                    className="flex gap-3 px-4 py-3 border-t border-gray-200 bg-white"
                >
                    <textarea
                        rows={1}
                        value={message}
                        onChange={(e) => setMessage(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="Type a message..."
                        maxLength={2000}
                        className="flex-grow resize-none text-black rounded-full border border-gray-300 px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                        style={{ fontFamily: "inherit" }}
                    />
                    <button
                        type="submit"
                        disabled={!message.trim()}
                        aria-label="Send message"
                        className={`rounded-full p-2 flex items-center justify-center transition-colors ${message.trim()
                                ? "bg-blue-600 text-white hover:bg-blue-700 cursor-pointer shadow-md"
                                : "bg-blue-300 text-white cursor-not-allowed"
                            }`}
                    >
                        <FiSend size={20} />
                    </button>
                </form>
            </aside>

            {/* Scrollbar styles */}
            <style>{`
        /* For Firefox */
        * {
          scrollbar-width: thin;
          scrollbar-color: #cbd5e1 #f3f4f6;
        }
        /* For Chrome, Edge and Safari */
        .scrollbar-thin::-webkit-scrollbar {
          width: 6px;
        }
        .scrollbar-thin::-webkit-scrollbar-track {
          background: #f3f4f6;
          border-radius: 3px;
        }
        .scrollbar-thin::-webkit-scrollbar-thumb {
          background-color: #cbd5e1;
          border-radius: 3px;
        }
      `}</style>
        </>
    );
}
