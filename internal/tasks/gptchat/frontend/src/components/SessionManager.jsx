import React, { useContext } from 'react';
import { ChatContext } from '../context/ChatContext'; //For switching session
import { ConfigContext } from '../context/ConfigContext';
import './SessionManager.css';

function SessionManager({ sessions, onSessionCreate, onSessionSwitch }) {
    const { selectedSession, setSelectedSession } = useContext(ChatContext);
    const { config, setConfig } = useContext(ConfigContext);

    const handleSessionClick = async (sessionId) => {
        onSessionSwitch(sessionId)
    };

    return (
        <div className="session-manager">
            <h3>Sessions</h3>
            <ul className="list-group">
                {sessions.map((session) => (
                    <li
                        key={session.id}
                        className={`list-group-item ${session.id === selectedSession ? 'active' : ''}`}
                        onClick={() => handleSessionClick(session.id)}
                    >
                        {session.name}
                    </li>
                ))}
            </ul>
            <button className="btn btn-primary" onClick={onSessionCreate}>
                New Session
            </button>
        </div>
    );
}

export default SessionManager;
