import React, { useState, useContext } from 'react';
import { Modal, Button, Form } from 'react-bootstrap';
import { storage } from '../utils/storage';
import { api } from '../utils/api';
import { ConfigContext } from '../context/ConfigContext';


function PrivateDatasetModal({ show, onHide }) {
    const [dataKey, setDataKey] = useState('');
    const [datasetName, setDatasetName] = useState('');
    const [selectedFile, setSelectedFile] = useState(null);
    const [datasets, setDatasets] = useState([]);  // To store dataset list
    const { config, updateConfig } = useContext(ConfigContext);


    // Load data key on modal show (or when it changes)
    React.useEffect(() => {
        const loadDataKey = async () => {
            const key = await storage.getCustomDatasetPassword();
            setDataKey(key || ''); // Ensure it's never null
        };
        if (show) {
            loadDataKey();
        }
    }, [show]);

    const handleDataKeyChange = async (e) => {
        const newKey = e.target.value;
        setDataKey(newKey);
        await storage.setCustomDatasetPassword(newKey); // Persist immediately
    };

    const handleFileChange = (e) => {
        if (e.target.files.length > 0) {
            setSelectedFile(e.target.files[0]);

            // Extract dataset name
            let filename = e.target.files[0].name;
            const fileext = filename.substring(filename.lastIndexOf('.')).toLowerCase();

            if (['.pdf', '.md', '.ppt', '.pptx', '.doc', '.docx'].indexOf(fileext) === -1) {
                alert('Unsupported file type. Supported types: .pdf, .md, .ppt, .pptx, .doc, .docx');
                e.target.value = ''; // Clear file input
                setSelectedFile(null);
                setDatasetName(''); // Clear dataset name as well.
                return
            }

            filename = filename.substring(0, filename.lastIndexOf('.'));
            filename = filename.replace(/[^a-zA-Z0-9]/g, '_');
            setDatasetName(filename)
        }

    };


    const handleUpload = async () => {
        if (!selectedFile) {
            alert("Please choose a file.");
            return;
        }

        if (!datasetName.trim()) {
            alert("Please input the dataset name.");
            return;
        }

        const formData = new FormData();
        formData.append('file', selectedFile);
        formData.append('file_key', datasetName);
        formData.append('data_key', dataKey);

        // Use api instead of fetch
        try {
            // call api to upload dataset
            console.log(`upload dataset: ${datasetName}`)
        } catch (error) {
            console.error("Error upload dataset:", error);
            // Handle the error (e.g., display an alert)
        }

    };

    const handleListDatasets = async () => {
        try {
            // call api to upload dataset
            // const data = await api.fetchDataset(dataKey, config.api_token);
            const data = { datasets: [], selected: [] } //TODO Remove after api implemented
            setDatasets(data.datasets); // Update local state for display
            console.log(`fetch dataset list: ${JSON.stringify(data)}`)
        } catch (error) {
            console.error("Error list dataset:", error);
            // Handle the error (e.g., display an alert)
        }
    };


    const handleDeleteDataset = async (datasetName) => {
        // Placeholder for delete dataset logic.  You'll need to implement
        // the actual API call and update the `datasets` state.
        console.log("Deleting dataset:", datasetName);
        alert(`Deleting dataset (not implemented): ${datasetName}`)
    };


    return (
        <Modal show={show} onHide={onHide} size="lg">
            <Modal.Header closeButton>
                <Modal.Title>Private Dataset Management</Modal.Title>
            </Modal.Header>
            <Modal.Body>
                <Form>
                    <Form.Group className="mb-3">
                        <Form.Label>Data Key:</Form.Label>
                        <Form.Control type="text" value={dataKey} onChange={handleDataKeyChange} />
                    </Form.Group>

                    <Form.Group className="mb-3">
                        <Form.Label>Dataset Name:</Form.Label>
                        <Form.Control type="text" value={datasetName} onChange={(e) => setDatasetName(e.target.value)} />
                    </Form.Group>

                    <Form.Group className="mb-3">
                        <Form.Label>Upload File:</Form.Label>
                        <Form.Control type="file" onChange={handleFileChange} />
                    </Form.Group>

                    <Button variant="primary" onClick={handleUpload} className="me-2">Upload</Button>
                    <Button variant="secondary" onClick={handleListDatasets}>List Datasets</Button>

                    <div className="mt-3">
                        <h5>Datasets:</h5>
                        {datasets.length > 0 ? (
                            <ul className="list-group">
                                {datasets.map((dataset, index) => (
                                    <li key={index} className="list-group-item d-flex justify-content-between align-items-center">
                                        {dataset.name}
                                        <Button variant="danger" size="sm" onClick={() => handleDeleteDataset(dataset.name)}>
                                            Delete
                                        </Button>
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <p>No datasets found.</p>
                        )}
                    </div>

                </Form>
            </Modal.Body>
            <Modal.Footer>
                <Button variant="secondary" onClick={onHide}>Close</Button>
            </Modal.Footer>
        </Modal>
    );
}

export default PrivateDatasetModal;
