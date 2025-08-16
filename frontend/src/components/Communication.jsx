import React, { useState } from "react";
import { FaUsers, FaComments, FaVideo, FaTimes, FaChevronRight } from "react-icons/fa";
import UsersList from "./UsersList";
import ChatMessage from "./ChatMessage";
import VideoCallSession from "./VideoCallSession";

function CommunicationPanel({ socket, roomId, userId, username }) {
    const [isOpen, setIsOpen] = useState(false);
    const [activeTab, setActiveTab] = useState("users");

    const toggleSidebar = () => {
        setIsOpen(!isOpen);
    };

    const tabs = [
        { id: "users", label: "Users", icon: FaUsers, component: UsersList },
        { id: "chat", label: "Chat", icon: FaComments, component: ChatMessage },
        { id: "video", label: "Video", icon: FaVideo, component: VideoCallSession },
    ];

    const ActiveComponent = tabs.find(tab => tab.id === activeTab)?.component;

    return (
        <>
            {/* Toggle Button */}
            <button
                onClick={toggleSidebar}
                className={`fixed top-25 -translate-y-1/2 z-[9999] transition-all duration-300 ${isOpen
                        ? "right-82 bg-gray-700 hover:bg-gray-600"
                        : "right-7 bg-indigo-600 hover:bg-indigo-700"
                    } text-white p-3 rounded-full shadow-lg`}
                aria-label={isOpen ? "Close sidebar" : "Open sidebar"}
            >
                {isOpen ? (
                    <FaTimes className="text-lg" />
                ) : (
                    <FaChevronRight className="text-lg" />
                )}
            </button>

            {/* Sidebar */}
            <div
                className={`fixed top-0 right-0 h-full w-80 bg-white shadow-2xl transform transition-transform duration-300 z-[9998] border-l border-gray-200 ${isOpen ? "translate-x-0" : "translate-x-full"
                    }`}
            >
                <div className="bg-gradient-to-r from-indigo-600 to-indigo-700 text-white p-4">
                    {/* Tab Navigation */}
                    <div className="flex bg-white/10 rounded-lg p-1">
                        {tabs.map((tab) => {
                            const IconComponent = tab.icon;
                            return (
                                <button
                                    key={tab.id}
                                    onClick={() => setActiveTab(tab.id)}
                                    className={`flex-1 flex items-center justify-center gap-2 py-2 px-3 rounded-md text-sm font-medium transition-all ${activeTab === tab.id
                                            ? "bg-white text-indigo-600 shadow-sm"
                                            : "text-white/80 hover:text-white hover:bg-white/10"
                                        }`}
                                >
                                    <IconComponent className="text-sm" />
                                    <span className="hidden sm:inline">{tab.label}</span>
                                </button>
                            );
                        })}
                    </div>
                </div>

                {/* Content Area */}
                <div className="flex-1 h-[88vh] overflow-hidden">
                    {ActiveComponent && (
                        <ActiveComponent
                            socket={socket}
                            roomId={roomId}
                            userId={userId}
                            username={username}
                            isSidebarMode={true}
                        />
                    )}
                </div>
            </div>

            {/* Overlay */}
            {isOpen && (
                <div
                    className="fixed inset-0 bg-black/20 z-[9997]"
                    onClick={toggleSidebar}
                />
            )}
        </>
    );
}

export default CommunicationPanel;