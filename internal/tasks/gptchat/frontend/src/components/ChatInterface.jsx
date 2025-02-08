import React, { useContext, useEffect, useRef } from 'react';
import { ChatContext } from '../context/ChatContext';
import { ConfigContext } from '../context/ConfigContext';
import ChatMessage from './ChatMessage';
import ChatInput from './ChatInput';
import './ChatInterface.css'; // Import styles

function ChatInterface({ showAlert }) {
    const { chatHistory, loading, clearChat } = useContext(ChatContext);
    const { config } = useContext(ConfigContext);
    const chatContainerRef = useRef(null);

    // Scroll to bottom whenever chatHistory changes
    useEffect(() => {
        if (chatContainerRef.current) {
            chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
        }
    }, [chatHistory]);

    return (
        <div className="chat-interface">
            <div className="chat-messages" ref={chatContainerRef}>
                {chatHistory.map((message) => (
                    <ChatMessage key={`${message.role}-${message.chatID}`} message={message} />
                ))}
                {loading && (
                    <div className="loading-indicator">
                        <p className="card-text placeholder-glow">
                            <span className="placeholder col-7"></span>
                            <span className="placeholder col-4"></span>
                            <span className="placeholder col-4"></span>
                            <span className="placeholder col-6"></span>
                            <span className="placeholder col-8"></span>
                        </p>
                    </div>
                )}
            </div>
            <ChatInput />
            <div className="chat-footer">
                <button className="btn btn-danger clear-chats" onClick={() => clearChat(config.selectedSession)}>
                    Clear Chat
                </button>
            </div>
        </div>
    );
}

export default ChatInterface;
