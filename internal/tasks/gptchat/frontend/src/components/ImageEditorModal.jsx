import React, { useState, useRef, useEffect } from 'react';
import { Modal, Button } from 'react-bootstrap'; // Using react-bootstrap

function ImageEditorModal({ show, onHide, imgSrc, onSave }) {
    const [prompt, setPrompt] = useState('');
    const canvasRef = useRef(null);
    const ctxRef = useRef(null); // Store the 2D context
    const isDrawingRef = useRef(false); // Keep track of drawing state


    useEffect(() => {
        if (show && imgSrc) {
            const canvas = canvasRef.current;
            ctxRef.current = canvas.getContext('2d');
            const img = new Image();
            img.crossOrigin = 'anonymous';
            img.onload = () => {
                if (img.naturalWidth !== img.naturalHeight) {
                    alert('Image must be square'); //basic check, can be improved.
                    onHide(); // Close if not square
                    return;
                }
                canvas.width = img.naturalWidth;
                canvas.height = img.naturalHeight;
                ctxRef.current.drawImage(img, 0, 0);
            };
            img.onerror = () => {
                alert('Error loading image');
                onHide();
            };
            img.src = imgSrc;
        }
    }, [show, imgSrc, onHide]);

    const getMousePos = (canvas, evt) => {
        const rect = canvas.getBoundingClientRect();
        return {
            x: evt.clientX - rect.left,
            y: evt.clientY - rect.top
        };
    };

    const startDrawing = (e) => {
        e.preventDefault();
        isDrawingRef.current = true;
    };

    const draw = (e) => {
        e.preventDefault();
        if (!isDrawingRef.current) return;
        const canvas = canvasRef.current;
        const ctx = ctxRef.current; // Access the context
        const { x, y } = getMousePos(canvas, e);

        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';
        ctx.strokeStyle = 'rgba(255, 255, 0, 1)';
        ctx.lineWidth = 40;
        ctx.lineTo(x, y);
        ctx.stroke();
    };

    const stopDrawing = (e) => {
        e.preventDefault();
        isDrawingRef.current = false;
        if (ctxRef.current) {
            ctxRef.current.beginPath(); // Start new path on mouse up
        }
    };

    const handleSave = async () => {
        if (!prompt.trim()) {
            alert('Please enter a prompt'); //Basic check. Use showAlert as in prev. components
            return;
        }

        //Get mask -- Code almost identical to the original, adapted for useRef
        const canvas = canvasRef.current;
        const ctx = ctxRef.current
        const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const maskData = new Uint8ClampedArray(imageData.data.length);

        for (let i = 0; i < imageData.data.length; i += 1) {
            maskData[i] = imageData.data[i];
        }

        for (let i = 0; i < imageData.data.length; i += 4) {
            if (imageData.data[i] >= 250 && imageData.data[i + 1] >= 250 && imageData.data[i + 2] <= 5) {
                maskData[i] = 255;
                maskData[i + 1] = 255;
                maskData[i + 2] = 255;
            } else {
                maskData[i] = 0;
                maskData[i + 1] = 0;
                maskData[i + 2] = 0;
            }
        }

        const maskCanvas = document.createElement('canvas');
        maskCanvas.width = canvas.width;
        maskCanvas.height = canvas.height;
        const maskCtx = maskCanvas.getContext('2d');
        const maskImageData = new ImageData(maskData, canvas.width, canvas.height);
        maskCtx.putImageData(maskImageData, 0, 0);

        const maskBlob = await new Promise((resolve) => {
            maskCanvas.toBlob(async (blob) => {
                resolve(blob);
            });
        });

        const rawImgBlob = await (await fetch(imgSrc)).blob();

        onSave(prompt, rawImgBlob, maskBlob); // Call the onSave callback
        onHide(); // Close modal on save

    }
    return (
        <Modal show={show} onHide={onHide} size="lg">
            <Modal.Header closeButton>
                <Modal.Title>Image Editor</Modal.Title>
            </Modal.Header>
            <Modal.Body>
                <div className="mb-3">
                    <label htmlFor="editPrompt" className="form-label">Prompt:</label>
                    <input
                        type="text"
                        className="form-control"
                        id="editPrompt"
                        value={prompt}
                        onChange={(e) => setPrompt(e.target.value)}
                    />
                </div>
                <canvas
                    ref={canvasRef}
                    style={{ width: '100%', border: '1px solid #000' }}
                    onMouseDown={startDrawing}
                    onMouseMove={draw}
                    onMouseUp={stopDrawing}
                    onMouseLeave={stopDrawing}  // Stop drawing if mouse leaves canvas
                    onTouchStart={startDrawing}  // For touch devices
                    onTouchMove={draw}
                    onTouchEnd={stopDrawing}
                />
            </Modal.Body>
            <Modal.Footer>
                <Button variant="secondary" onClick={onHide}>
                    Cancel
                </Button>
                <Button variant="primary" onClick={handleSave}>
                    Save Changes
                </Button>
            </Modal.Footer>
        </Modal>
    );
}

export default ImageEditorModal;
