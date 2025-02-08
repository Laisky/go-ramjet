import React from 'react';
import './Alert.css'

function Alert({ type, message, onClose }) {
    return (
        <div className={`alert alert-${type} alert-dismissible fade show`} role="alert">
            {message}
            <button type="button" className="btn-close" aria-label="Close" onClick={onClose}></button>
        </div>
    );
}

export default Alert;
