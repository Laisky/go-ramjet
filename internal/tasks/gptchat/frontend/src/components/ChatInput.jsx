import React, { useState, useContext } from 'react';
import { ChatContext } from '../context/ChatContext';
import { ConfigContext } from '../context/ConfigContext'; // Import
import './ChatInput.css';


function ChatInput() {
    const { sendMessage } = useContext(ChatContext);
    const { config } = useContext(ConfigContext); // Access config
    const [prompt, setPrompt] = useState('');

    const handleKeyDown = (event) => {
        if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
            event.preventDefault();
            handleSend();
        }
    };

    const handleSend = () => {
        if (prompt.trim() !== '') {
            sendMessage(prompt, config.selected_model);
            setPrompt('');
        }
    };

    return (
        <div className="chat-input">
            <textarea
                className="form-control"
                placeholder={`Enter your message [${config.selected_model}] (Ctrl+Enter to send)`}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={handleKeyDown}
            />
            <button className="btn btn-primary" onClick={handleSend}>
                Send
            </button>
        </div>
    );
}

export default ChatInput;
