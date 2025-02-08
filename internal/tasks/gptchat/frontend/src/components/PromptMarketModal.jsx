import React, { useState, useContext } from 'react';
import { Modal, Button } from 'react-bootstrap';
import { ConfigContext } from '../context/ConfigContext'; // Import
import { storage } from '../utils/storage';


function PromptMarketModal({ show, onHide }) {
    const [title, setTitle] = useState('');
    const [description, setDescription] = useState('');
    const { config, updateConfig } = useContext(ConfigContext); // Access config

    // Sample prompts (Replace with your actual data loading)
    const [samplePrompts, setSamplePrompts] = useState([
        { title: "Prompt 1", description: "Description 1" },
        { title: "Prompt 2", description: "Description 2" },
    ]);

    const handleAddPrompt = async () => {
        if (!title.trim() || !description.trim()) {
            alert("Please enter the title & description.");
            return;
        }

        // update prompt short
        const newShortcut = { title, description };
        const shortcuts = await storage.getPromptShortcuts();
        shortcuts.push(newShortcut)
        await storage.setPromptShortcuts(shortcuts);

        // Clear input
        setTitle('');
        setDescription('');
        onHide(); //hide the modal.
    };

    const handleSelectPrompt = (promptDescription) => {
        // set system prompt
        updateConfig({ ...config, system_prompt: promptDescription });
        onHide();
    };


    return (
        <Modal show={show} onHide={onHide}>
            <Modal.Header closeButton>
                <Modal.Title>Prompt Market</Modal.Title>
            </Modal.Header>
            <Modal.Body>
                <h5>Add New Prompt</h5>
                <div className="mb-3">
                    <label htmlFor="promptTitle" className="form-label">Title:</label>
                    <input
                        type="text"
                        className="form-control"
                        id="promptTitle"
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                    />
                </div>
                <div className="mb-3">
                    <label htmlFor="promptDescription" className="form-label">Description:</label>
                    <textarea
                        className="form-control"
                        id="promptDescription"
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        rows="3"
                    />
                </div>
                <Button variant="primary" onClick={handleAddPrompt} className="mb-3">Add Prompt</Button>

                <h5>Available Prompts</h5>
                <div className="prompt-list">
                    {samplePrompts.map((prompt, index) => (
                        <Button
                            key={index}
                            variant="outline-secondary"
                            className="m-1"
                            onClick={() => handleSelectPrompt(prompt.description)}
                        >
                            {prompt.title}
                        </Button>
                    ))}
                </div>
            </Modal.Body>
            <Modal.Footer>
                <Button variant="secondary" onClick={onHide}>Close</Button>
            </Modal.Footer>
        </Modal>
    );
}

export default PromptMarketModal;
