import React from 'react';
import { Modal } from 'react-bootstrap';

function DropfileModal({ show, onHide }) {
    return (
        <Modal show={show} onHide={onHide} centered>
            <Modal.Body>
                <div className="text-center">
                    <p>Drop files here</p>
                </div>
            </Modal.Body>
        </Modal>
    );
}

export default DropfileModal;
