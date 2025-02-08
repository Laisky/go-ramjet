import * as libs from './helpers';

// Predefined KvKey Constants (from original code, adapted for React)
export const KvKeyPinnedMaterials = 'config_api_pinned_materials';
export const KvKeyAllowedModels = 'config_chat_models';
export const KvKeyCustomDatasetPassword = 'config_chat_dataset_key';
export const KvKeyPromptShortCuts = 'config_prompt_shortcuts';
export const KvKeyPrefixSessionHistory = 'chat_user_session_';
export const KvKeyPrefixSessionConfig = 'chat_user_config_';
export const KvKeyPrefixSelectedSession = 'config_selected_session';
export const KvKeySyncKey = 'config_sync_key';
// export const KvKeyAutoSyncUserConfig = 'config_auto_sync_user_config'; // If you add auto-sync
export const KvKeyVersionDate = 'config_version_date';
export const KvKeyUserInfo = 'config_user_info';
export const KvKeyChatData = 'chat_data_'; // ${KvKeyChatData}${role}_${chatID}

export const DefaultModel = 'gpt-4o-mini'; // Use a constant

export const storage = {
    getActiveSessionID: async () => {
        let activeSession = await libs.KvGet(KvKeyPrefixSelectedSession);
        if (!activeSession) {
            activeSession = 1;
            await libs.KvSet(KvKeyPrefixSelectedSession, activeSession);
        }
        return parseInt(activeSession);
    },

    setActiveSessionID: async (sessionId) => {
        await libs.KvSet(KvKeyPrefixSelectedSession, sessionId);
    },

    loadSessions: async () => {
        const allSessionKeys = [];
        (await libs.KvList()).forEach((key) => {
            if (key.startsWith(KvKeyPrefixSessionHistory)) {
                allSessionKeys.push(key);
            }
        });

        if (allSessionKeys.length === 0) {
            // Create a default session if none exist
            const skey = `${KvKeyPrefixSessionHistory}1`;
            allSessionKeys.push(skey);
            await libs.KvSet(skey, []);
            await libs.KvSet(`${KvKeyPrefixSessionConfig}1`, storage.newSessionConfig());
        }

        const sessions = await Promise.all(
            allSessionKeys.map(async (key) => {
                const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));
                const sconfig = await storage.getChatSessionConfig(sessionID);
                const sessionName = sconfig.session_name || sessionID;
                return { id: sessionID, name: sessionName };
            })
        );

        return sessions;
    },

    createNewSession: async () => {
        let maxSessionID = 0;
        (await libs.KvList()).forEach((key) => {
            if (key.startsWith(KvKeyPrefixSessionHistory)) {
                const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ""));
                if (sessionID > maxSessionID) {
                    maxSessionID = sessionID;
                }
            }
        });

        const newSessionID = maxSessionID + 1;
        const sessionKey = `${KvKeyPrefixSessionHistory}${newSessionID}`;
        await libs.KvSet(sessionKey, []);
        await libs.KvSet(`${KvKeyPrefixSessionConfig}${newSessionID}`, storage.newSessionConfig());
        return { id: newSessionID, name: newSessionID };
    },

    getChatSessionConfig: async (sessionId) => {
        const configKey = `${KvKeyPrefixSessionConfig}${sessionId}`;
        let config = await libs.KvGet(configKey);
        if (!config) {
            config = storage.newSessionConfig();
            await storage.saveChatSessionConfig(config, sessionId);
        }
        return config;
    },

    saveChatSessionConfig: async (config, sessionId) => {
        const configKey = `${KvKeyPrefixSessionConfig}${sessionId}`;
        await libs.KvSet(configKey, config);
    },

    newSessionConfig: () => {
        return {
            api_token: 'FREETIER-' + libs.RandomString(32),
            api_base: 'https://api.openai.com',
            max_tokens: 1000,
            temperature: 1,
            presence_penalty: 0,
            frequency_penalty: 0,
            n_contexts: 6,
            system_prompt: '# Core Capabilities and Behavior\n\nI am an AI assistant focused on being helpful, direct, and accurate. I aim to:\n\n- Provide factual responses about past events\n- Think through problems systematically step-by-step\n- Use clear, varied language without repetitive phrases\n- Give concise answers to simple questions while offering to elaborate if needed\n- Format code and text using proper Markdown\n- Engage in authentic conversation by asking relevant follow-up questions\n\n# Knowledge and Limitations \n\n- My knowledge cutoff is April 2024\n- I cannot open URLs or external links\n- I acknowledge uncertainty about very obscure topics\n- I note when citations may need verification\n- I aim to be accurate but may occasionally make mistakes\n\n# Task Handling\n\nI can assist with:\n- Analysis and research\n- Mathematics and coding\n- Creative writing and teaching\n- Question answering\n- Role-play and discussions\n\nFor sensitive topics, I:\n- Provide factual, educational information\n- Acknowledge risks when relevant\n- Default to legal interpretations\n- Avoid promoting harmful activities\n- Redirect harmful requests to constructive alternatives\n\n# Formatting Standards\n\nI use consistent Markdown formatting:\n- Headers with single space after #\n- Blank lines around sections\n- Consistent emphasis markers (* or _)\n- Proper list alignment and nesting\n- Clean code block formatting\n\n# Interaction Style\n\n- I am intellectually curious\n- I show empathy for human concerns\n- I vary my language naturally\n- I engage authentically without excessive caveats\n- I aim to be helpful while avoiding potential misuse',
            selected_model: DefaultModel,
            chat_switch: {
                all_in_one: false,
                disable_https_crawler: true,
                enable_google_search: false,
                enable_talk: false,
                draw_n_images: 1,
            },
        };
    },

    // Chat History Functions
    async sessionChatHistory(sessionID) {
        let data = await libs.KvGet(this.kvSessionKey(sessionID));
        return data || [];
    },

    async activeSessionChatHistory() {
        const sid = await this.getActiveSessionID();
        return await this.sessionChatHistory(sid);
    },

    kvSessionKey(sessionID) {
        return `${KvKeyPrefixSessionHistory}${parseInt(sessionID) || 1}`;
    },

    // Chat Data Functions
    async getChatData(chatID, role) {
        const key = `${KvKeyChatData}${role}_${chatID}`;
        return (await libs.KvGet(key)) || {};
    },

    async setChatData(chatID, role, chatData) {
        const key = `${KvKeyChatData}${role}_${chatID}`;
        await libs.KvSet(key, chatData);
    },

    async updateChatData(chatID, role, partialChatData) {
        let chatData = await this.getChatData(chatID, role);
        if (typeof chatData !== 'object' || chatData === null) {
            chatData = { chatID, role };
        }
        Object.keys(partialChatData).forEach((key) => {
            if (partialChatData[key] !== undefined && partialChatData[key] !== null) {
                chatData[key] = partialChatData[key];
            }
        });
        await this.setChatData(chatID, role, chatData);
        return chatData;
    },

    async saveChats2Storage(chatData) {
        if (!chatData.chatID) {
            throw new Error('chatID is required');
        }

        chatData = await this.updateChatData(chatData.chatID, chatData.role, chatData);

        const storageActiveSessionKey = this.kvSessionKey(await this.getActiveSessionID());
        const session = await this.activeSessionChatHistory();

        let found = false;
        session.forEach((item) => {
            if (item.chatID === chatData.chatID && item.role === chatData.role) {
                found = true;
            }
        });

        if (!found && chatData.role === 'assistant') {
            session.forEach((item, idx) => {
                if (item.chatID === chatData.chatID) {
                    found = true;
                    if (item.role !== 'assistant') {
                        session.splice(idx + 1, 0, {
                            role: 'assistant',
                            chatID: chatData.chatID
                        });
                    }
                }
            });
        }

        if (!found) {
            session.push({
                role: chatData.role,
                chatID: chatData.chatID
            });
        }

        await this.setChatData(chatData.chatID, chatData.role, chatData);
        await libs.KvSet(storageActiveSessionKey, session);
    },
    async removeChatInStorage(chatid) {
        if (!chatid) {
            throw new Error('chatid is required');
        }

        const storageActiveSessionKey = this.kvSessionKey(await this.getActiveSessionID());
        let session = await this.activeSessionChatHistory();

        session = session.filter((item) => !(item.chatID === chatid));

        await libs.KvSet(storageActiveSessionKey, session);
    },
    async clearSessionAndChats(sessionID) {
        // Remove pinned materials (if you're storing them in context now)
        await libs.KvDel(KvKeyPinnedMaterials);

        if (!sessionID) {
            // Remove all sessions
            const sconfig = await this.getChatSessionConfig();

            await Promise.all((await libs.KvList()).map(async (key) => {
                if (
                    key.startsWith(KvKeyPrefixSessionHistory) ||
                    key.startsWith(KvKeyPrefixSessionConfig)
                ) {
                    await libs.KvDel(key);
                }
            }));

            // restore default session config
            await libs.KvSet(`${KvKeyPrefixSessionConfig}1`, sconfig);
            await libs.KvSet(this.kvSessionKey(1), []);
        } else {
            // Remove specific session
            await libs.KvSet(this.kvSessionKey(sessionID), []);
        }
        // Consider what you want to do on session clear (reload, redirect, etc.)
    },


    // Add other storage methods as needed (e.g., for pinned materials, prompt shortcuts, etc.)
    async getPinnedMaterials() {
        return await libs.KvGet(KvKeyPinnedMaterials) || ''; // Return empty string if null
    },
    async setPinnedMaterials(materials) {
        await libs.KvSet(KvKeyPinnedMaterials, materials);
    },
    async getPromptShortcuts() {
        let shortcuts = await libs.KvGet(KvKeyPromptShortCuts);
        if (!shortcuts) {
            shortcuts = [
                {
                    title: '中英互译',
                    description: 'As an English-Chinese translator, your task is to accurately translate text...' // Your default prompt
                }
            ];
            await libs.KvSet(KvKeyPromptShortCuts, shortcuts);
        }
        return shortcuts;
    },
    async setPromptShortcuts(shortcuts) {
        await libs.KvSet(KvKeyPromptShortCuts, shortcuts);
    },
    async getCustomDatasetPassword() {
        return await libs.KvGet(KvKeyCustomDatasetPassword) || '';
    },
    async setCustomDatasetPassword(password) {
        await libs.KvSet(KvKeyCustomDatasetPassword, password);
    },
};
