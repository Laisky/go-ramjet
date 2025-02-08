import React, { createContext, useState, useEffect, useCallback } from 'react';
import { storage } from '../utils/storage';
import { api } from '../utils/api';

const ChatContext = createContext();

const ChatProvider = ({ children }) => {
    const [chatHistory, setChatHistory] = useState([]);
    const [selectedSession, setSelectedSession] = useState(null);
    const [loading, setLoading] = useState(false);  // For loading indicators

    const loadChatHistory = useCallback(async (sessionId) => {
        setLoading(true);
        try {
            const history = await storage.sessionChatHistory(sessionId);
            // Map the simplified history to include full chat data
            const fullHistory = await Promise.all(
                history.map(async (item) => {
                    return await storage.getChatData(item.chatID, item.role);
                })
            );

            setChatHistory(fullHistory);
        } catch (error) {
            console.error("Error loading chat history:", error);
            // Consider showing an error message to the user
        } finally {
            setLoading(false);
        }
    }, []); // Add any dependencies if needed


    useEffect(() => {
        if (selectedSession) {
            loadChatHistory(selectedSession);
        }
    }, [selectedSession, loadChatHistory]);

    const sendMessage = async (prompt, model) => {
        setLoading(true);
        try {
            const newChatID = `chat-${Date.now()}-${libs.RandomString(6)}`;

            // Optimistically add user message to chat history
            const userMessage = {
                chatID: newChatID,
                role: 'user',
                content: prompt,
                model: model
            };

            setChatHistory(prevHistory => [...prevHistory, userMessage]);
            await storage.saveChats2Storage(userMessage);

            // Send message to the server and get AI response
            // Use `selectedSession` if you need it for the API call
            // Assuming api.sendChat2Server now just returns data
            const aiResponseData = await api.sendChat2Server(selectedSession, model, prompt, () => { }); // Replace with actual API call

            // Create a new chat object for the AI response based on data from sendChat2Server.
            const aiResponse = {
                chatID: newChatID, // Use same chatID as user message
                role: 'assistant',  // Corrected role
                content: aiResponseData.content,  // Assuming your API returns a 'content' field
                ...aiResponseData,    // Spread other properties from response (if applicable)
            };

            setChatHistory(prevHistory => [...prevHistory, aiResponse]); //Add to display
            await storage.saveChats2Storage(aiResponse);   // Save to Storage


        } catch (error) {
            console.error('Error sending message:', error);
            // Handle error (e.g., show an error message)
        } finally {
            setLoading(false);
        }
    };

    const clearChat = async (sessionId) => {
        try {
            await storage.clearSessionAndChats(sessionId);
            await loadChatHistory(sessionId); // Reload empty history
        } catch (error) {
            console.error("Error clearing chat:", error);
            // Handle the error (e.g., display an error message)
        }
    };

    const value = {
        chatHistory,
        selectedSession,
        setSelectedSession,
        sendMessage,
        loading,
        clearChat,
        loadChatHistory
    };

    return <ChatContext.Provider value={value}>{children}</ChatContext.Provider>;
};

export { ChatContext, ChatProvider };
