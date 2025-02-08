import axios from 'axios';
import * as libs from './helpers';
import { storage } from './storage';

const apiBaseURL = '/api'; // Adjust base URL if necessary

export const api = {
    sendChat2Server: async (session, model, prompt, showAlert) => {
        try {
            const config = await storage.getChatSessionConfig(session);
            const accessToken = config.api_token;
            const headers = {
                Authorization: `Bearer ${accessToken}`,
                'Content-Type': 'application/json',
                // Add other necessary headers
            };

            const response = await axios.post(`${apiBaseURL}/chat`, {
                session,
                model,
                prompt,
                // Include other required parameters
            }, { headers });

            return response.data;  // Or handle based on the response structure

        } catch (error) {
            console.error("sendChat2Server API Error:", error);
            showAlert('danger', `API call failed: ${error.message}`);
            throw error; // Re-throw to be handled by the component
        }
    },
    // Define other API functions (image generation, QA, etc.)
};
