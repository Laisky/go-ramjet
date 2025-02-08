import React, { useContext } from 'react';
import { ConfigContext } from '../context/ConfigContext';
import './ConfigurationSidebar.css'

function ConfigurationSidebar({ showAlert }) {
    const { config, updateConfig } = useContext(ConfigContext);

    const handleInputChange = (e) => {
        const { name, value } = e.target;
        updateConfig({ ...config, [name]: value });
    };

    const handleCheckboxChange = (e) => {
        const { name, checked } = e.target;
        const [section, key] = name.split('.'); // For nested properties (e.g., "chat_switch.all_in_one")
        if (section && key) {
            // update for nested element
            updateConfig({
                ...config,
                [section]: {
                    ...config[section],
                    [key]: checked
                }
            });
        }
    };

    return (
        <div className="configuration-sidebar">
            <h4>Configurations</h4>
            <div className="mb-3">
                <label htmlFor="apiToken" className="form-label">API Token:</label>
                <input
                    type="text"
                    className="form-control"
                    id="apiToken"
                    name="api_token"
                    value={config.api_token || ''}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label htmlFor="apiBase" className="form-label">API Base:</label>
                <input
                    type="text"
                    className="form-control"
                    id="apiBase"
                    name="api_base"
                    value={config.api_base || ''}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label className="form-label">Contexts: {config.n_contexts}</label>
                <input
                    type="range"
                    className="form-range"
                    min="1"
                    max="30"
                    step="1"
                    name="n_contexts"
                    value={config.n_contexts}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label className="form-label">Max Tokens: {config.max_tokens}</label>
                <input
                    type="range"
                    className="form-range"
                    min="100"
                    max="16384"
                    step="100"
                    name="max_tokens"
                    value={config.max_tokens}
                    onChange={handleInputChange}
                />
            </div>

            <div className="mb-3">
                <label className="form-label">Temperature: {config.temperature}</label>
                <input
                    type="range"
                    className="form-range"
                    min="0"
                    max="2"
                    step="0.1"
                    name="temperature"
                    value={config.temperature}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label className="form-label">Presence Penalty: {config.presence_penalty}</label>
                <input
                    type="range"
                    className="form-range"
                    min="-2"
                    max="2"
                    step="0.1"
                    name="presence_penalty"
                    value={config.presence_penalty}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label className="form-label">Frequency Penalty: {config.frequency_penalty}</label>
                <input
                    type="range"
                    className="form-range"
                    min="-2"
                    max="2"
                    step="0.1"
                    name="frequency_penalty"
                    value={config.frequency_penalty}
                    onChange={handleInputChange}
                />
            </div>
            <div className="mb-3">
                <label htmlFor="systemPrompt" className="form-label">System Prompt:</label>
                <textarea
                    className="form-control"
                    id="systemPrompt"
                    name="system_prompt"
                    value={config.system_prompt || ''}
                    onChange={handleInputChange}
                    rows="3"
                />
            </div>

            <div className="mb-3 form-check">
                <input
                    type="checkbox"
                    className="form-check-input"
                    id="allInOne"
                    name="chat_switch.all_in_one"
                    checked={config.chat_switch?.all_in_one || false}
                    onChange={handleCheckboxChange}
                />
                <label className="form-check-label" htmlFor="allInOne">All-in-One</label>
            </div>

            <div className="mb-3 form-check">
                <input
                    type="checkbox"
                    className="form-check-input"
                    id="disableHttpsCrawler"
                    name="chat_switch.disable_https_crawler"
                    checked={config.chat_switch?.disable_https_crawler || false}
                    onChange={handleCheckboxChange}
                />
                <label className="form-check-label" htmlFor="disableHttpsCrawler">Disable HTTPS Crawler</label>
            </div>

            <div className="mb-3 form-check">
                <input
                    type="checkbox"
                    className="form-check-input"
                    id="enableGoogleSearch"
                    name="chat_switch.enable_google_search"
                    checked={config.chat_switch?.enable_google_search || false}
                    onChange={handleCheckboxChange}
                />
                <label className="form-check-label" htmlFor="enableGoogleSearch">Enable Google Search</label>
            </div>
            <div className="mb-3 form-check">
                <input
                    type="checkbox"
                    className="form-check-input"
                    id="enableTalk"
                    name="chat_switch.enable_talk"
                    checked={config.chat_switch?.enable_talk || false}
                    onChange={handleCheckboxChange}
                />
                <label className="form-check-label" htmlFor="enableTalk">Enable Talk</label>
            </div>

            <div className="mb-3">
                <label htmlFor="drawNImage" className="form-label">Number Images Draw: </label>
                <input
                    type="number"
                    className="form-control"
                    id="drawNImage"
                    name="chat_switch.draw_n_images"
                    value={config.chat_switch?.draw_n_images}
                    onChange={handleInputChange}
                    min="1"
                    max="4"
                />
            </div>

            {/* Add other configuration options as needed */}
        </div>
    );
}

export default ConfigurationSidebar;
