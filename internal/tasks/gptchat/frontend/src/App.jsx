import React, { useState, useEffect } from 'react';
import ChatInterface from './components/ChatInterface';
import SessionManager from './components/SessionManager';
import ConfigurationSidebar from './components/ConfigurationSidebar';
import Alert from './components/Alert';
import { ChatContext } from './context/ChatContext';
import { ConfigContext } from './context/ConfigContext';
import { api } from './utils/api'; // Import your API functions
import { storage } from './utils/storage'; // Import storage functions
import './App.css';
import 'bootstrap/dist/css/bootstrap.min.css'; // Import Bootstrap CSS
import * as libs from './utils/helpers';

function App() {
    const [alerts, setAlerts] = useState([]);
    const [selectedSession, setSelectedSession] = useState(null);  // Use null or an initial session ID
    const [sessions, setSessions] = useState([]);
    const [config, setConfig] = useState({});

    // Function to show alerts
    const showAlert = (type, message) => {
        setAlerts([...alerts, { type, message, id: Date.now() }]);
    };

    // Function to close alerts
    const closeAlert = (id) => {
        setAlerts(alerts.filter(alert => alert.id !== id));
    };

    // Load initial session
    useEffect(() => {
        const loadInitialSession = async () => {
            try {
                const activeSession = await storage.getActiveSessionID();
                setSelectedSession(activeSession);

                // Load sessions
                const sessionsData = await storage.loadSessions();
                setSessions(sessionsData);

                // Load configurations
                const configData = await storage.getChatSessionConfig(activeSession)
                setConfig(configData);

            } catch (error) {
                showAlert('danger', `Failed to load initial data: ${error.message}`);
            }
        };
        loadInitialSession();
    }, []);

    const createNewSession = async () => {
        try {
            const newSession = await storage.createNewSession();
            setSessions([...sessions, newSession]);
            setSelectedSession(newSession.id);

            // Update config with new session
            const configData = await storage.getChatSessionConfig(newSession.id)
            setConfig(configData);

        } catch (error) {
            showAlert('danger', `Failed to create new session: ${error.message}`);
        }
    };

    const switchSession = async (sessionId) => {
        try {
            await storage.setActiveSessionID(sessionId);
            setSelectedSession(sessionId);

            // Load session configuration
            const configData = await storage.getChatSessionConfig(sessionId);
            setConfig(configData);

        } catch (error) {
            showAlert('danger', `Failed to switch session: ${error.message}`);
        }
    };

    //Provide Chat Context
    const chatContextValue = {
        selectedSession,
        sendMessage: async (prompt) => {
            try {
                await api.sendChat2Server(selectedSession, config.selected_model, prompt, showAlert);
                // update chatlog
            } catch (error) {
                showAlert('danger', `Send message failed: ${error.message}`);
            }
        },  // Simplified for the example
    };

    const configContextValue = {
        config,
        setConfig: async (newConfig) => {
            try {
                await storage.saveChatSessionConfig(newConfig, selectedSession);
                setConfig(newConfig); // Update the local state
            } catch (error) {
                showAlert('danger', `Failed to save config: ${error.message}`);
            }
        },
        showAlert // Pass showAlert to ConfigContext
    };

    return (
        <ChatContext.Provider value={chatContextValue}>
            <ConfigContext.Provider value={configContextValue}>
                <div className="app-container">
                    <header className="app-header">
                        {/* Your header content */}
                        <h1>My React Chat App</h1>
                    </header>
                    <SessionManager
                        sessions={sessions}
                        selectedSession={selectedSession}
                        onSessionCreate={createNewSession}
                        onSessionSwitch={switchSession}
                    />
                    <ChatInterface showAlert={showAlert} />
                    <ConfigurationSidebar showAlert={showAlert} />

                    <div className="alerts-container">
                        {alerts.map(alert => (
                            <Alert
                                key={alert.id}
                                type={alert.type}
                                message={alert.message}
                                onClose={() => closeAlert(alert.id)}
                            />
                        ))}
                    </div>
                </div>
            </ConfigContext.Provider>
        </ChatContext.Provider>
    );
}

export default App;
