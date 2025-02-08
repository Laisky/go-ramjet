import React from 'react';
import './ChatMessage.css';

function ChatMessage({ message }) {
    const { role, content, chatID, attachHTML, model } = message;
    const isUser = role === 'user';

    return (
        <div className={`chat-message ${isUser ? 'user-message' : 'ai-message'}`} id={chatID}>
            <div className="message-container">
                <div className="message-icon">
                    {isUser ? '🤔️' : '🤖️'}
                </div>
                <div className="message-content">
                    <div dangerouslySetInnerHTML={{ __html: content }} />
                    {attachHTML && <div dangerouslySetInnerHTML={{ __html: attachHTML }} />}
                </div>
                {!isUser && model &&
                    <div className="model-indicator">
                        {model}
                    </div>
                }
            </div>
        </div>
    );
}

export default ChatMessage;
