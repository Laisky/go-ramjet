import React, { createContext, useState, useEffect, useCallback } from 'react';
import { storage } from '../utils/storage';

const ConfigContext = createContext();

const ConfigProvider = ({ children }) => {
    const [config, setConfig] = useState(storage.newSessionConfig());
    const [selectedSession, setSelectedSession] = useState(null); // To track which session's config we're editing

    const loadConfig = useCallback(async (sessionId) => {
        try {
            const loadedConfig = await storage.getChatSessionConfig(sessionId);
            setConfig(loadedConfig);

        } catch (error) {
            console.error("Error loading config:", error);
            // Handle error (e.g., show a message to user)
        }
    }, []);

    useEffect(() => {
        // Load the initial configuration when the component mounts or selectedSession changes
        const loadInitialConfig = async () => {
            const session = await storage.getActiveSessionID()
            setSelectedSession(session);
            await loadConfig(session);
        };
        loadInitialConfig();

    }, [loadConfig]);

    useEffect(() => {
        if (selectedSession) {
            loadConfig(selectedSession)
        }
    }, [selectedSession, loadConfig]);

    const updateConfig = async (newConfig) => {
        try {
            await storage.saveChatSessionConfig(newConfig, selectedSession);
            setConfig(newConfig); // Update local state
        } catch (error) {
            console.error("Error updating config:", error);
            // Handle the error (e.g., display an alert)
        }
    };

    const value = {
        config,
        updateConfig, // Expose a function to update the config
        selectedSession,  // Might be useful to have the selected session ID here
        setSelectedSession // And a way to change it
    };

    return <ConfigContext.Provider value={value}>{children}</ConfigContext.Provider>;
};

export { ConfigContext, ConfigProvider };
