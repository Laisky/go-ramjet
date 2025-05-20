'use strict';

const libs = window.libs;

const robotIcon = '🤖️';

// const ChatModelTurbo35 = 'gpt-3.5-turbo';
// const ChatModelTurbo35V1106 = 'gpt-3.5-turbo-1106';
// const ChatModelTurbo35V0125 = 'gpt-3.5-turbo-0125';
// const ChatModelTurbo35_16K = "gpt-3.5-turbo-16k";
// const ChatModelTurbo35_0613 = "gpt-3.5-turbo-0613";
// const ChatModelTurbo35_0613_16K = "gpt-3.5-turbo-16k-0613";
// const ChatModelGPT4 = "gpt-4";
const ChatModelGPT4Turbo = 'gpt-4-turbo';
const ChatModelGPT4O = 'gpt-4o';
const ChatModelGPT4OSearch = 'gpt-4o-search-preview';
const ChatModelGPT4OMini = 'gpt-4o-mini';
const ChatModelGPT4OMiniSearch = 'gpt-4o-mini-search-preview';
// const ChatModelGPTO1Preview = 'o1-preview';
const ChatModelGPTO1 = 'o1';
// const ChatModelGPTO1Mini = 'o1-mini';
const ChatModelGPTO3 = 'o3';
const ChatModelGPTO3Mini = 'o3-mini';
const ChatModelGPTO4Mini = 'o4-mini';
const ChatModelDeepSeekChat = 'deepseek-chat';
const ChatModelDeepSeekResoner = 'deepseek-reasoner';
// const ChatModelDeepSeekCoder = 'deepseek-coder';
// const ChatModelGPT4Turbo1106 = 'gpt-4-1106-preview';
// const ChatModelGPT4Turbo0125 = 'gpt-4-0125-preview';
// const ChatModelGPT4Vision = 'gpt-4-vision-preview';
// const ChatModelClaude1 = 'claude-instant-1';
// const ChatModelClaude2 = 'claude-2';
const ChatModelClaude3Opus = 'claude-3-opus';
// const ChatModelClaude3Sonnet = 'claude-3-sonnet';
const ChatModelClaude35Sonnet = 'claude-3.5-sonnet';
const ChatModelClaude37Sonnet = 'claude-3.7-sonnet';
const ChatModelClaude37SonnetThinking = 'claude-3.7-sonnet-thinking';
// const ChatModelClaude35Sonnet8K = 'claude-3.5-sonnet-8k';
// const ChatModelClaude3Haiku = 'claude-3-haiku';
const ChatModelClaude35Haiku = 'claude-3.5-haiku';
// const ChatModelGPT4_0613 = "gpt-4-0613";
// const ChatModelGPT4_32K = "gpt-4-32k";
// const ChatModelGPT4_0613_32K = "gpt-4-32k-0613";
const ChatModelGemini2Pro = 'gemini-2.0-pro';
const ChatModelGemini25Pro = 'gemini-2.5-pro';
// const ChatModelGeminiProVision = 'gemini-pro-vision';
const ChatModelGemini25Flash = 'gemini-2.5-flash';
// const ChatModelGemini2Flash = 'gemini-2.0-flash';
const ChatModelGemini2FlashThinking = 'gemini-2.0-flash-thinking';
const ChatModelGemini2FlahExpImage = 'gemini-2.0-flash-exp-image-generation';
// const ChatModelGroqLlama2With70B4K = 'llama2-70b-4096';
// const ChatModelGroqMixtral8x7B32K = 'mixtral-8x7b-32768';
// const ChatModelGroqGemma2With9B = 'gemma2-9b-it';
const ChatModelGroqGemma3With27B = 'gemma-3-27b-it';
const ChatModelDeepResearch = 'deep-research';
// const ChatModelGroqllama3With8B = 'llama-3.1-8b-instant';
const ChatModelGroqllama3With70B = 'llama-3.3-70b-versatile';
const ChatModelQwenQwq32B = 'qwen-qwq-32b';
// const ChatModelGroqllama3With405B = 'llama-3.1-405b-instruct';
const QAModelBasebit = 'qa-bbt-xego';
const QAModelSecurity = 'qa-security';
const QAModelImmigrate = 'qa-immigrate';
const QAModelCustom = 'qa-custom';
const QAModelShared = 'qa-shared';
const CompletionModelDavinci3 = 'text-davinci-003';
// const ImageModelDalle2 = 'dall-e-2';
const ImageModelDalle3 = 'dall-e-3';
const ImageModelGptImage1 = 'gpt-image-1';
const ImageModelSdxlTurbo = 'sdxl-turbo';
// const ImageModelFluxPro = 'flux-pro';
const ImageModelFluxDev = 'flux-dev';
const ImageModelFluxPro11 = 'flux-1.1-pro';
const ImageModelFluxProUltra11 = 'flux-1.1-pro-ultra';
const ImageModelFluxSchnell = 'flux-schnell';
const ImageModelImagen3 = 'imagen-3.0';
const ImageModelImagen3Fast = 'imagen-3.0-fast';
// const ImageModelImg2Img = 'img-to-img';

const DefaultModel = ChatModelGPT4OMini;

const ChatTaskTypeChat = 'chat';
const ChatTaskTypeImage = 'image';
const ChatTaskTypeDeepResearch = 'deepresearch';

const ChatTaskStatusWaiting = 'waiting';
const ChatTaskStatusProcessing = 'processing';
const ChatTaskStatusDone = 'done';

// casual chat models

const ChatModels = [
    ChatModelDeepResearch,
    // ChatModelTurbo35,
    // ChatModelTurbo35V1106,
    // ChatModelTurbo35V0125,
    // ChatModelGPT4,
    ChatModelGPT4Turbo,
    ChatModelGPT4O,
    ChatModelGPT4OSearch,
    ChatModelGPT4OMini,
    ChatModelGPT4OMiniSearch,
    // ChatModelGPTO1Preview,
    ChatModelGPTO1,
    ChatModelGPTO3,
    // ChatModelGPTO1Mini,
    ChatModelGPTO3Mini,
    ChatModelGPTO4Mini,
    ChatModelDeepSeekChat,
    ChatModelDeepSeekResoner,
    // ChatModelDeepSeekCoder,
    // ChatModelGPT4Turbo1106,
    // ChatModelGPT4Turbo0125,
    // ChatModelClaude1,
    // ChatModelClaude2,
    ChatModelClaude3Opus,
    ChatModelClaude35Sonnet,
    ChatModelClaude37Sonnet,
    ChatModelClaude37SonnetThinking,
    // ChatModelClaude35Sonnet8K,
    // ChatModelClaude3Haiku,
    ChatModelClaude35Haiku,
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    // ChatModelGroqGemma2With9B,
    ChatModelGroqGemma3With27B,
    ChatModelGroqllama3With70B,
    ChatModelQwenQwq32B,
    // ChatModelGroqllama3With8B,
    // ChatModelGroqllama3With405B,
    // ChatModelGPT4Vision,
    // ChatModelGeminiPro,
    // ChatModelGeminiProVision,
    ChatModelGemini2Pro,
    ChatModelGemini25Pro,
    // ChatModelGemini2Flash,
    ChatModelGemini25Flash,
    ChatModelGemini2FlashThinking,
    ChatModelGemini2FlahExpImage
    // ChatModelTurbo35_16K,
    // ChatModelTurbo35_0613,
    // ChatModelTurbo35_0613_16K,
    // ChatModelGPT4_0613,
    // ChatModelGPT4_32K,
    // ChatModelGPT4_0613_32K,
];
const VisionModels = [
    ChatModelGPT4Turbo,
    ChatModelGPT4O,
    ChatModelGPT4OSearch,
    ChatModelGPT4OMini,
    ChatModelGPT4OMiniSearch,
    // ChatModelGeminiProVision,
    ChatModelGemini2Pro,
    ChatModelGemini25Pro,
    // ChatModelGemini2Flash,
    ChatModelGemini25Flash,
    ChatModelGemini2FlashThinking,
    ChatModelGemini2FlahExpImage,
    ChatModelClaude3Opus,
    ChatModelClaude35Sonnet,
    ChatModelClaude37Sonnet,
    ChatModelClaude37SonnetThinking,
    // ChatModelClaude35Sonnet8K,
    // ChatModelClaude3Haiku,
    ChatModelClaude35Haiku,
    // ChatModelGPTO1Preview,
    ChatModelGPTO1,
    ChatModelGPTO3,
    // ImageModelSdxlTurbo,
    // ImageModelImg2Img
    // ImageModelFluxPro,
    ImageModelFluxPro11,
    ImageModelFluxProUltra11,
    ImageModelFluxDev,
    ImageModelGptImage1
    // ImageModelImagen3,
    // ImageModelImagen3Fast
];
const QaModels = [
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    QAModelCustom,
    QAModelShared
];
const ImageModels = [
    ImageModelDalle3,
    ImageModelGptImage1,
    ImageModelSdxlTurbo,
    // ImageModelFluxPro,
    ImageModelFluxPro11,
    ImageModelFluxDev,
    ImageModelFluxProUltra11,
    ImageModelFluxSchnell,
    ImageModelImagen3,
    ImageModelImagen3Fast
    // ImageModelImg2Img
];
const CompletionModels = [
    CompletionModelDavinci3
];
const FreeModels = [
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    // ChatModelGroqGemma2With9B,
    ChatModelGroqGemma3With27B,
    ChatModelGroqllama3With70B,
    ChatModelQwenQwq32B,
    // ChatModelGroqllama3With8B,
    // ChatModelGroqllama3With405B,
    // ChatModelTurbo35,
    ChatModelGPT4OMini,
    // ChatModelGPT4OMiniSearch,
    ChatModelDeepSeekChat,
    // ChatModelDeepSeekCoder,
    // ChatModelTurbo35V0125,
    // ChatModelGeminiPro,
    // ChatModelGeminiProVision,
    // ChatModelGemini2Flash,
    ChatModelGemini25Flash,
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    ImageModelSdxlTurbo
    // ImageModelImg2Img
];
const AllModels = [].concat(
    ChatModels,
    QaModels,
    ImageModels,
    CompletionModels
);
const ModelCategories = {
    OpenAI: [
        ChatModelGPT4OMini,
        ChatModelGPT4O,
        ChatModelGPT4OMiniSearch,
        ChatModelGPT4OSearch,
        ChatModelGPT4Turbo,
        ChatModelGPTO3Mini,
        ChatModelGPTO4Mini,
        ChatModelGPTO1,
        ChatModelGPTO3
    ],
    Anthropic: [
        ChatModelClaude3Opus,
        ChatModelClaude35Sonnet,
        ChatModelClaude37Sonnet,
        ChatModelClaude37SonnetThinking,
        ChatModelClaude35Haiku
    ],
    Google: [
        ChatModelGemini2Pro,
        ChatModelGemini25Pro,
        // ChatModelGemini2Flash,
        ChatModelGemini25Flash,
        ChatModelGemini2FlashThinking,
        ChatModelGemini2FlahExpImage,
        ChatModelGroqGemma3With27B
    ],
    Deepseek: [
        ChatModelDeepSeekChat,
        ChatModelDeepSeekResoner
    ],
    Others: [
        ChatModelDeepResearch,
        // ChatModelGroqGemma2With9B,
        ChatModelGroqllama3With70B,
        ChatModelQwenQwq32B
    ]
};

// const ModelPriceUSD = {
//     ImageModelDalle3: '0.04',
//     ImageModelFluxPro11: '0.04',
//     ImageModelFluxSchnell: '0.003'
// };

// custom dataset's end-to-end password
const KvKeyPinnedMaterials = 'config_api_pinned_materials';
const KvKeyAllowedModels = 'config_chat_models';
const KvKeyCustomDatasetPassword = 'config_chat_dataset_key';
const KvKeyPromptShortCuts = 'config_prompt_shortcuts';
const KvKeyPrefixSessionHistory = 'chat_user_session_';
const KvKeyPrefixSessionConfig = 'chat_user_config_';
const KvKeyPrefixSelectedSession = 'config_selected_session';
const KvKeySyncKey = 'config_sync_key';
// const KvKeyAutoSyncUserConfig = 'config_auto_sync_user_config';
const KvKeyVersionDate = 'config_version_date';
const KvKeyUserInfo = 'config_user_info';
const KvKeyChatData = 'chat_data_'; // ${KvKeyChatData}${role}_${chatID}

const RoleHuman = 'user';
const RoleSystem = 'system';
const RoleAI = 'assistant';

const chatContainer = document.getElementById('chatContainer');
const configContainer = document.getElementById('hiddenChatConfigSideBar');

// user-input could be re-render to talking widget,
// so these widgets could be override after re-rendering.
//
// ⚠️ be careful these elements could be null after when talking widget is active.
let chatPromptInputEle = chatContainer.querySelector('.user-input .input.prompt');
let chatPromptInputBtn = chatContainer.querySelector('.user-input .btn.send');

// muRenderAfterAiRespForChatID is a mutex for rendering chat
// to avoid rendering the same chat multiple times.
// will be updated in renderAfterAiResp and reloadAiResp.
const muRenderAfterAiRespForChatID = {};

let audioStream;
const httpsRegexp = /\bhttps:\/\/\S+/;

/**
 * setup confirm modal callback, shoule be an async function
 */
let deleteCheckCallback,
    /**
     * global shared modal to act as confirm dialog
     */
    deleteCheckModal;
let singleInputCallback,
    singleInputModal,
    editImageModalCallback,
    systemPromptModalCallback;

// could be controlled(interrupt) anywhere, so it's global
let globalAIRespSSE, globalAIRespEle, globalAIRespData, globalAIRespHeartBeatTimer;
// Add these variables at the beginning of your file, near other globals
let sseMessageBuffer = '';
let sseMessageFragmented = false;
let responseContainer = null;
let reasoningContainer = null;
// const safariSSEHeartbeatTimer = null;
// const safariSSERetryCount = 0;
// const MAX_SSE_RETRIES = 3;

// [
//     {
//         filename: xx,
//         contentB64: xx,
//         cache_key: xx
//     }
// ]
//
// should invoke updateChatVisionSelectedFileStore after update this object
let chatVisionSelectedFileStore = [];

function IsChatModel (model) {
    return ChatModels.includes(model);
};

function IsQaModel (model) {
    return QaModels.includes(model);
};

function IsCompletionModel (model) {
    return CompletionModels.includes(model);
};

function IsImageModel (model) {
    return ImageModels.includes(model);
};

function ShowSpinner () {
    document.getElementById('spinner').toggleAttribute('hidden', false);
};
function HideSpinner () {
    document.getElementById('spinner').toggleAttribute('hidden', true);
};

async function OpenaiSelectedModel () {
    const sconfig = await getChatSessionConfig();
    let selectedModel = sconfig.selected_model || DefaultModel;

    if (!AllModels.includes(selectedModel)) {
        selectedModel = DefaultModel;
    }

    return selectedModel;
};

/** get or set chat static context
 *
 * @param {string} prompt
 * @returns {string} prompt
 */
async function OpenaiChatStaticContext (prompt) {
    const sconfig = await getChatSessionConfig();

    if (prompt) {
        sconfig.system_prompt = prompt;
        await saveChatSessionConfig(sconfig);
    }

    return sconfig.system_prompt || '';
};

function SingleInputModal (title, message, callback, defaultVal) {
    const modal = document.getElementById('singleInputModal');
    singleInputCallback = async () => {
        try {
            ShowSpinner();
            await callback(modal.querySelector('.modal-body input').value);
        } finally {
            HideSpinner();
        }
    }

    modal.querySelector('.modal-title').innerHTML = title;
    modal.querySelector('.modal-body label.form-label').innerHTML = message;
    modal.querySelector('.modal-body input').value = defaultVal || '';
    singleInputModal.show();
    modal.querySelector('.modal-body input').focus();
};

// show modal to confirm,
// callback will be called if user click yes
//
// params:
//   - title: modal title
//   - callback: async callback function
function ConfirmModal (title, callback) {
    deleteCheckCallback = async () => {
        try {
            ShowSpinner();
            await callback();
        } finally {
            HideSpinner();
        }
    };
    document.getElementById('deleteCheckModal').querySelector('.modal-title').innerHTML = title;
    deleteCheckModal.show();
};

// main entry
let mainRunned = false;
async function main (event) {
    if (mainRunned) {
        return;
    }
    mainRunned = true;

    setupDarkMode();
    await setupModals();
    await dataMigrate();
    await setupHeader();
    setupSingleInputModal();
    await removeOrphanChatData();

    checkUpgrade(); // run in background
    await setupChatJs();
};
main();

async function setupDarkMode () {
    setInterval(() => {
        document.documentElement.setAttribute('data-bs-theme', (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
    }, 1000);
}

/**
 * scan all chat data, and remove the chat data which id does not exists in all chat session history
 */
async function removeOrphanChatData () {
    console.debug('Starting orphan chat data cleanup...');
    const allKeys = await libs.KvList();
    const chatDataKeys = new Map(); // Map<chatID, Set<fullKey>>
    const sessionHistoryKeys = [];
    const validChatIDs = new Set();

    // 1. Identify all individual chat data keys and session history keys
    allKeys.forEach(key => {
        if (key.startsWith(KvKeyChatData)) {
            // Example key: chat_data_user_chat-1705899120122-NwN9sB
            const parts = key.substring(KvKeyChatData.length).split('_');
            if (parts.length >= 2) {
                const role = parts[0];
                const chatID = parts.slice(1).join('_'); // Rejoin in case chatID has underscores
                if (role === RoleHuman || role === RoleAI) {
                    if (!chatDataKeys.has(chatID)) {
                        chatDataKeys.set(chatID, new Set());
                    }
                    chatDataKeys.get(chatID).add(key);
                } else {
                    console.warn(`Unexpected role found in chat data key: ${key}`);
                }
            } else {
                console.warn(`Malformed chat data key found: ${key}`);
            }
        } else if (key.startsWith(KvKeyPrefixSessionHistory)) {
            sessionHistoryKeys.push(key);
        }
    });

    console.debug(`Found ${chatDataKeys.size} unique chatIDs in individual data.`);
    console.debug(`Found ${sessionHistoryKeys.length} session history keys.`);

    // 2. Collect all valid chatIDs from all session histories
    for (const historyKey of sessionHistoryKeys) {
        try {
            const sessionHistory = await libs.KvGet(historyKey);
            if (Array.isArray(sessionHistory)) {
                sessionHistory.forEach(chatItem => {
                    if (chatItem && chatItem.chatID) {
                        validChatIDs.add(chatItem.chatID);
                    }
                });
            } else {
                console.warn(`Session history data is not an array for key: ${historyKey}`);
            }
        } catch (error) {
            console.error(`Error reading session history for key ${historyKey}:`, error);
            // Decide whether to continue or stop. Continuing seems safer for cleanup.
        }
    }

    console.debug(`Collected ${validChatIDs.size} valid chatIDs from session histories.`);

    // 3. Identify and delete orphaned chat data
    let deletedCount = 0;
    for (const [chatID, fullKeys] of chatDataKeys.entries()) {
        if (!validChatIDs.has(chatID)) {
            console.debug(`Found orphaned chatID: ${chatID}. Deleting associated data...`);
            for (const fullKeyToDelete of fullKeys) {
                try {
                    await libs.KvDel(fullKeyToDelete);
                    console.debug(`Deleted orphan data key: ${fullKeyToDelete}`);
                    deletedCount++;
                } catch (error) {
                    console.error(`Error deleting orphan data key ${fullKeyToDelete}:`, error);
                }
            }
        }
    }

    if (deletedCount > 0) {
        console.log(`Orphan chat data cleanup finished. Deleted ${deletedCount} KV entries.`);
    } else {
        console.debug('Orphan chat data cleanup finished. No orphan data found.');
    }
}

/**
 * Helper function to convert base64 string to Blob
 * @param {string} base64 - The base64 encoded string
 * @param {string} contentType - The content type of the blob (e.g., 'image/png')
 * @returns {Blob}
 */
function base64ToBlob (base64, contentType = 'application/octet-stream') {
    const byteCharacters = atob(base64);
    const byteNumbers = new Array(byteCharacters.length);
    for (let i = 0; i < byteCharacters.length; i++) {
        byteNumbers[i] = byteCharacters.charCodeAt(i);
    }
    const byteArray = new Uint8Array(byteNumbers);
    return new Blob([byteArray], { type: contentType });
}

/**
 * show image edit modal
 *
 * @param {string} imgSrc - image url or base64 encoded image
 */
async function showImageEditModal (chatID, imgSrc) {
    const modalEle = document.getElementById('modal-draw-canvas');
    const canvasContainer = modalEle.querySelector('.modal-body');
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');

    // Load the image onto the canvas
    const img = new Image();
    img.crossOrigin = 'anonymous';
    await new Promise((resolve, reject) => {
        img.onload = () => {
            if (img.naturalWidth !== img.naturalHeight) {
                reject(new Error('Image must be square'));
            }

            canvas.width = img.naturalWidth;
            canvas.height = img.naturalHeight;
            ctx.drawImage(img, 0, 0);
            resolve();
        };
        img.onerror = reject;
        img.src = imgSrc;
    });

    // Append the canvas to the modal's body
    canvasContainer.innerHTML = '';
    canvasContainer.appendChild(canvas);

    // Variables for drawing transparent areas
    let isDrawing = false;

    function getMousePos (canvas, evt) {
        const rect = canvas.getBoundingClientRect();
        return {
            x: evt.clientX - rect.left,
            y: evt.clientY - rect.top
        };
    }

    // Drawing function with transparency
    function draw (e) {
        if (!isDrawing) return;
        const { x, y } = getMousePos(canvas, e);
        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';
        ctx.strokeStyle = 'rgba(255, 255, 0, 1)'; // Fully transparent color
        ctx.lineWidth = 40; // Adjust line width as needed
        ctx.lineTo(x, y);
        ctx.stroke();
    }

    // Event listeners for drawing
    canvas.addEventListener('mousedown', (e) => {
        e.preventDefault();
        isDrawing = true;
    });

    canvas.addEventListener('mousemove', (e) => {
        e.preventDefault();
        if (isDrawing) draw(e);
    });

    canvas.addEventListener('mouseup', (e) => {
        e.preventDefault();
        isDrawing = false;
        ctx.beginPath(); // Start a new path for next drawing
    });

    const imgEditModal = new window.bootstrap.Modal(modalEle);
    imgEditModal.show();

    // Button click event for generating and downloading mask
    editImageModalCallback = async (e) => {
        e.preventDefault();

        // check data
        const prompt = modalEle.querySelector('.prompt .input').value.trim();
        if (prompt.length === 0) {
            showalert('danger', 'Please enter a prompt');
            return;
        }

        // Extract image data and create mask
        const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const maskData = new Uint8ClampedArray(imageData.data.length);

        for (let i = 0; i < imageData.data.length; i += 1) {
            maskData[i] = imageData.data[i];
        }

        // This is for dall-e-2 inpainting
        //
        // set alpha to where the image is been drawn to yellow
        // for (let i = 0; i < imageData.data.length; i += 4) {
        //     if (imageData.data[i] >= 250 && imageData.data[i + 1] >= 250 && imageData.data[i + 2] <= 5) {
        //         maskData[i + 3] = 0; // set alpha to 0
        //     }
        // }

        // This is for flux inpainting
        //
        // set color to black where the image is been drawn to yellow,
        // and set remaining color to white
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

        // Create a new canvas and context for the mask
        const maskCanvas = document.createElement('canvas');
        maskCanvas.width = canvas.width;
        maskCanvas.height = canvas.height;
        const maskCtx = maskCanvas.getContext('2d');

        // Put the mask data onto the mask canvas
        const maskImageData = new ImageData(maskData, canvas.width, canvas.height);
        maskCtx.putImageData(maskImageData, 0, 0);

        const maskBlob = await new Promise((resolve) => {
            maskCanvas.toBlob(async (blob) => {
                resolve(blob);
            })
        });

        const rawImgBlob = await (await fetch(imgSrc)).blob();

        imgEditModal.hide();

        // inpaintingImageByDalle(chatID, prompt, rawImgBlob, maskBlob);
        inpaintingImageByFlux(chatID, prompt, rawImgBlob, maskBlob);
    };
}

/**
 * replace data url's prefix to `data:application/octet-stream;base64,`
 */
// function replaceDataUrlPrefix (dataUrl) {
//     return dataUrl.replace(/^data:image\/(png|jpeg);base64,/, 'data:application/octet-stream;base64,');
// }

async function inpaintingImageByFlux (chatID, prompt, rawImgBlob, maskBlob) {
    globalAIRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    const chatData = await getChatData(chatID, RoleAI);
    const selectedModel = chatData.model;

    const rawImgBase64 = await new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.readAsDataURL(rawImgBlob);
    });
    const maskBase64 = await new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.readAsDataURL(maskBlob);
    });

    console.log(rawImgBase64);

    // replace data url's prefix to `data:application/octet-stream;base64,`
    // const rawImgUrl = replaceDataUrlPrefix(rawImgBase64);
    // const rawMaskUrl = replaceDataUrlPrefix(maskBase64);

    const reqBody = {
        input: {
            prompt,
            mask: maskBase64,
            image: rawImgBase64,
            // mask: rawMaskUrl,
            // image: rawImgUrl,
            seed: Date.now(),
            steps: 30,
            guidance: 3,
            safety_tolerance: 5,
            prompt_upsampling: false
        }
    };

    ShowSpinner();
    try {
        const sconfig = await getChatSessionConfig();
        const resp = await fetch('/images/edit/flux/flux-fill-pro', {
            method: 'POST',
            headers: {
                Authorization: 'Bearer ' + sconfig.api_token,
                'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
                'X-Laisky-Api-Base': sconfig.api_base
            },
            body: JSON.stringify(reqBody)
        });
        if (!resp.ok || resp.status !== 200) {
            throw new Error(`[${resp.status}]: ${await resp.text()}`);
        }
        const respData = await resp.json();

        globalAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
        globalAIRespEle.dataset.taskType = ChatTaskTypeImage;
        globalAIRespEle.dataset.taskId = respData.task_id;
        globalAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);
        globalAIRespEle.innerHTML = `
            <p dir="auto" class="card-text placeholder-glow">
                <span class="placeholder col-7"></span>
                <span class="placeholder col-4"></span>
                <span class="placeholder col-4"></span>
                <span class="placeholder col-6"></span>
                <span class="placeholder col-8"></span>
            </p>`;

        // save img to storage no matter it's done or not
        let attachHTML = '';
        respData.image_urls.forEach((url) => {
            attachHTML += `<div class="ai-resp-image">
            <div class="hover-btns">
                <i class="bi bi-pencil-square"></i>
            </div>
            <img src="${url}">
        </div>`
        });

        await saveChats2Storage({
            role: RoleAI,
            chatID,
            model: selectedModel,
            content: attachHTML
        });
    } catch (err) {
        abortAIResp(`Failed to send request: ${renderError(err)}`);
    } finally {
        HideSpinner();
    }
}

// async function inpaintingImageByDalle (chatID, prompt, rawImgBlob, maskBlob) {
//     const formData = new FormData();
//     formData.append('image', rawImgBlob, 'image.png');
//     formData.append('mask', maskBlob, 'mask.png');
//     formData.append('prompt', prompt);
//     formData.append('model', ImageModelDalle2);
//     formData.append('response_format', 'b64_json');

//     try {
//         await reloadAiResp(chatID, async () => {
//             const sconfig = await getChatSessionConfig();
//             const resp = await fetch('/oneapi/v1/images/edits', {
//                 method: 'POST',
//                 body: formData,
//                 headers: {
//                     Authorization: `Bearer ${sconfig.api_token}`
//                 }
//             });
//             const respData = await resp.json();
//             const respImageData = respData.data[0].b64_json;
//             const content = `<div class="ai-resp-image">
//                     <div class="hover-btns">
//                         <i class="bi bi-pencil-square"></i>
//                     </div>
//                     <img src="data:image/png;base64,${respImageData}">
//                 </div>`

//             await append2Chats({
//                 chatID,
//                 role: RoleAI,
//                 model: ImageModelDalle2,
//                 content
//             });
//             await saveChats2Storage({
//                 role: RoleAI,
//                 chatID,
//                 model: ImageModelDalle2,
//                 content
//             });
//         });
//     } catch (e) {
//         abortAIResp(e);
//     } finally {
//         HideSpinner();
//     }
// }

async function dataMigrate () {
    const sid = await activeSessionID();
    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    let sconfig = await libs.KvGet(skey);

    // set selected session
    if (!await libs.KvGet(KvKeyPrefixSelectedSession)) {
        await libs.KvSet(KvKeyPrefixSelectedSession, parseInt(sid));
    }

    // move config from localstorage to session config
    {
        // move global config
        const storageVals = { // old: new
            config_prompt_shortcuts: KvKeyPromptShortCuts,
            config_chat_dataset_key: KvKeyCustomDatasetPassword,
            config_api_pinned_materials: KvKeyPinnedMaterials,
            config_chat_models: KvKeyAllowedModels,
            config_selected_session: KvKeyPrefixSelectedSession
        };
        await Promise.all(Object.keys(storageVals)
            .map(async (oldKey) => {
                const val = libs.GetLocalStorage(oldKey);
                if (!val) {
                    return;
                }

                const newKey = storageVals[oldKey];
                await libs.KvSet(newKey, val);
                localStorage.removeItem(oldKey);
            })
        );

        // move session config
        if (!sconfig) {
            console.log(`generate new session config for ${sid} during data migrate`);
            sconfig = newSessionConfig();

            sconfig.api_token = libs.GetLocalStorage('config_api_token_value') || sconfig.api_token;
            sconfig.token_type = libs.GetLocalStorage('config_api_token_type') || sconfig.token_type;
            sconfig.max_tokens = libs.GetLocalStorage('config_api_max_tokens') || sconfig.max_tokens;
            sconfig.temperature = libs.GetLocalStorage('config_api_temperature') || sconfig.temperature;
            sconfig.presence_penalty = libs.GetLocalStorage('config_api_presence_penalty') || sconfig.presence_penalty;
            sconfig.frequency_penalty = libs.GetLocalStorage('config_api_frequency_penalty') || sconfig.frequency_penalty;
            sconfig.n_contexts = libs.GetLocalStorage('config_api_n_contexts') || sconfig.n_contexts;
            sconfig.system_prompt = libs.GetLocalStorage('config_api_static_context') || sconfig.system_prompt;
            sconfig.selected_model = libs.GetLocalStorage('config_chat_model') || sconfig.selected_model;
        }

        // set api token from url params
        const apikey = new URLSearchParams(location.search).get('apikey');
        if (apikey) {
            // remove apikey from url params
            const url = new URL(location.href);
            url.searchParams.delete('apikey');
            window.history.pushState({}, document.title, url);
            sconfig.api_token = apikey;
        }

        await libs.KvSet(skey, sconfig);
    }

    // list all session configs
    await Promise.all((await libs.KvList()).map(async (key) => {
        if (!key.startsWith(KvKeyPrefixSessionConfig)) {
            return;
        }

        let eachSconfig = await libs.KvGet(key);
        if (!eachSconfig) {
            console.log(`generate new session config for ${key}`);
            eachSconfig = newSessionConfig();
        }

        // set default api_token
        if (!eachSconfig.api_token || eachSconfig.api_token === 'DEFAULT_PROXY_TOKEN') {
            console.log(`generate new api_token for ${key}`);
            eachSconfig.api_token = 'FREETIER-' + libs.RandomString(32);
        }
        // set default api_base
        if (!eachSconfig.api_base) {
            eachSconfig.api_base = 'https://api.openai.com';
        }

        // set default chat controller,
        // if add new field, should also add set default value below
        if (!eachSconfig.chat_switch) {
            eachSconfig.chat_switch = {
                all_in_one: false,
                disable_https_crawler: true,
                enable_google_search: false,
                enable_talk: false,
                draw_n_images: 1
            };
        }
        if (!eachSconfig.chat_switch.all_in_one) {
            eachSconfig.chat_switch.all_in_one = false;
        }
        if (!eachSconfig.chat_switch.draw_n_images) {
            eachSconfig.chat_switch.draw_n_images = 1;
        }

        // change model
        if (!eachSconfig.selected_model || !AllModels.includes(eachSconfig.selected_model)) {
            eachSconfig.selected_model = DefaultModel;
        }

        console.debug('migrate session config: ', key, eachSconfig);
        await libs.KvSet(key, eachSconfig);
    }))

    { // move session history from localstorage to kv
        await Promise.all(Object.keys(localStorage).map(async (key) => {
            if (!key.startsWith(KvKeyPrefixSessionHistory)) {
                return;
            }

            // move from localstorage to kv
            await libs.KvSet(key, JSON.parse(localStorage[key]));
            localStorage.removeItem(key);
        }));
    }

    // update chat history, add chatID to each chat
    await Promise.all((await libs.KvList()).map(async (key) => {
        if (!key.startsWith(KvKeyPrefixSessionHistory)) {
            return;
        }

        const chats = await libs.KvGet(key);
        if (!chats) {
            return;
        }

        await Promise.all(chats.map(async (chat) => {
            if (!chat.chatID) {
                chat.chatID = newChatID();
            }

            // move chat data from session to individual chat data
            if (!await libs.KvExists(`${KvKeyChatData}${chat.role}_${chat.chatID}`)) {
                await setChatData(chat.role, chat.chatID, chat);
            }
        }));

        await libs.KvSet(key, chats);
    }));
}

/**
 * check if there is new version, if so, show modal to ask user to reload page.
 */
async function checkUpgrade () {
    // fetch server's version
    const resp = await fetch('/version',
        {
            method: 'GET',
            cache: 'no-cache'
        });
    if (!resp.ok) {
        console.error('failed to fetch version');
        return;
    }

    let currentVer = null;
    const data = await resp.json();
    for (const item of data.Settings) {
        if (item.Key === 'vcs.time') {
            currentVer = item.Value;
            break;
        }
    }

    // fetch local's version
    const localVer = await libs.KvGet(KvKeyVersionDate);

    // check version
    if (currentVer && currentVer !== localVer) {
        await libs.KvSet(KvKeyVersionDate, currentVer); // save/skip this version
        if (localVer) {
            // check if modal is already open
            if (!document.querySelector('#deleteCheckModal.show')) {
                ConfirmModal(`New version found ${localVer} -> ${currentVer}, reload page to upgrade?`, async () => {
                    location.reload();
                });
            }
        }
    }
}

function setupSingleInputModal () {
    singleInputCallback = null;
    singleInputModal = new window.bootstrap.Modal(document.getElementById('singleInputModal'));
    document.getElementById('singleInputModal')
        .querySelector('.modal-body .yes')
        .addEventListener('click', async (e) => {
            e.preventDefault();

            if (singleInputCallback) {
                await singleInputCallback();
            }

            singleInputModal.hide();
        });
}

async function setupModals () {
    // init delete check modal
    deleteCheckModal = new window.bootstrap.Modal(document.getElementById('deleteCheckModal'));
    document.getElementById('deleteCheckModal')
        .querySelector('.modal-body .yes')
        .addEventListener('click', async (e) => {
            e.preventDefault();

            if (deleteCheckCallback) {
                await deleteCheckCallback();
            }

            deleteCheckModal.hide();
        });

    // init system prompt modal
    const saveSystemPromptModelEle = document.querySelector('#save-system-prompt.modal');
    const saveSystemPromptModel = new window.bootstrap.Modal(saveSystemPromptModelEle);
    saveSystemPromptModelEle
        .querySelector('.btn.save')
        .addEventListener('click', async (evt) => {
            evt.preventDefault();
            if (systemPromptModalCallback) {
                await systemPromptModalCallback(evt);
            }

            saveSystemPromptModel.hide();
        });

    // init edit image modal
    const editImageModalEle = document.querySelector('#modal-draw-canvas.modal');
    const editImageModal = new window.bootstrap.Modal(editImageModalEle);
    editImageModalEle
        .querySelector('.btn.save')
        .addEventListener('click', async (evt) => {
            evt.preventDefault();
            if (editImageModalCallback) {
                await editImageModalCallback(evt);
            }

            editImageModal.hide();
        });
}

async function setupByUserInfo (userInfo) {
    const headerBarEle = document.getElementById('headerbar');
    let allowedModels = [];
    let selectedModel = await OpenaiSelectedModel();

    // setup chat models
    {
        if (userInfo.allowed_models.includes('*')) {
            userInfo.allowed_models = Array.from(AllModels);
        } else {
            userInfo.allowed_models.push(QAModelCustom, QAModelShared);
        }
        userInfo.allowed_models = userInfo.allowed_models.filter((model) => {
            return AllModels.includes(model);
        });

        userInfo.allowed_models.sort();
        await libs.KvSet(KvKeyAllowedModels, userInfo.allowed_models);
        allowedModels = userInfo.allowed_models;

        if (!allowedModels.includes(selectedModel)) {
            if (allowedModels.includes(DefaultModel)) {
                selectedModel = DefaultModel;
            } else {
                selectedModel = '';
                AllModels.forEach((model) => {
                    if (selectedModel !== '' || !allowedModels.includes(model)) {
                        return;
                    }

                    if (ChatModels.includes(model)) {
                        selectedModel = model;
                    }
                });
            }

            const sconfig = await getChatSessionConfig();
            sconfig.selected_model = selectedModel;
            await saveChatSessionConfig(sconfig);
        }

        const modelsContainer = document.querySelector('#headerbar .chat-models');
        // const unsupportedModels = [];
        let modelsEle = '';
        Object.entries(ModelCategories).forEach(([category, models]) => {
            modelsEle += `<li class="model-category">${category}</li>`;
            models.forEach(model => {
                if (allowedModels.includes(model)) {
                    const isRecommended = [].includes(model);
                    const isFree = FreeModels.includes(model);
                    modelsEle += `<li><a class="dropdown-item${isRecommended ? ' recommended' : ''}${isFree ? ' free-model' : ''}" href="#" data-model="${model}">${model}</a></li>`;
                }
            });
        });
        modelsContainer.innerHTML = modelsEle;
    }

    // setup chat qa models
    // {
    //     const qaModelsContainer = headerBarEle.querySelector('.dropdown-menu.qa-models');
    //     let modelsEle = '';

    //     // Add QA category header
    //     modelsEle += '';

    //     const allowedQaModels = [QAModelCustom, QAModelShared];
    //     window.data.qa_chat_models.forEach((item) => {
    //         if (allowedModels.includes(item.name)) {
    //             allowedQaModels.push(item.name);
    //             const isFree = FreeModels.includes(item.name);
    //             modelsEle += `<li><a class="dropdown-item${isFree ? ' free-model' : ''}" href="#" data-model="${item.name}">${item.name}</a></li>`;
    //         }
    //     });

    //     // Add remaining QA models
    //     allowedModels.forEach((model) => {
    //         if (QaModels.includes(model) && !allowedQaModels.includes(model)) {
    //             const isFree = FreeModels.includes(model);
    //             modelsEle += `<li><a class="dropdown-item${isFree ? ' free-model' : ''}" href="#" data-model="${model}">${model}</a></li>`;
    //         }
    //     });

    //     qaModelsContainer.innerHTML = modelsEle;
    // }

    // setup chat image models
    {
        const imageModelsContainer = headerBarEle.querySelector('.dropdown-menu.image-models');
        let modelsEle = '';

        // Add Image category header
        modelsEle += '';

        // Process each image model
        allowedModels.forEach((model) => {
            if (ImageModels.includes(model)) {
                const isFree = FreeModels.includes(model);
                modelsEle += `<li><a class="dropdown-item${isFree ? ' free-model' : ''}" href="#" data-model="${model}">${model}</a></li>`;
            }
        });

        imageModelsContainer.innerHTML = modelsEle;
    }

    // add active to selected model
    document.querySelectorAll('#headerbar .chat-models li a, ' +
        '#headerbar .qa-models li a, ' +
        '#headerbar .image-models li a'
    ).forEach((elem) => {
        if (elem.dataset.model === selectedModel) {
            elem.classList.add('active');
        }
    });

    // listen click events
    const modelElems = document
        .querySelectorAll('#headerbar .chat-models li a, ' +
            '#headerbar .qa-models li a, ' +
            '#headerbar .image-models li a'
        );
    modelElems.forEach((elem) => {
        elem.addEventListener('click', async (evt) => {
            evt.preventDefault();
            const evtTarget = libs.evtTarget(evt);
            modelElems.forEach((elem) => {
                elem.classList.remove('active');
            })

            evtTarget.classList.add('active');
            const selectedModel = evtTarget.dataset.model;

            const sconfig = await getChatSessionConfig();
            sconfig.selected_model = selectedModel;
            await saveChatSessionConfig(sconfig);

            // add active to dropdown-toggle
            document.querySelectorAll('#headerbar .navbar-nav a.dropdown-toggle')
                .forEach((elem) => {
                    elem.classList.remove('active');
                });
            evtTarget.closest('.dropdown').querySelector('a.dropdown-toggle').classList.add('active');

            // add hint to input text
            if (chatPromptInputBtn) {
                chatPromptInputEle.attributes.placeholder.value = `[${selectedModel}] CTRL+Enter to send`;
            }
        });
    });

    enhanceDropdownMenus();
}

/**
 * setup dropdown menus
 */
function enhanceDropdownMenus () {
    const modelDropdowns = document.querySelectorAll('#headerbar .dropdown-menu');
    modelDropdowns.forEach(dropdown => {
        // If more than 8 items in dropdown, add scrollable class
        if (dropdown.querySelectorAll('li').length > 5) {
            dropdown.classList.add('scrollable');
        }

        // Add scroll event listener to prevent body scrolling while navigating dropdown
        dropdown.addEventListener('wheel', (e) => {
            if (dropdown.scrollHeight > dropdown.clientHeight) {
                e.stopPropagation();
            }
        });
    });
}

async function loadAndUpdateUserInfo (oldUserInfo) {
    const sconfig = await getChatSessionConfig();

    // get users' models
    const headers = new Headers();
    headers.append('Authorization', 'Bearer ' + sconfig.api_token);
    const response = await fetch('/user/me', {
        method: 'GET',
        cache: 'no-cache',
        headers
    });

    if (response.status !== 200) {
        throw new Error('failed to get user info, please refresh your browser.');
    }

    const newUserInfo = await response.json();
    await libs.KvSet(KvKeyUserInfo, newUserInfo);

    // if user info changed, update it
    if (!oldUserInfo || !libs.Compatible(oldUserInfo, newUserInfo)) {
        console.log('user info changed, update it');
        await setupByUserInfo(newUserInfo);
    }
}

/**
 * setup header bar
 */
async function setupHeader () {
    // get user info from cache
    const userInfo = await libs.KvGet(KvKeyUserInfo);
    if (userInfo) {
        console.log('use cached user info');
        await setupByUserInfo(userInfo);
        try {
            await loadAndUpdateUserInfo(userInfo);
        } catch (error) {
            console.error('Error loading and updating user info:', error);
            showalert('danger', 'Failed to load user information. Please check your API token and network connection.');
        }
    } else {
        try {
            await loadAndUpdateUserInfo();
        } catch (error) {
            console.error('Error loading and updating user info:', error);
            showalert('danger', 'Failed to load user information. Please check your API token and network connection.');
        }
    }

    // click header to scroll to top
    document.getElementById('headerbar').addEventListener('click', scrollChatToTop);
}

// eslint-disable-next-line no-unused-vars
async function setupChatJs () {
    await setupSessionManager();
    await setupConfig();
    await setupChatInput();
    await setupChatSwitchs();
    await setupPromptManager();
    await setupPrivateDataset();
    setupScrollDetection();
    setupGlobalAiRespHeartbeatTimer();
    setInterval(fetchImageDrawingResultBackground, 3000);
    setInterval(fetchDeepResearchResultBackground, 3000);
}

function newChatID () {
    return `chat-${(new Date()).getTime()}-${libs.RandomString(6)}`;
}

/**
 * check if AI response is IDLE periodly
 */
function setupGlobalAiRespHeartbeatTimer () {
    globalAIRespHeartBeatTimer = Date.now();

    setInterval(async () => {
        if (!globalAIRespSSE) {
            return;
        }

        if (Date.now() - globalAIRespHeartBeatTimer > 1000 * 120) {
            console.warn('no heartbeat for 120s, abort AI resp');
            await abortAIResp('no heartbeat for 120s, abort AI resp automatically');
        }
    }, 3000);
}

// show alert
//
// type: primary, secondary, success, danger, warning, info, light, dark
function showalert (type, msg) {
    const alertEle = `<div class="alert alert-${type} alert-dismissible" role="alert">
            <div>${libs.sanitizeHTML(msg)}</div>
            <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
        </div>`;

    // append as first child
    chatContainer.querySelector('.chatManager .alerts-container .alerts')
        .insertAdjacentHTML('afterbegin', alertEle);
}

// check sessionID's type, secure convert to int, default is 1
function kvSessionKey (sessionID) {
    sessionID = parseInt(sessionID) || 1
    return `${KvKeyPrefixSessionHistory}${sessionID}`
}

/**
 *
 * @param {*} sessionID
 * @returns {Array} An array of chat messages.
 */
async function sessionChatHistory (sessionID) {
    let data = (await libs.KvGet(kvSessionKey(sessionID)));
    if (!data) {
        return [];
    }

    // fix legacy bug for marshal data twice
    if (typeof data === 'string') {
        data = JSON.parse(data);
        await libs.KvSet(kvSessionKey(sessionID), data);
    }

    return data;
}

/**
 *
 * @returns {Array} An array of chat messages, oldest first.
 */
async function activeSessionChatHistory () {
    const sid = await activeSessionID();
    if (!sid) {
        return [];
    }

    return await sessionChatHistory(sid);
}

async function activeSessionID () {
    const itemEle = document.querySelector('#sessionManager .card-body button.active');
    if (itemEle) {
        return parseInt(itemEle.closest('.session').dataset.session);
    }

    let activeSession = await libs.KvGet(KvKeyPrefixSelectedSession);
    if (activeSession) {
        if (!document.querySelector(`#sessionManager .card-body .session[data-session="${activeSession}"]`)) {
            // if session not exists on tabs, choose first tab's session as active session
            const firstSessionEle = document.querySelector('#sessionManager .card-body .session');
            if (firstSessionEle) {
                activeSession = parseInt(firstSessionEle.dataset.session || '1');
                await libs.KvSet(KvKeyPrefixSelectedSession, activeSession);
            }
        }

        return parseInt(activeSession);
    }

    return 1;
}

async function listenSessionSwitch (evt) {
    const ele = libs.evtTarget(evt);
    // if (!ele.classList.contains('list-group-item')) {
    //     ele = ele.closest('.list-group-item');
    // }
    const activeSid = parseInt(ele.dataset.session);
    await changeSession(activeSid);
}

async function changeSession (activeSid) {
    if (globalAIRespSSE) { // auto stop previous sse when switch session
        console.warn('auto stop previous sse because of session switch');
        globalAIRespSSE.close();
        globalAIRespSSE = null;
        unlockChatInput();
    }

    // deactive all sessions
    document
        .querySelectorAll(`
            #sessionManager .sessions .session,
            #chatContainer .sessions .session-tabs .session
        `)
        .forEach((ele) => {
            const item = ele.querySelector('.list-group-item');
            if (parseInt(ele.dataset.session) === activeSid) {
                item.classList.add('active');
            } else {
                item.classList.remove('active');
            }
        })

    // restore session history
    chatContainer.querySelector('.conservations .chats').innerHTML = '';
    for (const item of await sessionChatHistory(activeSid)) {
        const chatData = await getChatData(item.chatID, item.role);
        await append2Chats(true, chatData);
        if (item.role === RoleAI) {
            await renderAfterAiResp(chatData, false);
        }
    }

    await libs.KvSet(KvKeyPrefixSelectedSession, activeSid);
    await updateConfigFromSessionConfig();
    await autoToggleUserImageUploadBtn();
    libs.EnableTooltipsEverywhere();
}

let mutexFetchImageDrawingResultBackground = false;

/**
 * Fetches the image drawing result background for the AI response and displays it in the chat container.
 * @returns {Promise<void>}
 */
async function fetchImageDrawingResultBackground () {
    if (mutexFetchImageDrawingResultBackground) {
        return;
    }

    mutexFetchImageDrawingResultBackground = true;
    try {
        const elements = chatContainer
            .querySelectorAll(`.role-ai .ai-response[data-task-type="${ChatTaskTypeImage}"][data-task-status="${ChatTaskStatusWaiting}"]`) || [];

        await Promise.all(Array.from(elements).map(async (item) => {
            if (item.dataset.taskStatus !== ChatTaskStatusWaiting) {
                return;
            }

            // const taskId = item.dataset.taskId;
            const chatID = item.closest('.role-ai').dataset.chatid;
            const chatData = await getChatData(chatID, RoleAI) || {};
            const imageUrls = chatData.taskData || [];

            try {
                // should not use Promise.all here, because we need to check each image's status
                // one by one, not all at once to avoid data race of chatData.taskData
                for (const imageUrl of imageUrls) {
                    // check any err msg
                    const errFileUrl = imageUrl.slice(0, imageUrl.lastIndexOf('-')) + '.err.txt';
                    // const errFileUrl = imageUrl.replace(/(\.\w+)$/, '.err.txt');
                    const errFileResp = await fetch(`${errFileUrl}?rr=${libs.RandomString(12)}`, {
                        method: 'GET',
                        cache: 'no-cache'
                    });
                    if (errFileResp.ok || errFileResp.status === 200) {
                        const errText = await errFileResp.text();
                        item.innerHTML += `<p>🔥Someting in trouble...</p><pre style="text-wrap: pretty;">${errText}</pre>`;
                        await checkIsImageAllSubtaskDone(item, imageUrl, false);
                        await saveChats2Storage({
                            role: RoleAI,
                            chatID,
                            model: chatData.model,
                            content: item.innerHTML
                        });
                        return;
                    }

                    // check is image ready
                    const imgResp = await fetch(`${imageUrl}?rr=${libs.RandomString(12)}`, {
                        method: 'GET',
                        cache: 'no-cache'
                    });
                    if (!imgResp.ok || imgResp.status !== 200) {
                        return;
                    }

                    // check is all tasks finished
                    await checkIsImageAllSubtaskDone(item, imageUrl, true);
                }
            } catch (err) {
                console.warn('fetch img result, ' + err);
            };
        }));
    } finally {
        mutexFetchImageDrawingResultBackground = false;
    }
}

/** append chat to chat container
 *
 * @param {element} item ai respnse
 * @param {string} imageUrl current subtask's image url
 * @param {boolean} succeed is current subtask succeed
 */
async function checkIsImageAllSubtaskDone (item, imageUrl, succeed) {
    const chatID = item.closest('.role-ai').dataset.chatid;
    const chatData = await getChatData(chatID, RoleAI) || {};

    let processingImageUrls = chatData.taskData || [];
    if (!processingImageUrls.includes(imageUrl)) {
        return;
    }

    // remove current subtask from imageUrls(tasks)
    processingImageUrls = processingImageUrls.filter((url) => url !== imageUrl)
    chatData.taskData = processingImageUrls;

    const succeedImageUrls = chatData.succeedImageUrls || [];
    if (succeed) {
        succeedImageUrls.push(imageUrl);
        chatData.succeedImageUrls = succeedImageUrls;
    }

    if (processingImageUrls.length === 0) {
        if (succeedImageUrls.length > 0) {
            let imgHTML = '';
            succeedImageUrls.forEach((url) => {
                imgHTML += `<div class="ai-resp-image">
                <div class="hover-btns">
                <i class="bi bi-pencil-square"></i>
                </div>
                <img src="${url}">
                </div>`;
            });
            item.innerHTML = imgHTML;
            bindImageOperationInAiResp(chatID);

            if (succeedImageUrls.length > 1) {
                item.classList.add('multi-images');
            }
        } else {
            item.innerHTML = '<p>🔥Someting in trouble...</p><pre style="text-wrap: pretty;">No image generated</pre>';
        }

        item.dataset.taskStatus = ChatTaskStatusDone;
        chatData.taskStatus = ChatTaskStatusDone;
        chatData.content = item.innerHTML;
        await setChatData(chatID, RoleAI, chatData);

        renderAfterAiResp(chatData, false);
    } else {
        await setChatData(chatID, RoleAI, chatData);
    }
}

let mutexFetchDeepResearchResultBackground = false;

/**
* Fetches the deep research result background for the AI response and displays it in the chat container.
*/
async function fetchDeepResearchResultBackground () {
    if (mutexFetchDeepResearchResultBackground) {
        return;
    }

    mutexFetchDeepResearchResultBackground = true;
    try {
        const elements = chatContainer
            .querySelectorAll(`.role-ai .ai-response[data-task-type="${ChatTaskTypeDeepResearch}"][data-task-status="${ChatTaskStatusWaiting}"]`) || [];

        // should not use Promise.all here, because we need to check each image's status
        // one by one, not all at once to avoid data race of chatData.taskData
        for (const item of elements) {
            if (item.dataset.taskStatus !== ChatTaskStatusWaiting) {
                return;
            }

            const chatID = item.closest('.role-ai').dataset.chatid;
            let chatData = await getChatData(chatID, RoleAI) || {};
            const taskId = chatData.taskId;

            try {
                const resp = await fetch(`/deepresearch/${taskId}`, {
                    method: 'GET',
                    cache: 'no-cache'
                });
                if (!resp.ok || resp.status !== 200) {
                    return;
                }

                const respData = await resp.json();

                switch (respData.status) {
                case 'pending':
                case 'running':
                    return;
                case 'failed':
                    item.innerHTML = `<p>🔥Someting in trouble...</p><pre style="text-wrap: pretty;">${respData.failed_reason}</pre>`;
                    item.dataset.taskStatus = ChatTaskStatusDone;
                    chatData = await updateChatData(chatID, RoleAI, {
                        taskStatus: ChatTaskStatusDone,
                        content: item.innerHTML
                    });
                    break;
                case 'success':
                    item.innerHTML = await renderHTML(respData.result_article, true);
                    item.dataset.taskStatus = ChatTaskStatusDone;
                    chatData = await updateChatData(chatID, RoleAI, {
                        model: 'deep-research',
                        rawContent: respData.result_article,
                        taskStatus: ChatTaskStatusDone,
                        content: item.innerHTML,
                        attachHTML: `
                            <p style="margin-bottom: 0;">
                                <button class="btn btn-info" type="button" data-bs-toggle="collapse" data-bs-target="#chatRef-${chatID}" aria-expanded="false" aria-controls="chatRef-${chatID}" style="font-size: 0.6em">
                                    > toggle reference
                                </button>
                            </p>
                            <div>
                                <div class="collapse" id="chatRef-${chatID}">
                                    <div class="card card-body">${combineRefsWithIndex(respData.result_references.url_to_unified_index)}</div>
                                </div>
                            </div>`
                    });

                    renderAfterAiResp(chatData, false);
                }
            } catch (err) {
                console.warn('fetch deep research result, ' + err);
            };
        }
    } finally {
        mutexFetchDeepResearchResultBackground = false;
    }
}

/**
 * Clears all user sessions and chats from local storage,
 * and resets the chat UI to its initial state.
 * @param {Event} evt - The event that triggered the function (optional).
 * @param {string} sessionID - The session ID to clear (optional).
 *
 * @returns {void}
 */
async function clearSessionAndChats (evt, sessionID) {
    console.debug('clearSessionAndChats', evt, sessionID)
    if (evt) {
        evt.stopPropagation();
    }

    // remove pinned materials
    await libs.KvDel(KvKeyPinnedMaterials);

    if (!sessionID) { // remove all session
        const sconfig = await getChatSessionConfig();

        await Promise.all((await libs.KvList()).map(async (key) => {
            if (
                key.startsWith(KvKeyPrefixSessionHistory) || // remove all sessions
                key.startsWith(KvKeyPrefixSessionConfig) // remove all sessions' config
            ) {
                await libs.KvDel(key);
            }
        }));

        // restore session config
        await libs.KvSet(`${KvKeyPrefixSessionConfig}1`, sconfig);
        await libs.KvSet(kvSessionKey(1), []);
    } else { // only remove one session's chat, keep config
        await Promise.all((await libs.KvList()).map(async (key) => {
            if (
                key.startsWith(KvKeyPrefixSessionHistory) && // remove all sessions
                key.endsWith(`_${sessionID}`) // remove specified session
            ) {
                await libs.KvSet(key, []);
            }
        }));
    }

    location.reload();
}

function bindSessionEditBtn () {
    document.querySelectorAll('#sessionManager .sessions .session .bi.bi-pencil-square')
        .forEach((item) => {
            if (item.dataset.bindClicked) {
                return;
            } else {
                item.dataset.bindClicked = true;
            }

            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const evtTarget = libs.evtTarget(evt);
                const sid = parseInt(evtTarget.closest('.session').dataset.session);
                const sconfig = await getChatSessionConfig(sid);
                const oldSessionName = sconfig.session_name || sid;

                SingleInputModal('Edit session', 'Session name', async (newSessionName) => {
                    if (!newSessionName) {
                        return;
                    }

                    // update session config
                    sconfig.session_name = newSessionName;
                    await saveChatSessionConfig(sconfig, sid);

                    // update session name
                    document
                        .querySelector(`#sessionManager .sessions [data-session="${sid}"]`).dataset.name = newSessionName;
                    document
                        .querySelector(`#sessionManager .sessions [data-session="${sid}"] .col`).innerHTML = newSessionName;
                    chatContainer
                        .querySelector(`.sessions [data-session="${sid}"] .col`)
                        .innerHTML = newSessionName;
                }, oldSessionName);
            })
        })
}

function bindSessionDeleteBtn () {
    const btns = document.querySelectorAll('#sessionManager .sessions .session .bi-trash') || [];
    btns.forEach((item) => {
        if (item.dataset.bindClicked) {
            return;
        } else {
            item.dataset.bindClicked = true;
        }

        item.addEventListener('click', async (evt) => {
            evt.stopPropagation();
            const evtTarget = libs.evtTarget(evt);

            // if there is only one session, don't delete it
            if (document.querySelectorAll('#sessionManager .sessions .session').length === 1) {
                return;
            }

            const activeSid = await activeSessionID();
            const deleteSid = parseInt(evtTarget.closest('.session').dataset.session);
            const deleteSessionName = evtTarget.closest('.session').dataset.name;
            ConfirmModal(`Are you sure to delete session "${deleteSessionName}"?`, async () => {
                await libs.KvDel(`${KvKeyPrefixSessionHistory}${deleteSid}`);
                await libs.KvDel(`${KvKeyPrefixSessionConfig}${deleteSid}`);
                document
                    .querySelectorAll(`#sessionManager .sessions [data-session="${deleteSid}"]`)
                    .forEach((item) => {
                        item.remove();
                    });
                chatContainer
                    .querySelectorAll(`.sessions [data-session="${deleteSid}"]`)
                    .forEach((item) => {
                        item.remove();
                    });

                if (activeSid === deleteSid) {
                    // current active session has been deleted, so need to switch to new session
                    const newSid = await activeSessionID();
                    await changeSession(newSid);
                }
            });
        });
    });
}

function bindSessionDuplicateBtn () {
    const btns = document.querySelectorAll('#sessionManager .sessions .session .bi-files') || [];
    btns.forEach((item) => {
        if (item.dataset.bindClicked) {
            return;
        } else {
            item.dataset.bindClicked = true;
        }

        item.addEventListener('click', async (evt) => {
            evt.stopPropagation();
            const evtTarget = libs.evtTarget(evt);
            const sessionEle = evtTarget.closest('.session');
            const originalSessionID = parseInt(sessionEle.dataset.session);

            // Find the highest existing session ID
            let maxSessionID = 0;
            (await libs.KvList()).forEach((key) => {
                if (key.startsWith(KvKeyPrefixSessionHistory)) {
                    const id = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));
                    if (id > maxSessionID) {
                        maxSessionID = id;
                    }
                }
            });
            const newSessionID = maxSessionID + 1;

            // Get original session data
            const originalSconfig = await getChatSessionConfig(originalSessionID);
            const originalChatHistory = await sessionChatHistory(originalSessionID);
            const originSessionName = originalSconfig.session_name;

            // Create new session config
            const newSconfig = JSON.parse(JSON.stringify(originalSconfig)); // Deep copy
            newSconfig.session_name = `${originalSconfig.session_name || originalSessionID}_copy`;

            // Save new session data
            await libs.KvSet(`${KvKeyPrefixSessionConfig}${newSessionID}`, newSconfig);
            await libs.KvSet(kvSessionKey(newSessionID), originalChatHistory);

            // Add new session to UI
            const newSessionName = newSconfig.session_name;
            const sessionManagerHtml = `
                <div class="list-group session" data-session="${newSessionID}" data-name="${newSessionName}">
                    <button type="button" class="list-group-item list-group-item-action" aria-current="true">
                        <div class="col">${newSessionName}</div>
                        <i class="bi bi-pencil-square"></i>
                        <i class="bi bi-files"></i>
                        <i class="bi bi-trash col-auto"></i>
                    </button>
                </div>`;
            const chatContainerHtml = `
                <div class="list-group session" data-session="${newSessionID}" data-name="${newSessionName}">
                    <button type="button" class="list-group-item list-group-item-action" aria-current="true">
                        <div class="col">${newSessionName}</div>
                    </button>
                </div>`;

            document.querySelector('#sessionManager .sessions').insertAdjacentHTML('beforeend', sessionManagerHtml);
            chatContainer.querySelector('.sessions .session-tabs').insertAdjacentHTML('beforeend', chatContainerHtml);

            // Bind listeners for the new session
            const newSessionElements = document.querySelectorAll(`.session[data-session="${newSessionID}"]`);
            newSessionElements.forEach(el => el.addEventListener('click', listenSessionSwitch));
            bindSessionEditBtn();
            bindSessionDeleteBtn();
            bindSessionDuplicateBtn(); // Bind duplicate for the newly added item

            // Optionally switch to the new session
            // await changeSession(newSessionID);

            showalert('success', `Session "${originSessionName}" duplicated to "${newSessionName}"`);
        });
    });
}

/** setup session manager and restore current chat history
 *
 */
async function setupSessionManager () {
    const selectedSessionID = await activeSessionID();

    // bind remove all sessions
    {
        document
            .querySelector('#sessionManager .btn.purge')
            .addEventListener('click', clearSessionAndChats);
    }

    // restore all sessions from storage
    {
        const allSessionKeys = [];
        (await libs.KvList()).forEach((key) => {
            if (key.startsWith(KvKeyPrefixSessionHistory)) {
                allSessionKeys.push(key);
            }
        });

        if (allSessionKeys.length === 0) { // there is no session, create one
            // create session history
            const skey = kvSessionKey(1);
            allSessionKeys.push(skey);
            await libs.KvSet(skey, []);

            // create session config
            console.log('generate new session config for 1 during setup session manager');
            await libs.KvSet(`${KvKeyPrefixSessionConfig}1`, newSessionConfig());
        }

        await Promise.all(allSessionKeys.map(async (key) => {
            const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));

            let active = '';
            const sconfig = await getChatSessionConfig(sessionID);
            const sessionName = sconfig.session_name || sessionID;
            if (sessionID === selectedSessionID) {
                active = 'active';
            }

            document
                .querySelector('#sessionManager .sessions')
                .insertAdjacentHTML(
                    'beforeend',
                    `<div class="list-group session" data-session="${sessionID}" data-name="${sessionName}">
                        <button type="button" class="list-group-item list-group-item-action ${active}" aria-current="true">
                            <div class="col">${sessionName}</div>
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-files"></i>
                            <i class="bi bi-trash col-auto"></i>
                        </button>
                    </div>`);
            chatContainer
                .querySelector('.sessions .session-tabs')
                .insertAdjacentHTML(
                    'beforeend',
                    `<div class="list-group session" data-session="${sessionID}" data-name="${sessionName}">
                        <button type="button" class="list-group-item list-group-item-action ${active}" aria-current="true">
                            <div class="col">${sessionName}</div>
                        </button>
                    </div>`);
        }));

        // restore conservation history
        const chatHistory = await activeSessionChatHistory();
        for (const chat of chatHistory) {
            const chatData = await getChatData(chat.chatID, chat.role);
            append2Chats(true, chatData);
            if (chat.role === RoleAI) {
                renderAfterAiResp(chatData, false);
            }
        }
    }

    // add widget to scroll bottom
    {
        document.querySelector('#chatContainer .chatManager .card-footer .scroll-down')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                scrollChatToDown();
            });
    }

    // add new session
    {
        document
            .querySelector('#sessionManager .btn.new-session')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                // const evtTarget = libs.evtTarget(evt);

                const oldSessionConfig = await getChatSessionConfig();

                let maxSessionID = 0;
                (await libs.KvList()).forEach((key) => {
                    if (key.startsWith(KvKeyPrefixSessionHistory)) {
                        const id = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));
                        if (id > maxSessionID) {
                            maxSessionID = id;
                        }
                    }
                });

                // deactive all sessions
                document.querySelectorAll(`
                    #sessionManager .sessions .list-group-item.active,
                    #chatContainer .sessions .session-tabs .list-group-item.active
                `).forEach((item) => {
                    item.classList.remove('active');
                });

                // add new active session
                chatContainer
                    .querySelector('.chatManager .conservations .chats').innerHTML = '';
                const newSessionID = maxSessionID + 1;
                document
                    .querySelector('#sessionManager .sessions')
                    .insertAdjacentHTML(
                        'beforeend',
                        `<div class="list-group session" data-session="${newSessionID}" data-sesison="${newSessionID}">
                            <button type="button" class="list-group-item list-group-item-action active" aria-current="true">
                                <div class="col">${newSessionID}</div>
                                <i class="bi bi-pencil-square"></i>
                                <i class="bi bi-files"></i>
                                <i class="bi bi-trash col-auto"></i>
                            </button>
                        </div>`);
                chatContainer
                    .querySelector('.sessions .session-tabs')
                    .insertAdjacentHTML(
                        'beforeend',
                        `<div class="list-group session" data-session="${newSessionID}" data-sesison="${newSessionID}">
                            <button type="button" class="list-group-item list-group-item-action active" aria-current="true">
                                <div class="col">${newSessionID}</div>
                            </button>
                        </div>`);

                // save new session history and config
                await libs.KvSet(kvSessionKey(newSessionID), []);
                console.log(`generate new session config for ${newSessionID} during new session`);
                const sconfig = newSessionConfig();

                // keep old session's api token and api base
                sconfig.api_token = oldSessionConfig.api_token;
                sconfig.api_base = oldSessionConfig.api_base;

                await libs.KvSet(`${KvKeyPrefixSessionConfig}${newSessionID}`, sconfig);
                await libs.KvSet(KvKeyPrefixSelectedSession, newSessionID);

                // bind session switch listener for new session
                document
                    .querySelector(`
                        #sessionManager .sessions .session[data-session="${newSessionID}"],
                        #chatContainer .sessions .session-tabs .session[data-session="${newSessionID}"]
                    `)
                    .addEventListener('click', listenSessionSwitch);

                bindSessionEditBtn();
                bindSessionDeleteBtn();
                bindSessionDuplicateBtn();
                await updateConfigFromSessionConfig();
            })
    }

    // bind session switch
    {
        document
            .querySelectorAll(`
                #sessionManager .sessions .session,
                #chatContainer .sessions .session-tabs .session
            `)
            .forEach((item) => {
                item.addEventListener('click', listenSessionSwitch);
            })
    }

    bindSessionEditBtn();
    bindSessionDeleteBtn();
    bindSessionDuplicateBtn();
}

/**
 * Retrieve the chat history for the active session.
 *
 * @param {string} chatid - Chat id.
 */
async function removeChatInStorage (chatid) {
    if (!chatid) {
        throw new Error('chatid is required');
    }

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    let session = await activeSessionChatHistory();

    // remove all chats with the same chatid, including user's and ai's
    session = session.filter((item) => !(item.chatID === chatid));

    await libs.KvSet(storageActiveSessionKey, session);
}

/**
 * Retrieve chat data from storage by chatID and role.
 *
 * @param {string} chatID - Chat id.
 * @param {string} role - Chat role.
 * @returns {object} The chat data object or an empty object if not found.
 */
async function getChatData (chatID, role) {
    const key = `${KvKeyChatData}${role}_${chatID}`;
    // Return an empty object if no data is found
    return (await libs.KvGet(key)) || {};
}

/**
 * Set chat data to storage by chatID and role.
 *
 * @param {string} chatID - Chat id.
 * @param {string} role - Chat role.
 * @param {object} chatData - Chat data to be saved.
 */
async function setChatData (chatID, role, chatData) {
    const key = `${KvKeyChatData}${role}_${chatID}`;
    await libs.KvSet(key, chatData);
}

/**
 * Update chat data by merging partialChatData into the existing chat data.
 *
 * @param {string} chatID - Chat id.
 * @param {string} role - Chat role.
 * @param {object} partialChatData - Partial data to update.
 * @returns {object} The updated chat data object.
 */
async function updateChatData (chatID, role, partialChatData) {
    let chatData = await getChatData(chatID, role);
    // Ensure we have an object to merge into
    if (typeof chatData !== 'object' || chatData === null) {
        chatData = {
            chatID,
            role
        };
    }

    // Merge only defined, non-null values
    Object.keys(partialChatData).forEach((key) => {
        if (partialChatData[key] !== undefined && partialChatData[key] !== null) {
            chatData[key] = partialChatData[key];
        }
    });

    await setChatData(chatID, role, chatData);
    return chatData;
}

/** append or update chat history by chatID and role.
 * Only save {chatID, role} in session, the full chatData is saved in KvKeyChatData.
 *
 * @param {object} chatData - chat item
 *   @property {string} chatID - chat id
 *   @property {string} role - user or assistant
 *   @property {string} content - rendered chat content
 *   @property {string} attachHTML - chat response's attach html
 *   @property {string} rawContent - chat response's raw content
 *   @property {string} reasoningContent - chat response's reasoning content
 *   @property {string} costUsd - chat cost in USD
 *   @property {string} model - chat model
 *   @property {string} requestid - chat request id
*/
async function saveChats2Storage (chatData) {
    if (!chatData.chatID) {
        throw new Error('chatID is required');
    }

    chatData = await updateChatData(chatData.chatID, chatData.role, chatData);

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    const session = await activeSessionChatHistory();

    // if chat is already in history, find and update it.
    let found = false;
    session.forEach((item, idx) => {
        if (item.chatID === chatData.chatID && item.role === chatData.role) {
            found = true;
        }
    });

    // if ai response is not in history, add it after user's chat which has same chatID
    if (!found && chatData.role === RoleAI) {
        session.forEach((item, idx) => {
            if (item.chatID === chatData.chatID) {
                found = true;
                if (item.role !== RoleAI) {
                    session.splice(idx + 1, 0, {
                        role: RoleAI,
                        chatID: chatData.chatID
                    });
                }
            }
        });
    }

    // if chat is not in history, add it
    if (!found) {
        session.push({
            role: chatData.role,
            chatID: chatData.chatID
        });
    }

    // save chat data
    await setChatData(chatData.chatID, chatData.role, chatData);

    // save session chat history
    await libs.KvSet(storageActiveSessionKey, session);
}

function scrollChatToDown () {
    libs.ScrollDown(document.querySelector('html'));
    libs.ScrollDown(chatContainer.querySelector('.chatManager .conservations'));
}

function scrollChatToTop (evt) {
    // evt.preventDefault();
    // evt.stopPropagation();

    // do not scroll to top if user is clicking on the dropdowns in the header
    if (evt.target.closest('.dropdown-menu') || evt.target.closest('.dropdown-toggle')) {
        return;
    }

    chatContainer.querySelector('.chatManager .conservations')
        .scrollTo({
            top: 0,
            left: 0,
            behavior: 'smooth'
        });
}

// Track user scrolling during AI responses
let userManuallyScrolled = false;

/**
 * Scrolls to a specific chat element unless the user has manually scrolled
 * @param {HTMLElement} chatEle - The chat element to scroll to
 * @param {boolean} force - Whether to force scrolling even if the user scrolled manually
 */
function scrollToChat (chatEle, force = false) {
    if (userManuallyScrolled && !force) {
        return;
    }

    chatEle.scrollIntoView({ behavior: 'smooth', block: 'end' });
}

/**
 * Sets up scroll detection to track user scrolling during AI responses
 */
function setupScrollDetection () {
    const conservationsContainer = chatContainer.querySelector('.chatManager .conservations');

    // Reset the manual scroll flag when a new response starts
    conservationsContainer.addEventListener('wheel', function () {
        // Only set the flag if AI is actively responding
        if (globalAIRespSSE) {
            userManuallyScrolled = true;
        }
    });

    conservationsContainer.addEventListener('touchmove', function () {
        // Only set the flag if AI is actively responding
        if (globalAIRespSSE) {
            userManuallyScrolled = true;
        }
    });

    // Reset the flag when AI response ends or a new one begins
    document.addEventListener('ai-response-start', function () {
        userManuallyScrolled = false;
    });

    document.addEventListener('ai-response-end', function () {
        userManuallyScrolled = false;
    });
}

/**
* Gets the last N chat messages to provide as context for the AI.
* Enhanced to properly handle multi-modal content including images and text.
*
* @param {number} N - The number of messages to retrieve.
* @param {string} ignoredChatID - If provided, the chat with this chatID will be ignored.
* @returns {Array} An array of chat messages with proper content formatting.
*/
async function getLastNChatMessages (N, ignoredChatID) {
    console.debug('getLastNChatMessages', N, ignoredChatID);

    const systemPrompt = await OpenaiChatStaticContext();
    const latestMessages = [];
    const historyMessages = await activeSessionChatHistory();
    let nHuman = 1;
    let latestRole = RoleHuman;
    // Only include images from the latest conversation round,
    // no matter they come from users or AI
    let latestImagesFound = false;

    for (let i = historyMessages.length - 1; i >= 0; i--) {
        const role = historyMessages[i].role;
        const chatID = historyMessages[i].chatID;

        // Skip ignored chatID if specified (for reload/edit cases)
        if (ignoredChatID && ignoredChatID === chatID) {
            continue;
        }

        // Skip system messages and consecutive messages from the same role
        if (role !== RoleHuman && role !== RoleAI) {
            continue;
        }

        if (latestRole && latestRole === role) {
            console.warn(`Latest role is same as current role, skipping. latestRole=${latestRole}`);
            continue;
        }

        // Get the full chat data
        const chatData = await getChatData(chatID, role);

        // Prepare message content and files
        let messageContent = '';
        const files = [];

        if (role === RoleAI && chatData.rawContent && chatData.rawContent.includes('🔥Someting in trouble')) {
            messageContent = 'There was an error during the AI response. Please try again.';
        } else if (chatData.rawContent) {
            messageContent = chatData.rawContent;

            // Extract image base64 data from markdown syntax ![AI-generated image](data:image/png;base64,...)
            const imgRegex = /!\[AI-generated image\]\((data:image\/[^)]+)\)/g;
            const matches = [...chatData.rawContent.matchAll(imgRegex)];

            if (matches.length > 0) {
                // Remove the image markdown from the text
                messageContent = chatData.rawContent.replace(imgRegex, '');

                // Add images to files array
                for (const match of matches) {
                    const dataUrl = match[1];
                    const [header, base64Data] = dataUrl.split(',');
                    const imgType = header.match(/data:image\/([^;]+)/)[1];

                    files.push({
                        type: 'image',
                        name: `image_${Date.now()}_${files.length}.${imgType}`,
                        content: base64Data
                    });
                }
            }
        } else if (chatData.content) {
            messageContent = chatData.content;
        }

        // Handle attachHTML (images from previous conversations)
        if (chatData.attachHTML) {
            const tempDiv = document.createElement('div');
            tempDiv.innerHTML = chatData.attachHTML;

            const imgElements = tempDiv.querySelectorAll('img');
            imgElements.forEach(img => {
                const imgSrc = img.getAttribute('src');
                if (imgSrc && imgSrc.startsWith('data:image/')) {
                    const [header, base64Data] = imgSrc.split(',');
                    const imgType = header.match(/data:image\/([^;]+)/)[1];

                    files.push({
                        type: 'image',
                        name: img.getAttribute('data-name') || `attached_image_${Date.now()}_${files.length}.${imgType}`,
                        content: base64Data
                    });
                }
            });
        }

        // Add the message to our history array
        const message = {
            role,
            content: messageContent
        };

        if (!latestImagesFound && files.length > 0) {
            latestImagesFound = true;
            message.files = files;
        }

        latestMessages.unshift(message);

        // Update tracking variables
        latestRole = role;

        // Count human messages and break when we have enough context
        if (role === RoleHuman) {
            nHuman++;
            if (nHuman > N) {
                break;
            }
        }
    }

    // Add system prompt if available
    if (systemPrompt) {
        latestMessages.unshift({
            role: RoleSystem,
            content: systemPrompt
        });
    }

    // Save pinned URLs to session config
    const sconfig = await getChatSessionConfig();
    const pinnedUrls = getPinnedMaterials() || [];
    if (pinnedUrls.length > 0) {
        sconfig.pinnedMaterials = pinnedUrls;
        await saveChatSessionConfig(sconfig);
    }

    return latestMessages;
}

/**
 * Locks the chat input field and button to prevent user interaction.
 */
function lockChatInput () {
    if (chatPromptInputEle) {
        chatPromptInputEle.setAttribute('disabled', 'disabled');
        chatPromptInputEle.classList.add('disabled-input');
    }

    if (chatPromptInputBtn) {
        chatPromptInputBtn.setAttribute('disabled', 'disabled');
        chatPromptInputBtn.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>';
    }
}

/**
 * Unlocks the chat input field and button to allow user interaction.
 */
function unlockChatInput () {
    if (chatPromptInputEle) {
        chatPromptInputEle.removeAttribute('disabled');
        chatPromptInputEle.classList.remove('disabled-input');
    }

    if (chatPromptInputBtn) {
        chatPromptInputBtn.removeAttribute('disabled');
        chatPromptInputBtn.innerHTML = '<i class="bi bi-send"></i>';
    }
}

/**
 * Checks if the chat prompt input is allowed to be used.
 */
function isAllowChatPrompInput () {
    return !chatPromptInputBtn || !chatPromptInputBtn.classList.contains('disabled');
}

/**
 * Parses the AI response and extracts different content types including text, images, reasoning, and annotations.
 *
 * @param {string} chatmodel - The model used for generating the response
 * @param {object} payload - The raw response payload from the API
 * @returns {object} An object containing the extracted content components
 */
function parseChatResp (chatmodel, payload) {
    let respChunk = '';
    let reasoningChunk = '';
    let annotations = [];

    // Handle empty or malformed payload
    if (!payload.choices || payload.choices.length === 0) {
        console.debug('Empty or malformed payload in parseChatResp:', payload);
        return { respChunk: '', reasoningChunk: '', annotations: [] };
    }

    // Extract annotations if present
    annotations = payload.choices[0].delta.annotations || [];

    // Handle different model types and response formats
    if (IsChatModel(chatmodel) || IsQaModel(chatmodel)) {
        // Extract reasoning content (for models that support thinking/reasoning)
        reasoningChunk += payload.choices[0].delta.reasoning_content || '';
        reasoningChunk += payload.choices[0].delta.reasoning || '';

        // Handle <think> tags for reasoning in some models
        if (payload.choices[0].delta.content === '<think>') {
            globalAIRespData.isThinking = true;
            return { respChunk, reasoningChunk, annotations };
        } else if (payload.choices[0].delta.content === '</think>') {
            globalAIRespData.isThinking = false;
            return { respChunk, reasoningChunk, annotations };
        }

        // Process content based on whether AI is in thinking mode
        if (globalAIRespData.isThinking) {
            reasoningChunk += payload.choices[0].delta.content || '';
        } else {
            // Handle text content
            if (typeof payload.choices[0].delta.content === 'string') {
                respChunk = payload.choices[0].delta.content;
            } else if (Array.isArray(payload.choices[0].delta.content)) {
                // Handle multi-modal content (array format used by models like Gemini)
                // {"type":"image_url","image_url":{"url":"data:image/png;base64,xxxxxxx"}}
                for (const item of payload.choices[0].delta.content) {
                    if (item.type === 'text') {
                        respChunk += item.text || '';
                    } else if (item.type === 'image_url' && item.image_url) {
                        // For image content, create HTML markup to display it
                        const imgSrc = item.image_url.url;
                        const imgAlt = 'AI-generated image';

                        // Use a placeholder in the text stream that will be replaced by the actual image
                        respChunk += `\n![${imgAlt}](${imgSrc})\n`;
                    }
                }
            }
        }
    } else if (IsCompletionModel(chatmodel)) {
        respChunk = payload.choices[0].text || '';
    } else {
        console.warn(`Unknown chat model type in parseChatResp: ${chatmodel}`);
    }

    return {
        respChunk,
        reasoningChunk,
        annotations
    };
}

/**
 * extract https urls from reqPrompt and pin them to the chat conservation window
 *
 * @param {string} reqPrompt - request prompt
 * @returns {string} modified request prompt
 */
async function userPromptEnhance (reqPrompt) {
    const pinnedUrls = getPinnedMaterials() || [];
    const sconfig = await getChatSessionConfig();
    const urls = reqPrompt.match(httpsRegexp);

    if (sconfig.chat_switch.disable_https_crawler) {
        console.debug('https create new material is disabled, skip prompt enhance');
        return reqPrompt;
    }

    if (urls) {
        urls.forEach((url) => {
            if (!pinnedUrls.includes(url)) {
                pinnedUrls.push(url);
            }
        });
    }

    if (pinnedUrls.length === 0) {
        return reqPrompt;
    }

    let urlEle = '';
    for (const url of pinnedUrls) {
        urlEle += `<p><i class="bi bi-trash"></i> <a href="${url}" class="link-primary" target="_blank">${url}</a></p>`;
    }

    // save to storage
    // FIXME save to session config
    await libs.KvSet(KvKeyPinnedMaterials, urlEle);
    await restorePinnedMaterials();

    // re generate reqPrompt
    reqPrompt = reqPrompt.replace(httpsRegexp, '');
    reqPrompt += '\nrefs:\n- ' + pinnedUrls.join('\n- ');
    return reqPrompt;
}

async function restorePinnedMaterials () {
    const urlEle = await libs.KvGet(KvKeyPinnedMaterials) || '';
    const container = document.querySelector('#chatContainer .pinned-refs');
    container.innerHTML = urlEle;

    // bind to remove pinned materials
    document.querySelectorAll('#chatContainer .pinned-refs p .bi-trash')
        .forEach((item) => {
            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const evtTarget = libs.evtTarget(evt);

                const container = evtTarget.closest('.pinned-refs');
                const ele = evtTarget.closest('p');
                ele.parentNode.removeChild(ele);

                // update storage
                await libs.KvSet(KvKeyPinnedMaterials, container.innerHTML);
            })
        })
}

function getPinnedMaterials () {
    const urls = []
    document.querySelectorAll('#chatContainer .pinned-refs a')
        .forEach((item) => {
            urls.push(item.innerHTML);
        })

    return urls;
}

async function sendGptImage1EditPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    if (chatVisionSelectedFileStore.length === 0) {
        throw new Error('No images provided for image-to-image generation.');
    }

    const formData = new FormData();
    formData.append('prompt', prompt);
    formData.append('model', selectedModel);
    formData.append('quality', 'high');
    formData.append('size', '1024x1024');

    // Append images and update user input display
    let attachHTML = '';
    for (const item of chatVisionSelectedFileStore) {
        const blob = base64ToBlob(item.contentB64, 'image/png'); // Assuming PNG, adjust if needed
        formData.append('image[]', blob, item.filename);
        attachHTML += `<img src="data:image/png;base64,${item.contentB64}" data-name="${item.filename}">`;
    }

    // Insert images into user history
    const userChatData = await getChatData(chatID, RoleHuman);
    await saveChats2Storage({
        role: RoleHuman,
        chatID,
        content: userChatData.content, // Keep original text prompt
        attachHTML
    });

    // Update user input display in the UI
    const userTextElement = chatContainer.querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start`);
    if (userTextElement) {
        userTextElement.insertAdjacentHTML('beforeend', attachHTML);
    }

    // Clear the store after processing
    chatVisionSelectedFileStore = [];
    updateChatVisionSelectedFileStore();

    const sconfig = await getChatSessionConfig();
    try {
        const resp = await fetch('/oneapi/v1/images/edits', {
            method: 'POST',
            headers: {
                Authorization: `Bearer ${sconfig.api_token}`,
                'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
                'X-Laisky-Api-Base': sconfig.api_base
                // Content-Type is set automatically by FormData
            },
            body: formData
        });

        if (!resp.ok) {
            throw new Error(`[${resp.status}]: ${await resp.text()}`);
        }

        const requestid = resp.headers.get('x-oneapi-request-id') || '';
        const respData = await resp.json();

        let resultHTML = '';
        if (respData.data && respData.data.length > 0) {
            respData.data.forEach(item => {
                let imgSrc = '';
                if (item.b64_json) {
                    imgSrc = `data:image/png;base64,${item.b64_json}`;
                } else if (item.url) {
                    imgSrc = item.url; // Assuming direct URL if b64_json is not present
                }

                if (imgSrc) {
                    resultHTML += `<div class="ai-resp-image">
                        <div class="hover-btns">
                            <i class="bi bi-pencil-square"></i>
                        </div>
                        <img src="${imgSrc}">
                    </div>`;
                }
            });
        } else {
            throw new Error('No image data received in the response.');
        }

        currentAIRespEle.innerHTML = resultHTML;
        currentAIRespEle.dataset.taskStatus = ChatTaskStatusDone; // Mark as done immediately for direct response
        currentAIRespEle.dataset.taskType = ChatTaskTypeImage;

        // Save the result to storage
        const chatData = await getChatData(chatID, RoleAI) || {};
        chatData.chatID = chatID;
        chatData.role = RoleAI;
        chatData.model = selectedModel;
        chatData.content = resultHTML; // Save the rendered HTML
        chatData.taskStatus = ChatTaskStatusDone;
        chatData.taskType = ChatTaskTypeImage;
        chatData.requestid = requestid;
        await saveChats2Storage(chatData);

        // Final rendering steps
        renderAfterAiResp(chatData, false); // Render without saving again
    } catch (err) {
        console.error('Error sending gpt-image-1 edit request:', err);
        await abortAIResp(`Failed to generate image edit: ${renderError(err)}`);
    } finally {
        unlockChatInput(); // Ensure input is unlocked even on error
    }
}

/**
 * Sends an txt2image prompt to the server for the selected model and updates the current AI response element with the task information.
 * @param {string} chatID - The chat ID.
 * @param {string} selectedModel - The selected image model.
 * @param {HTMLElement} currentAIRespEle - The current AI response element to update with the task information.
 * @param {string} prompt - The image prompt to send to the server.
 * @throws {Error} Throws an error if the selected model is unknown or if the response from the server is not ok.
 */
async function sendTxt2ImagePrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    const url = '/images/generations';
    const sconfig = await getChatSessionConfig();
    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: JSON.stringify({
            model: selectedModel,
            prompt
        })
    });
    if (!resp.ok || resp.status !== 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
    currentAIRespEle.dataset.taskType = ChatTaskTypeImage;
    // currentAIRespEle.dataset.model = selectedModel;
    // currentAIRespEle.dataset.taskId = respData.task_id;
    // currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // let attachHTML = '';
    // respData.image_urls.forEach((url) => {
    //     attachHTML += `<div class="ai-resp-image">
    //     <div class="hover-btns">
    //     <i class="bi bi-pencil-square"></i>
    //     </div>
    //     <img src="${url}">
    //     </div>`;
    // })

    const chatData = await getChatData(chatID, RoleAI) || {};
    chatData.chatID = chatID;
    chatData.role = RoleAI;
    chatData.model = selectedModel;
    // chatData.content = attachHTML;
    chatData.taskId = respData.task_id;
    chatData.taskStatus = ChatTaskStatusWaiting;
    chatData.taskType = ChatTaskTypeImage;
    chatData.taskData = respData.image_urls;

    // save img to storage no matter it's done or not
    await saveChats2Storage(chatData);
}

async function sendSdxlturboPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
    case ImageModelSdxlTurbo:
        url = '/images/generations/sdxl-turbo';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    let imageBase64 = '';
    if (chatVisionSelectedFileStore.length !== 0) {
        imageBase64 = chatVisionSelectedFileStore[0].contentB64;

        // insert image to user input & hisotry
        await appendImg2UserInput(chatID, imageBase64, `${libs.DateStr()}.png`);

        chatVisionSelectedFileStore = [];
        updateChatVisionSelectedFileStore();
    }

    const sconfig = await getChatSessionConfig();
    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: JSON.stringify({
            model: selectedModel,
            text: prompt,
            image: imageBase64
        })
    });
    if (!resp.ok || resp.status !== 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
    currentAIRespEle.dataset.taskType = ChatTaskTypeImage;
    // currentAIRespEle.dataset.model = selectedModel;
    // currentAIRespEle.dataset.taskId = respData.task_id;
    // currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // save img to storage no matter it's done or not
    // let attachHTML = '';
    // respData.image_urls.forEach((url) => {
    //     attachHTML += `<div class="ai-resp-image">
    //         <div class="hover-btns">
    //             <i class="bi bi-pencil-square"></i>
    //         </div>
    //         <img src="${url}">
    //     </div>`
    // });

    const chatData = await getChatData(chatID, RoleAI) || {};
    chatData.chatID = chatID;
    chatData.role = RoleAI;
    chatData.model = selectedModel;
    // chatData.content = attachHTML;
    chatData.taskId = respData.task_id;
    chatData.taskStatus = ChatTaskStatusWaiting;
    chatData.taskType = ChatTaskTypeImage;
    chatData.taskData = respData.image_urls;

    // save img to storage no matter it's done or not
    await saveChats2Storage(chatData);
}

async function sendFluxProPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    const nImage = parseInt(document.getElementById('selectDrawNImage').value);
    const url = `/images/generations/flux/${selectedModel}`;
    console.debug(`sendFluxProPrompt2Server, url=${url}`);

    const sconfig = await getChatSessionConfig();

    const payload = {
        input: {
            prompt,
            steps: 30,
            aspect_ratio: '1:1',
            height: 1440,
            width: 1440,
            safety_tolerance: 5,
            guidance: 3,
            interval: 2,
            seed: Date.now(),
            n_images: nImage
        }
    };

    // add image_prompt for vision model
    if (VisionModels.includes(selectedModel)) {
        // get first image in store
        if (chatVisionSelectedFileStore.length !== 0) {
            const imageBase64 = chatVisionSelectedFileStore[0].contentB64;

            // insert image to user input & hisotry
            await appendImg2UserInput(chatID, imageBase64, `${libs.DateStr()}.png`);

            chatVisionSelectedFileStore = [];
            updateChatVisionSelectedFileStore();

            // https://replicate.com/black-forest-labs/flux-pro/api/learn-more#option-3-data-uri
            payload.input.image_prompt = `data:application/octet-stream;base64,${imageBase64}`;
        }
    }

    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: JSON.stringify(payload)
    });
    if (!resp.ok || resp.status !== 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
    currentAIRespEle.dataset.taskType = ChatTaskTypeImage;
    // currentAIRespEle.dataset.model = selectedModel;
    // currentAIRespEle.dataset.taskId = respData.task_id;
    // currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // save img to storage no matter it's done or not
    // let attachHTML = '';
    // respData.image_urls.forEach((url) => {
    //     attachHTML += `<div class="ai-resp-image">
    //         <div class="hover-btns">
    //             <i class="bi bi-pencil-square"></i>
    //         </div>
    //         <img src="${url}">
    //     </div>`
    // });

    const chatData = await getChatData(chatID, RoleAI) || {};
    chatData.chatID = chatID;
    chatData.role = RoleAI;
    chatData.model = selectedModel;
    // chatData.content = attachHTML;
    chatData.taskId = respData.task_id;
    chatData.taskStatus = ChatTaskStatusWaiting;
    chatData.taskType = ChatTaskTypeImage;
    chatData.taskData = respData.image_urls;

    // save img to storage no matter it's done or not
    await saveChats2Storage(chatData);
}

// =====================================
// DO NOT DELETE THIS COMMENTED CODE
// THERE IS NO IMG-2-IMG MODEL NOW, BUT IT MAY BE USED IN THE FUTURE
// =====================================
// async function sendImg2ImgPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
//     let url;
//     switch (selectedModel) {
//     case ImageModelImg2Img:
//         url = '/images/generations/lcm';
//         break;
//     default:
//         throw new Error(`unknown image model: ${selectedModel}`);
//     }

//     // get first image in store
//     if (chatVisionSelectedFileStore.length === 0) {
//         throw new Error('no image selected');
//     }
//     const imageBase64 = chatVisionSelectedFileStore[0].contentB64;

//     // insert image to user input & hisotry
//     await appendImg2UserInput(chatID, imageBase64, `${libs.DateStr()}.png`);

//     chatVisionSelectedFileStore = [];
//     updateChatVisionSelectedFileStore();

//     const sconfig = await getChatSessionConfig();
//     const resp = await fetch(url, {
//         method: 'POST',
//         headers: {
//             Authorization: 'Bearer ' + sconfig.api_token,
//             'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
//             'X-Laisky-Api-Base': sconfig.api_base
//         },
//         body: JSON.stringify({
//             model: selectedModel,
//             prompt,
//             image_base64: imageBase64
//         })
//     });
//     if (!resp.ok || resp.status !== 200) {
//         throw new Error(`[${resp.status}]: ${await resp.text()}`);
//     }
//     const respData = await resp.json();

//     currentAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
//     currentAIRespEle.dataset.taskType = 'image';
//     currentAIRespEle.dataset.model = selectedModel;
//     currentAIRespEle.dataset.taskId = respData.task_id;
//     currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

//     // save img to storage no matter it's done or not
//     let attachHTML = '';
//     respData.image_urls.forEach((url) => {
//         attachHTML += `<img src="${url}">`;
//     })

//     const chatData = await libs.KvGet(`${KvKeyChatData}${RoleAI}_${chatID}`) || {};
//     chatData.chatID = chatID;
//     chatData.role = RoleAI;
//     chatData.model = selectedModel;
//     chatData.content = attachHTML;
//     chatData.taskId = respData.task_id;
//     chatData.taskStatus = ChatTaskStatusWaiting;
//     chatData.taskType = ChatTaskTypeImage;

//     // save img to storage no matter it's done or not
//     await saveChats2Storage(chatData);
// }

/**
 * Sends a prompt to the server for the selected model and updates the current AI response element with the task information.
 *
 * @param {string} model - The selected model.
 * @param {string} prompt - The prompt to send to the server.
 * @returns chat/complete/image/qa/unknown
 */
async function detectPromptTaskType (model, prompt) {
    if (model === ChatModelDeepResearch) {
        return 'deepresearch';
    } else if (IsChatModel(model)) {
        const sconfig = await getChatSessionConfig();
        if (sconfig.chat_switch.all_in_one) {
            try {
                const resp = await fetch('/api', {
                    headers: {
                        'Content-Type': 'application/json',
                        Authorization: 'Bearer ' + sconfig.api_token,
                        'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
                        'X-Laisky-Api-Base': sconfig.api_base
                    },
                    method: 'POST',
                    body: JSON.stringify({
                        model: DefaultModel,
                        max_tokens: 50,
                        stream: false,
                        messages: [
                            {
                                role: RoleSystem,
                                content: 'Please determine the user\'s intent based on the prompt. Return "image" if the user wants to generate an image, otherwise return "text". Do not provide any other irrelevant content except "image"/"text".'
                            },
                            {
                                role: RoleHuman,
                                content: prompt
                            }
                        ]
                    })
                });

                const respData = await resp.json();
                if (respData.choices[0].message.content.includes('image')) {
                    return 'image';
                }
            } catch (err) {
                console.warn(`failed to request llm to detect prompt task type: ${renderError(err)}`)
            }
        }

        return 'chat';
    } else if (IsCompletionModel(model)) {
        return 'complete';
    } else if (IsQaModel(model)) {
        return 'qa';
    } else if (IsImageModel(model)) {
        return 'image';
    } else {
        return 'unknown';
    }
}

async function appendImg2UserInput (chatID, imgDataBase64, imgName) {
    // insert image to user hisotry
    const text = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start pre`).innerHTML;
    await saveChats2Storage({
        role: RoleHuman,
        chatID,
        content: text,
        attachHTML: `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`
    });

    // insert image to user input
    chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start`)
        .insertAdjacentHTML(
            'beforeend',
            `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`
        );
}

/**
 * Creates and connects to a Server-Sent Events (SSE) endpoint.
 * @param {string} endpoint - The SSE endpoint URL.
 * @param {object} options - Options for the SSE connection.
 * @returns {object} The SSE object.
 */
// function createAndConnectSSE (endpoint, options) {
//     const sse = new window.SSE(endpoint, options);

//     // Add Safari-specific error and reconnection handling
//     if (/^((?!chrome|android).)*safari/i.test(navigator.userAgent)) {
//         console.debug('Safari SSE connection handling activated');

//         // Keep tabs on connection health
//         safariSSEHeartbeatTimer = setInterval(() => {
//             if (!globalAIRespSSE || !globalAIRespHeartBeatTimer) return;

//             // If no heartbeat for 30 seconds in Safari, reconnect
//             if (Date.now() - globalAIRespHeartBeatTimer > 30000) {
//                 console.warn('Safari SSE connection heartbeat timeout, attempting reconnection');
//                 try {
//                     // Clean up existing connection
//                     if (globalAIRespSSE) {
//                         globalAIRespSSE.close();
//                     }

//                     // Only retry a limited number of times
//                     if (safariSSERetryCount < MAX_SSE_RETRIES) {
//                         safariSSERetryCount++;

//                         // Create new connection with the same parameters
//                         globalAIRespSSE = createAndConnectSSE('/api', options);
//                     } else {
//                         console.error('Safari SSE max retries exceeded');
//                         abortAIResp('Connection failed after multiple attempts. Please try again.');
//                         clearInterval(safariSSEHeartbeatTimer);
//                     }
//                 } catch (e) {
//                     console.error('Safari SSE reconnection failed:', e);
//                     abortAIResp('Connection failed after multiple attempts. Please try again.');
//                     clearInterval(safariSSEHeartbeatTimer);
//                 }
//             }
//         }, 5000);
//     }

//     // Add enhanced error handling
//     sse.addEventListener('error', async (err) => {
//         console.warn('SSE error:', err);

//         // Try to extract any available error information
//         let errorMessage = 'Connection error';
//         if (err && err.data) {
//             errorMessage = err.data;
//         }

//         // Safari-specific handling
//         if (/^((?!chrome|android).)*safari/i.test(navigator.userAgent)) {
//             if (sseMessageFragmented && safariSSERetryCount < MAX_SSE_RETRIES) {
//                 console.warn('Safari SSE reconnection attempt during fragmentation');
//                 safariSSERetryCount++;

//                 // Wait briefly - Safari sometimes needs this delay
//                 setTimeout(() => {
//                     if (globalAIRespSSE) {
//                         globalAIRespSSE.close();
//                     }

//                     // Create new connection with same parameters
//                     globalAIRespSSE = createAndConnectSSE('/api', options);
//                 }, 1000);
//                 return;
//             }
//         }

//         await abortAIResp(`Failed to receive chat response: ${errorMessage}`);
//     });

//     // Add connection success handler to reset retry count
//     sse.addEventListener('open', () => {
//         console.debug('SSE connection established');
//         safariSSERetryCount = 0;
//         globalAIRespHeartBeatTimer = Date.now();
//     });

//     // Start the connection
//     sse.stream();
//     return sse;
// }

/**
 * Sends a chat prompt to the server for the selected model and updates the current AI response element with the task information.
 *
 * @param {string} chatID - The chat ID.
 * @param {string} reqPrompt - Optional. The chat prompt to send to the server.
 *
 * @returns {string} The chat ID.
 */
async function sendChat2Server (chatID, reqPrompt) {
    let selectedModel = await OpenaiSelectedModel();
    if (!chatID) { // if chatID is empty, it's a new request
        chatID = newChatID();

        if (!reqPrompt) {
            reqPrompt = libs.TrimSpace(chatPromptInputEle.value || '');
        }

        if (chatPromptInputEle) {
            chatPromptInputEle.value = '';
        }

        // if prompt is empty, just ignore it.
        //
        // it is unable to just send image without text,
        // because claude will return error if prompt is empty.
        if (reqPrompt === '') {
            return chatID;
        }

        append2Chats(false,
            {
                chatID,
                role: RoleHuman,
                content: reqPrompt,
                model: selectedModel,
                isHistory: false
            });
        await saveChats2Storage({
            role: RoleHuman,
            chatID,
            model: selectedModel,
            content: reqPrompt
        });
    } else { // if chatID is not empty, it's a reload request
        reqPrompt = chatContainer
            .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start pre`).innerHTML;
    }

    // extract and pin new material in chat
    if (reqPrompt !== '') {
        reqPrompt = await userPromptEnhance(reqPrompt);
    }

    globalAIRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    lockChatInput();

    // get chatmodel from url parameters
    if (location.search) {
        const params = new URLSearchParams(location.search);
        if (params.has('chatmodel')) {
            selectedModel = params.get('chatmodel');
        }
    }

    // these extras will append to the tail of AI's response
    globalAIRespData = {
        chatID,
        role: RoleAI,
        content: '',
        attachHTML: '',
        rawContent: '',
        isThinking: false,
        reasoningContent: '',
        costUsd: '',
        model: selectedModel,
        requestid: '',
        taskType: ChatTaskTypeChat,
        taskStatus: null,
        taskId: null
    };
    let reqBody = null;
    const sconfig = await getChatSessionConfig();

    let messages;
    const nContexts = parseInt(sconfig.n_contexts);
    let url, project;
    const urlParams = new URLSearchParams(location.search);

    const promptType = await detectPromptTaskType(selectedModel, reqPrompt);
    console.debug(`detected prompt type ${promptType}`);

    switch (promptType) {
    case 'chat':
        messages = await getLastNChatMessages(nContexts, chatID);
        if (reqPrompt !== '') {
            messages.push({
                role: RoleHuman,
                content: reqPrompt
            });
        } else {
            messages.push({
                role: RoleHuman
            });
        }

        // there are pinned files, add them to user's prompt
        if (VisionModels.includes(selectedModel) && chatVisionSelectedFileStore.length > 0) {
            if (messages.length === 0) {
                // This should not happen if prompt is always present
                messages.push({ role: RoleHuman, content: '' });
            }

            // Ensure the last message has a files array
            if (!messages[messages.length - 1].files) {
                messages[messages.length - 1].files = [];
            }

            let accumulatedAttachHTML = '';
            const userChatDisplayElement = chatContainer
                .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start`);

            for (const item of chatVisionSelectedFileStore) {
                messages[messages.length - 1].files.push({
                    type: 'image',
                    name: item.filename,
                    content: item.contentB64
                });

                const singleImageHTML = `<img src="data:image/png;base64,${item.contentB64}" data-name="${item.filename}">`;
                accumulatedAttachHTML += singleImageHTML;

                // Insert image into the live DOM for immediate display
                if (userChatDisplayElement) {
                    userChatDisplayElement.insertAdjacentHTML('beforeend', singleImageHTML);
                } else {
                    console.warn(`User display element not found for chatID ${chatID} when appending image to DOM.`);
                }
            }

            // The text content should have been set when the user's message was initially created.
            // For a new request, reqPrompt is the initial prompt.
            // For a reload, reqPrompt is the *edited* prompt text.
            const currentTextContent = reqPrompt;

            // Prepare the updated chat data for storage
            // Append new images to any existing attachHTML.
            const updatedUserChatData = {
                role: RoleHuman,
                chatID,
                content: currentTextContent,
                attachHTML: accumulatedAttachHTML // Ensure attachHTML is set to the freshly built accumulatedAttachHTML
            };

            // Save the user's message to storage once, with all images included in attachHTML
            await saveChats2Storage(updatedUserChatData);

            chatVisionSelectedFileStore = [];
            updateChatVisionSelectedFileStore();
        }

        reqBody = JSON.stringify({
            model: selectedModel,
            stream: true,
            max_tokens: parseInt(sconfig.max_tokens),
            temperature: parseFloat(sconfig.temperature),
            presence_penalty: parseFloat(sconfig.presence_penalty),
            frequency_penalty: parseFloat(sconfig.frequency_penalty),
            messages,
            stop: ['\n\n'],
            laisky_extra: {
                chat_switch: sconfig.chat_switch
            }
        });
        break;
    case 'complete':
        reqBody = JSON.stringify({
            model: selectedModel,
            stream: true,
            max_tokens: parseInt(sconfig.max_tokens),
            temperature: parseFloat(sconfig.temperature),
            presence_penalty: parseFloat(sconfig.presence_penalty),
            frequency_penalty: parseFloat(sconfig.frequency_penalty),
            prompt: reqPrompt,
            stop: ['\n\n']
        });
        break;
    case 'qa':
        // {
        //     "question": "XFS 是干啥的",
        //     "text": " XFS is a simple CLI tool that can be used to create volumes/mounts and perform simple filesystem operations.\n",
        //     "url": "http://xego-dev.basebit.me/doc/xfs/support/xfs2_cli_instructions/"
        // }

        switch (selectedModel) {
        case QAModelBasebit:
        case QAModelSecurity:
        case QAModelImmigrate:
            window.data.qa_chat_models.forEach((item) => {
                if (item.name === selectedModel) {
                    url = item.url;
                    project = item.project;
                }
            })

            if (!project) {
                console.error("can't find project name for chat model: " + selectedModel);
                return chatID;
            }

            url = `${url}?p=${project}&q=${encodeURIComponent(reqPrompt)}`;
            break;
        case QAModelCustom:
            url = `/ramjet/gptchat/ctx/search?q=${encodeURIComponent(reqPrompt)}`;
            break;
        case QAModelShared:
            // example url:
            //
            // https://chat2.laisky.com/?chatmodel=qa-shared&uid=public&chatbot_name=default

            url = `/ramjet/gptchat/ctx/share?uid=${urlParams.get('uid')}` +
                        `&chatbot_name=${urlParams.get('chatbot_name')}` +
                        `&q=${encodeURIComponent(reqPrompt)}`;
            break;
        default:
            console.error('unknown qa chat model: ' + selectedModel);
        }

        globalAIRespEle.scrollIntoView({ behavior: 'smooth' });
        try {
            const resp = await fetch(url, {
                method: 'GET',
                cache: 'no-cache',
                headers: {
                    Connection: 'keep-alive',
                    'Content-Type': 'application/json',
                    Authorization: 'Bearer ' + sconfig.api_token,
                    'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
                    'X-Laisky-Api-Base': sconfig.api_base,
                    'X-PDFCHAT-PASSWORD': await libs.KvGet(KvKeyCustomDatasetPassword)
                }
            });

            if (!resp.ok || resp.status !== 200) {
                throw new Error(`[${resp.status}]: ${await resp.text()}`);
            }

            const data = await resp.json();
            if (!data || !data.text) {
                await abortAIResp('cannot gather sufficient context to answer the question');
                return chatID;
            }

            globalAIRespData.attachHTML = `
                    <p style="margin-bottom: 0;">
                        <button class="btn btn-info" type="button" data-bs-toggle="collapse" data-bs-target="#chatRef-${chatID}" aria-expanded="false" aria-controls="chatRef-${chatID}" style="font-size: 0.6em">
                            > toggle reference
                        </button>
                    </p>`;

            if (data.url) {
                globalAIRespData.attachHTML += `
                    <div>
                        <div class="collapse" id="chatRef-${chatID}">
                            <div class="card card-body">${combineRefs(data.url)}</div>
                        </div>
                    </div>`;
            }

            const messages = [{
                role: RoleHuman,
                content: `Use the following pieces of context to answer the users question.
                    the context that help you answer the question is between ">>>>>>>" and "<<<<<<<",
                    the user' question that you should answer in after "<<<<<<<".
                    you should directly answer the user's question, and you can use the context to help you answer the question.

                    >>>>>>>
                    context: ${data.text}
                    <<<<<<<

                    question: ${reqPrompt}
                    `
            }];
            const model = DefaultModel; // rewrite chat model

            reqBody = JSON.stringify({
                model,
                stream: true,
                max_tokens: parseInt(sconfig.max_tokens),
                temperature: parseFloat(sconfig.temperature),
                presence_penalty: parseFloat(sconfig.presence_penalty),
                frequency_penalty: parseFloat(sconfig.frequency_penalty),
                messages,
                stop: ['\n\n']
            });
        } catch (err) {
            await abortAIResp(`failed to fetch qa data: ${renderError(err)}`);
            return chatID;
        }

        break;
    case 'image':
        if (!IsImageModel(selectedModel) && sconfig.chat_switch.all_in_one) {
            selectedModel = ImageModelFluxSchnell;
        }

        try {
            switch (selectedModel) {
            case ImageModelDalle3:
            case ImageModelImagen3:
            case ImageModelImagen3Fast:
                await sendTxt2ImagePrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
                // case ImageModelImg2Img:
                //     await sendImg2ImgPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                //     break;
            case ImageModelGptImage1:
                if (chatVisionSelectedFileStore.length > 0) {
                    // Image-to-Image mode
                    await sendGptImage1EditPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                } else {
                    // Text-to-Image mode
                    await sendTxt2ImagePrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                }
                break;
            case ImageModelSdxlTurbo:
                await sendSdxlturboPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
                // case ImageModelFluxPro:
            case ImageModelFluxPro11:
            case ImageModelFluxDev:
            case ImageModelFluxProUltra11:
            case ImageModelFluxSchnell:
                await sendFluxProPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            default:
                throw new Error(`unknown image model: ${selectedModel}`);
            }
        } catch (err) {
            await abortAIResp(`failed to send image prompt: ${renderError(err)}`);
        } finally {
            unlockChatInput();
        }

        return chatID;
    case 'deepresearch': //  new case for deep-research
        try {
            const resp = await fetch('/deepresearch', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: 'Bearer ' + sconfig.api_token,
                    'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
                    'X-Laisky-Api-Base': sconfig.api_base
                },
                body: JSON.stringify({ prompt: reqPrompt })
            });

            if (!resp.ok || resp.status !== 200) {
                throw new Error(`[${resp.status}]: ${await resp.text()}`);
            }

            const respData = await resp.json();
            console.log('deepresearch', respData)
            globalAIRespEle.dataset.taskStatus = ChatTaskStatusWaiting;
            globalAIRespEle.dataset.taskType = ChatTaskTypeDeepResearch;

            // Save task ID and other relevant data to chatData
            const chatData = await getChatData(chatID, RoleAI) || {};
            chatData.chatID = chatID;
            chatData.role = RoleAI;
            chatData.model = selectedModel;
            chatData.taskId = respData.task_id;
            chatData.taskStatus = ChatTaskStatusWaiting;
            chatData.taskType = ChatTaskTypeDeepResearch;

            await saveChats2Storage(chatData);

            globalAIRespEle.innerHTML = `
                <p dir="auto" class="card-text placeholder-glow">
                    <span class="placeholder col-7"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-6"></span>
                    <span class="placeholder col-8"></span>
                </p>`;
        } catch (err) {
            await abortAIResp(`failed to send deepresearch prompt: ${renderError(err)}`);
        } finally {
            unlockChatInput();
        }
        return chatID;
    default:
        globalAIRespEle.innerHTML = '<p>🔥Someting in trouble...</p>' +
                '<pre style="text-wrap: pretty;">' +
                `unimplemented model: ${libs.sanitizeHTML(selectedModel)}</pre>`;
        await saveChats2Storage({
            role: RoleAI,
            chatID,
            model: selectedModel,
            content: globalAIRespEle.innerHTML
        });
        unlockChatInput();
        return chatID;
    }

    if (!reqBody) {
        return chatID;
    }

    globalAIRespHeartBeatTimer = Date.now();
    globalAIRespSSE = new window.SSE('/api', {
        headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        method: 'POST',
        payload: reqBody
    });

    userManuallyScrolled = false;
    let isChatRespDone = false;

    // Modify the message event handler portion of sendChat2Server
    globalAIRespSSE.addEventListener('message', async (evt) => {
        evt.stopPropagation();
        globalAIRespHeartBeatTimer = Date.now();

        // set request id to ai response element
        if (!globalAIRespData.requestid) {
            const ids = evt.headers['x-oneapi-request-id'] || [];
            if (ids.length > 0) {
                globalAIRespData.requestid = ids[0];
            }
        }

        let data = evt.data;
        console.debug(`received stream data: ${data}`);

        // Special message handling
        if (data === '[DONE]') {
            sseMessageBuffer = '';
            sseMessageFragmented = false;
            isChatRespDone = true;

            if (globalAIRespSSE) {
                globalAIRespSSE.close();
                globalAIRespSSE = null;
                unlockChatInput();
            }

            console.debug(`chat response done for chat ${chatID}`);
            await renderAfterAiResp(globalAIRespData, true);
            return chatID;
        } else if (data === '[HEARTBEAT]') {
            return chatID;
        }

        // remove prefix [HEARTBEAT]
        data = data.replace(/^\[HEARTBEAT\]+/, '');

        // Handle message fragmentation
        let payload = null;
        try {
            // Try to parse as is first
            payload = JSON.parse(data);
            // If successful, reset buffer
            sseMessageBuffer = '';
            sseMessageFragmented = false;
        } catch (e) {
            console.debug('Received potentially fragmented message, buffering');

            // Message might be fragmented, append to buffer
            sseMessageBuffer += data;
            sseMessageFragmented = true;

            // Try to parse the accumulated buffer
            try {
                payload = JSON.parse(sseMessageBuffer);
                // If successful parsing the buffer, clear it
                sseMessageBuffer = '';
                sseMessageFragmented = false;
            } catch (bufferErr) {
                // Buffer still doesn't contain valid JSON, wait for more fragments
                console.debug('Buffer still incomplete, waiting for more fragments');
                return chatID;
            }
        }

        // If we get here, we have a valid payload
        if (payload && !sseMessageFragmented) {
            try {
                const { respChunk, reasoningChunk, annotations } = parseChatResp(selectedModel, payload);

                // Add to our global data store
                globalAIRespData.reasoningContent += reasoningChunk;
                globalAIRespData.rawContent += respChunk;

                // Store annotations if they exist
                if (annotations && annotations.length > 0) {
                    // Initialize annotations array if it doesn't exist
                    if (!globalAIRespData.annotations) {
                        globalAIRespData.annotations = [];
                    }

                    // Append new annotations, avoiding duplicates
                    annotations.forEach(annotation => {
                        // Only add if it's not already in the array
                        if (!globalAIRespData.annotations.some(a =>
                            a.type === annotation.type &&
                            a.url_citation?.url === annotation.url_citation?.url)) {
                            globalAIRespData.annotations.push(annotation);
                        }
                    });
                }

                if (payload.choices[0].finish_reason) {
                    isChatRespDone = true;
                }

                // Handle rendering differently based on task status
                if (reasoningChunk || respChunk) {
                    switch (globalAIRespEle.dataset.taskStatus) {
                    case 'waiting':
                        // First response - initialize the containers
                        globalAIRespEle.dataset.taskStatus = 'writing';
                        globalAIRespEle.innerHTML = '';

                        // Create containers for reasoning and response once
                        if (globalAIRespData.reasoningContent) {
                            const reasoningHTML = `
                                <div class="thinking-container">
                                    <p class="d-inline-flex gap-1">
                                        <button class="btn btn-secondary" type="button" data-bs-toggle="collapse"
                                            data-bs-target="#chatReasoning_${chatID}" aria-expanded="true"
                                            aria-controls="chatReasoning_${chatID}">
                                            Thinking...
                                        </button>
                                    </p>
                                    <div class="collapse show" id="chatReasoning_${chatID}">
                                        <div class="card card-body reasoning-content"></div>
                                    </div>
                                </div>`;
                            globalAIRespEle.insertAdjacentHTML('beforeend', reasoningHTML);
                            reasoningContainer = globalAIRespEle.querySelector('.reasoning-content');

                            // Initialize reasoning container with current content
                            if (globalAIRespData.reasoningContent.trim()) {
                                reasoningContainer.innerHTML = await renderHTML(globalAIRespData.reasoningContent, true);
                            }
                        }

                        // Create response container
                        globalAIRespEle.insertAdjacentHTML('beforeend', '<div class="response-content"></div>');
                        responseContainer = globalAIRespEle.querySelector('.response-content');

                        // Add initial content
                        if (respChunk) {
                            responseContainer.innerHTML = respChunk;
                        }
                        break;
                    case 'writing':
                        // Incremental updates - only render the new chunks
                        if (reasoningChunk && reasoningContainer) {
                            // For small chunks, just append plaintext for performance
                            if (reasoningChunk.length < 50 && !reasoningChunk.includes('\n')) {
                                reasoningContainer.insertAdjacentText('beforeend', reasoningChunk);
                            } else {
                                // For larger or formatted chunks, re-render the full content
                                reasoningContainer.innerHTML = await renderHTML(globalAIRespData.reasoningContent, true);
                            }
                        }

                        if (respChunk && responseContainer) {
                            // For response content, more care is needed since it might be partial markdown
                            // We'll render the full content but only when necessary
                            if (payload.choices[0].finish_reason || respChunk.includes('\n\n')) {
                                // Complete section or paragraph - render the full content
                                responseContainer.innerHTML = await renderHTML(globalAIRespData.rawContent);
                            } else {
                                // Just append the plain text for efficiency
                                responseContainer.insertAdjacentText('beforeend', respChunk);
                            }
                        }

                        scrollToChat(globalAIRespEle);
                        break;
                    }
                }
            } catch (err) {
                await abortAIResp(`Error processing payload: ${renderError(err)}`);
            }
        }

        // When response is complete, ensure final rendering
        if (isChatRespDone) {
            sseMessageBuffer = '';
            sseMessageFragmented = false;

            // If we have annotations, render them as references
            if (globalAIRespData.annotations && globalAIRespData.annotations.length > 0) {
                const referencesHTML = parseAnnotationsAsRef(globalAIRespData.annotations);
                if (referencesHTML) {
                    // Create a collapsible references section similar to deepResearch format
                    globalAIRespData.attachHTML = (globalAIRespData.attachHTML || '') +
                        `<div>
                            <p class="d-inline-flex gap-1">
                                <button class="btn btn-primary btn-sm" type="button" data-bs-toggle="collapse"
                                    data-bs-target="#chatRefs_${chatID}" aria-expanded="false"
                                    aria-controls="chatRefs_${chatID}">
                                    References
                                </button>
                            </p>
                            <div class="collapse" id="chatRefs_${chatID}">
                                <div class="card card-body">
                                    ${referencesHTML}
                                </div>
                            </div>
                        </div>`;
                }
            }

            // Final rendering of the complete content
            if (responseContainer) {
                responseContainer.innerHTML = await renderHTML(globalAIRespData.rawContent, true);
            }

            if (globalAIRespSSE) {
                globalAIRespSSE.close();
                globalAIRespSSE = null;
                unlockChatInput();
            }

            console.debug(`chat response done for chat ${chatID}`);
            await renderAfterAiResp(globalAIRespData, true);
        }
    })

    globalAIRespSSE.addEventListener('error', async (err) => {
        console.warn('SSE error:', err);
        await abortAIResp(`Failed to receive chat response: ${renderError(err)}`);
    });

    globalAIRespSSE.stream();

    // Add this to the SSE class definition or modify it to track listeners:
    if (!window.SSE.prototype.listeners) {
        window.SSE.prototype.listeners = [];
        const originalAddEventListener = window.SSE.prototype.addEventListener;
        window.SSE.prototype.addEventListener = function (type, callback) {
            this.listeners.push({ type, callback });
            return originalAddEventListener.call(this, type, callback);
        };
    }

    return chatID;
}

/**
 * @type {boolean} mutex for renderHTML
 */
let muRenderHTML = false;
let latestRenderHTMLResult = '';

/**
 * Renders HTML content from markdown, with optional force rendering.
 * @param {string} markdown - The HTML content to render.
 * @param {boolean} force - Whether to force the rendering even if it's already in progress.
 * @return {Promise<string>} - The rendered HTML content.
 */
async function renderHTML (markdown, force = false) {
    if (!force && muRenderHTML) {
        return latestRenderHTMLResult || '';
    }
    muRenderHTML = true;

    try {
        latestRenderHTMLResult = await libs.Markdown2HTML(markdown);
        return latestRenderHTMLResult || '';
    } finally {
        muRenderHTML = false;
    }
}

/**
 * Aborts the current AI response and updates the current AI response element with the error message.
 *
 * @param {*} err
 * @returns {string}
 */
function renderError (err) {
    if (!err) {
        return 'unknown empty error';
    }

    if (err instanceof Error) {
        return err.message;
    }

    if (err.data) {
        return err.data;
    }

    try {
        return JSON.stringify(err);
    } catch (_) {
        return String(err);
    }
}

/**
 * do render and save chat after ai response finished
 *
 * @param {object} chatData - chat item
 *   @property {string} chatID - chat id
 *   @property {string} role - user or assistant
 *   @property {string} content - rendered chat content
 *   @property {string} attachHTML - chat response's attach html
 *   @property {string} rawContent - chat response's raw content
 *   @property {string} reasoningContent - chat response's reasoning content
 *   @property {string} costUsd - chat cost in USD
 *   @property {string} model - chat model
 *   @property {string} requestid - request id
 * @param {boolean} saveStorage - save to storage or not.
 *                                if it's restore chat, there is no need to save to storage.
 */
async function renderAfterAiResp (chatData, saveStorage = false) {
    const aiRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatData.chatID} .ai-response`);
    if (!aiRespEle) {
        console.warn(`can not find ai-response element for chatid=${chatData.chatID}`);
        return;
    }

    if (muRenderAfterAiRespForChatID[chatData.chatID]) {
        console.warn(`chat ${chatData.chatID} is already in rendering`);
        return;
    } else {
        muRenderAfterAiRespForChatID[chatData.chatID] = true;
    }

    const rawContent = chatData.rawContent || '';
    const attachHTML = chatData.attachHTML || '';

    if (rawContent && rawContent !== 'undefined') {
        let renderedHTML = '';
        if (chatData.reasoningContent && chatData.reasoningContent.trim() !== '') {
            const expanded = saveStorage ? 'true' : 'false';
            const showed = saveStorage ? ' show' : '';
            renderedHTML += `
                <div class="thinking-container">
                    <p class="d-inline-flex gap-1">
                        <button class="btn btn-secondary" type="button" data-bs-toggle="collapse"
                            data-bs-target="#chatReasoning_${chatData.chatID}" aria-expanded="${expanded}"
                            aria-controls="chatReasoning_${chatData.chatID}">
                            Thinking
                        </button>
                    </p>
                    <div class="collapse${showed}" id="chatReasoning_${chatData.chatID}">
                        <div class="card card-body reasoning-content">
                            ${await renderHTML(chatData.reasoningContent, true)}
                        </div>
                    </div>
                </div>`;
        }

        renderedHTML += await renderHTML(chatData.rawContent, true);
        latestRenderHTMLResult = '';

        aiRespEle.innerHTML = renderedHTML;
        aiRespEle.innerHTML += attachHTML;
    }

    // setup prism
    {
        // add line number
        aiRespEle.querySelectorAll('pre').forEach((item) => {
            item.classList.add('line-numbers');
        });
    }

    // setup mathjax
    try {
        if (!window.MathJax) {
            const script = document.createElement('script');
            script.src = 'https://s3.laisky.com/static/mathjax/2.7.3/MathJax-2.7.3/MathJax.js?config=TeX-MML-AM_CHTML';
            script.async = true;
            script.onload = () => {
                window.MathJax.Hub.Queue(['Typeset', window.MathJax.Hub]);
            };
            script.onerror = (e) => {
                console.error(`failed to load mathjax: ${e}`);
            };
            document.head.appendChild(script);
        } else {
            window.MathJax.Hub.Queue(['Typeset', window.MathJax.Hub]);
        }
    } catch (e) {
        console.error(`failed to render mathjax: ${e}`);
    }

    // should save html before prism formatted,
    // because prism.js do not support formatted html.
    chatData.content = aiRespEle.innerHTML;

    try {
        window.mermaid && await window.mermaid.run({ querySelector: 'pre.mermaid' });
    } catch (err) {
        console.error('mermaid run error:', err);
    }

    // add cost tips
    let costUsd = chatData.costUsd;
    const sconfig = await getChatSessionConfig();

    // First ensure the info div exists
    aiRespEle.insertAdjacentHTML('beforeend', `<div class="info"><i class="model">${chatData.model || ''}</i></div>`);
    const infoDiv = aiRespEle.querySelector('div.info');

    // Check if this is an API that provides cost information
    if (sconfig.api_token.startsWith('laisky-') ||
        sconfig.api_token.startsWith('sk-') ||
        sconfig.api_token.startsWith('FREETIER-')) {
        // Display existing cost if we already have it
        if (costUsd) {
            infoDiv.insertAdjacentHTML('beforeend', `<i class="cost">$${costUsd}</i>`);
        } else if (chatData.requestid) {
            // Otherwise fetch the cost if we have a request ID
            try {
                // Add placeholder while loading
                const costPlaceholder = document.createElement('i');
                costPlaceholder.className = 'cost cost-loading';
                costPlaceholder.textContent = '$ ...';
                infoDiv.appendChild(costPlaceholder);

                // Fetch cost information
                const resp = await fetch(`/oneapi/api/cost/request/${chatData.requestid}`);
                if (resp.ok) {
                    const data = await resp.json();
                    costUsd = data.cost_usd;

                    // Update the cost in the UI and save to chat data
                    if (costUsd) {
                        chatData.costUsd = costUsd;
                        costPlaceholder.textContent = `$${costUsd}`;
                        costPlaceholder.classList.remove('cost-loading');

                        // Save the updated cost to storage
                        if (saveStorage) {
                            await setChatData(chatData.chatID, chatData.role, chatData);
                        }
                    } else {
                        costPlaceholder.textContent = '$0.00';
                    }
                } else {
                    console.warn('Failed to fetch cost data:', resp.status);
                    costPlaceholder.textContent = '$?';
                }
            } catch (err) {
                console.error('Error fetching cost:', err);
                const costEle = infoDiv.querySelector('.cost');
                if (costEle) {
                    costEle.textContent = '$?';
                }
            }
        }
    }
    window.Prism.highlightAllUnder(aiRespEle);
    libs.EnableTooltipsEverywhere();

    // in the scenario of restore chat, the chatEle is already in view,
    // no need to scroll and save to storage
    if (saveStorage) {
        scrollToChat(aiRespEle);
        await saveChats2Storage(chatData);
    }

    bindImageOperationInAiResp(chatData.chatID);
    await addOperateBtnBelowAiResponse(chatData.chatID);
}

function bindImageOperationInAiResp (chatID) {
    const aiRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    if (!aiRespEle) {
        console.warn(`can not find ai-response element for chatid=${chatID}`);
        return;
    }

    const images = aiRespEle.querySelectorAll('.ai-resp-image') || [];
    for (const img of images) {
        const editBtn = img.querySelector('.hover-btns .bi-pencil-square');
        if (!editBtn) {
            continue;
        }

        editBtn.addEventListener('click', async (evt) => {
            evt.stopPropagation();
            const evtTarget = libs.evtTarget(evt);

            // read image data to base64 encoded str
            const imgUrl = evtTarget.closest('.ai-resp-image').querySelector('img').src;
            showImageEditModal(chatID, imgUrl);
        });
    }
}

/**
 * add operate buttons to ai response element after ai response finished
 *
 * @param {*} chatID - chat id
 */
async function addOperateBtnBelowAiResponse (chatID) {
    const aiRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    if (!aiRespEle) {
        console.warn(`can not find ai-response element for chatid=${chatID}`);
        return;
    }

    // Create a new div element just under ai-response
    const divContainer = document.createElement('div');
    divContainer.className = 'operator';
    aiRespEle.appendChild(divContainer);

    const chatData = await getChatData(chatID, RoleAI) || {};

    // add voice button
    divContainer.insertAdjacentHTML('beforeend', `
        <button type="button" class="btn btn-primary" data-fn="voice">
            <i class="bi bi-volume-up"></i>
        </button>
    `);
    divContainer.querySelector('.btn[data-fn="voice"]')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            let textContent = '';
            if (!chatData.rawContent) {
                console.warn(`can not find ai response or ai raw response for copy, chatid=${chatID}`);
                return;
            } else {
                textContent = chatData.rawContent;
            }

            await tts(chatID, textContent);
        });

    // add copy button
    divContainer.insertAdjacentHTML('beforeend', `
        <button type="button" class="btn btn-success" data-bs-toggle="tooltip" data-bs-placement="top" title="copy raw" data-fn="copy">
            <i class="bi bi-copy"></i>
        </button>
    `);
    divContainer.querySelector('button[data-fn="copy"]')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            const chatData = await getChatData(chatID, RoleAI) || {};

            // aiRespEle.dataset.copyBinded = true;
            let copyContent = '';
            if (!chatData.rawContent) {
                console.warn(`can not find ai response or ai raw response for copy, chatid=${chatID}`);
            } else {
                copyContent = chatData.rawContent;
            }

            if (!await libs.Copy2Clipboard(copyContent)) {
                alert('failed to copy, please copy manually');
            }
        });

    // add reload button
    divContainer.insertAdjacentHTML('beforeend', `
        <button type="button" class="btn btn-secondary" data-bs-toggle="tooltip" data-bs-placement="top" aria-label="reload" data-bs-original-title="reload" data-fn="reload">
            <i class="bi bi-arrow-clockwise" data-fn="reload"></i>
        </button>
    `);
    divContainer.querySelector('button[data-fn="reload"]')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            const evtTarget = libs.evtTarget(evt);

            // hide tooltip manually
            libs.DisableTooltipsEverywhere();

            const chatID = evtTarget.closest('.role-ai').dataset.chatid;
            // put image back to vision store
            putBackAttachmentsInUserInput(chatID);

            await reloadAiResp(chatID);
        });

    libs.EnableTooltipsEverywhere();
}

/**
 * request audio from tts server and play it
 *
 * @param {string} text - text to enhance
 */
async function tts (chatID, text) {
    const sconfig = await getChatSessionConfig();

    // fetch wav bytes from tts server, play it
    let audio;
    try {
        ShowSpinner();
        const url = `/audio/tts?apikey=${sconfig.api_token}&text=${encodeURIComponent(text)}`;
        const resp = await fetch(url, {
            method: 'GET'
        });

        // create audio element
        const respBlob = await resp.blob();
        const wavBlob = new Blob([respBlob], { type: 'audio/wav' });
        const wavUrl = URL.createObjectURL(wavBlob);
        audio = new Audio(wavUrl);
    } finally {
        HideSpinner();
    }

    // comment this block since of do not try auto play audio,
    // always showthe audio control widget.
    // try {
    //     // sometimes browser will block audio play,
    //     // try to play audio automately, if failed, add a play button.
    //     await audio.play();
    //     chatContainer.querySelector(`#${chatID} .ai-response .ai-resp-audio`)?.remove();
    //     return;
    // } catch (err) {
    //     console.error(`failed to play audio automately: ${renderError(err)}`);
    // }

    // for mobile device, autoplay is disabled, so we need to add a play button,
    // and play audio when user click the button.
    audio.controls = true;
    audio.playsinline = true;
    audio.preload = 'auto';
    audio.classList.add('ai-resp-audio');

    // replace the old audio element
    chatContainer.querySelector(`#${chatID} .ai-response .ai-resp-audio`)?.remove();
    chatContainer.querySelector(`#${chatID} .ai-response`)
        .insertAdjacentElement('beforeend', audio);

    // try autoplay
    try {
        await audio.play();
    } catch (err) {
        console.error(`failed to play audio: ${renderError(err)}`);
    }
}

/**
 * parse references to markdown links
 *
 * @param {Array} arr - references
 * @returns {string} markdown links
 */
function combineRefs (arr) {
    let markdown = '';
    for (const val of arr) {
        const sanitizedVal = libs.sanitizeHTML(decodeURIComponent(val)); // Sanitize the URL
        if (val.startsWith('https') || val.startsWith('http')) {
            // markdown += `- <${val}>\n`;
            markdown += `<li><a href="${sanitizedVal}">${sanitizedVal}</li>`;
        } else { // sometimes refs are just plain text, not url
            // markdown += `- \`${val}\`\n`;
            markdown += `<li><p>${sanitizedVal}</p></li>`;
        }
    }

    return `<ul style="margin-bottom: 0;">${markdown}</ul>`;
}

/**
 * Parses AI response annotations into formatted HTML references.
 * Handles url_citation annotations and creates an indexed reference list.
 *
 * @param {Array} annotations - Array of annotation objects from AI response
 * @returns {string} HTML unordered list with sorted, indexed references
 */
function parseAnnotationsAsRef (annotations) {
    if (!annotations || !Array.isArray(annotations) || annotations.length === 0) {
        return '';
    }

    // Create a map to track unique URLs with their citation order
    const uniqueRefs = new Map();
    let refIndex = 1;

    // Process each annotation
    for (const annotation of annotations) {
        // Only handle url_citation type annotations
        if (annotation.type === 'url_citation' && annotation.url_citation) {
            const { url, title } = annotation.url_citation;

            // Only add each URL once - keep first occurrence index
            if (url && !uniqueRefs.has(url)) {
                uniqueRefs.set(url, {
                    index: refIndex++,
                    title: title || url,
                    url
                });
            }
        }
    }

    // If no valid citations found, return empty string
    if (uniqueRefs.size === 0) {
        return '';
    }

    // Convert to HTML list items
    let refsHtml = '<ul class="reference-links">';
    uniqueRefs.forEach((details, url) => {
        const sanitizedUrl = libs.sanitizeHTML(decodeURIComponent(url));
        const sanitizedTitle = libs.sanitizeHTML(details.title);

        refsHtml += `<li>
            <span class="ref-number">[${details.index}]</span>
            <a href="${sanitizedUrl}" target="_blank" rel="noopener noreferrer">
                ${sanitizedTitle || sanitizedUrl}
            </a>
            <button class="btn btn-sm btn-light copy-link" data-url="${sanitizedUrl}"></button>
            </li>`;
    });
    refsHtml += '</ul>';

    return refsHtml;
}

/**
 * parse references to markdown links with index.
 * References are sorted by their index.
 * Each list item shows the index and the URL/text.
 *
 * @param {object} refObject - An object where keys are reference URLs/text and values are their index order.
 * @returns {string} HTML unordered list with sorted references.
 */
function combineRefsWithIndex (refObject) {
    const entries = [];
    // Convert object to array of {url, index} entries
    for (const url in refObject) {
        if (Object.prototype.hasOwnProperty.call(refObject, url)) {
            entries.push({ url, index: refObject[url] });
        }
    }
    // sort entries by their index value in ascending order
    entries.sort((a, b) => a.index - b.index);

    let markdown = '';
    for (const entry of entries) {
        // decode and sanitize the url/text
        const sanitizedVal = libs.sanitizeHTML(decodeURIComponent(entry.url));
        // include the index in the output (e.g., "1: ..." )
        if (entry.url.startsWith('http')) {
            markdown += `<li>[${entry.index}] <a href="${sanitizedVal}" target="_blank">${sanitizedVal}</a></li>`;
        } else {
            markdown += `<li>[${entry.index}] <p>${sanitizedVal}</p></li>`;
        }
    }

    return `<ul style="margin-bottom: 0;">${markdown}</ul>`;
}

// parse langchain qa references to markdown links
// function wrapRefLines (input) {
//     const lines = input.split('\n')
//     let result = ''
//     for (let i = 0; i < lines.length; i++) {
//         // skip empty lines
//         if (lines[i].trim() === '') {
//             continue
//         }

//         result += `* <${lines[i]}>\n`
//     }
//     return result
// }

async function abortAIResp (err) {
    if (typeof err === 'string') {
        err = new Error(err);
    }

    console.error(`abort AI resp: ${renderError(err)}`);
    if (globalAIRespSSE) {
        globalAIRespSSE.close();
        globalAIRespSSE = null;
        unlockChatInput();
    }

    if (!globalAIRespEle) {
        console.warn('globalAIRespEle is not set for abortAIResp');
        return;
    }

    // const chatID = globalAIRespEle.closest('.role-ai').dataset.chatid;
    let errMsg;
    if (err.data) {
        errMsg = err.data;
    } else {
        errMsg = err.toString();
    }

    if (errMsg === '[object CustomEvent]') {
        if (navigator.userAgent.includes('Firefox')) {
            // firefox will throw this error when window.SSE is closed, just ignore it.
            return;
        }

        errMsg = 'There may be a network issue, please check the network connection and try again later.';
    }

    if (typeof errMsg !== 'string') {
        errMsg = JSON.stringify(errMsg);
    }

    // if errMsg contains
    if (errMsg.includes('Access denied due to invalid subscription key or wrong API endpoint')) {
        showalert('danger', 'API TOKEN invalid, please ask admin to get new token.\nAPI TOKEN 无效，请联系管理员获取新的 API TOKEN。');
    }

    if (globalAIRespEle.dataset.taskStatus === 'waiting') {
        globalAIRespData.rawContent = `<p>🔥Someting in trouble...</p><pre style="text-wrap: pretty;">${libs.RenderStr2HTML(errMsg)}</pre>`;
    } else {
        globalAIRespData.rawContent += `<p>🔥Someting in trouble...</p><pre style="text-wrap: pretty;">${libs.RenderStr2HTML(errMsg)}</pre>`;
    }

    await renderAfterAiResp(globalAIRespData, true);
    // scrollToChat(globalAIRespEle);
    // await appendChats2Storage(RoleAI, chatID, globalAIRespEle.innerHTML);
}

/**
 * click to select images
 */
async function bindUserInputSelectFilesBtn () {
    await libs.waitElementReady('#chatContainer .user-input .btn.upload');
    chatContainer.querySelector('.user-input .btn.upload')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            // const evtTarget = libs.evtTarget(evt);

            const inputEle = document.createElement('input');
            inputEle.type = 'file';
            inputEle.multiple = true;
            inputEle.accept = 'image/*';

            inputEle.addEventListener('change', async (evt) => {
                const evtTarget = libs.evtTarget(evt);
                const files = evtTarget.files;
                for (const file of files) {
                    readFileForVision(file);
                }
            });

            inputEle.click();
        });
}

/**
 * auto display or hide user input select files button according to selected model
 */
async function autoToggleUserImageUploadBtn () {
    if (!chatPromptInputEle) {
        return;
    }

    const sconfig = await getChatSessionConfig();
    const isVision = VisionModels.includes(sconfig.selected_model);

    const btnEle = chatContainer.querySelector('.user-input .btn.upload');
    if ((isVision && btnEle) || (!isVision && !btnEle)) {
        // everything is ok
        return;
    }

    // const userPrompt = chatContainer.querySelector('.user-input .prompt').value;

    const uploadEleHtml = '<button class="btn btn-outline-secondary upload" type="button"><i class="bi bi-images"></i></button>';
    if (isVision && chatPromptInputBtn) {
        chatPromptInputBtn.insertAdjacentHTML('beforebegin', uploadEleHtml);
        bindUserInputSelectFilesBtn();
    } else {
        btnEle.remove();
    }
}

/**
 * bind text chat input. will be skipped if talking enabled
 */
async function setupChatInput () {
    if (!chatPromptInputEle) {
        return
    }

    // bind input press enter
    {
        let isComposition = false
        chatPromptInputEle
            .addEventListener('compositionstart', async (evt) => {
                evt.stopPropagation();
                isComposition = true;
            });
        chatPromptInputEle
            .addEventListener('compositionend', async (evt) => {
                evt.stopPropagation();
                isComposition = false;
            });

        chatPromptInputEle
            .addEventListener('keydown', async (evt) => {
                evt.stopPropagation();
                if (evt.key !== 'Enter' ||
                    isComposition ||
                    (evt.key === 'Enter' && !(evt.ctrlKey || evt.metaKey || evt.altKey)) ||
                    !isAllowChatPrompInput()) {
                    return;
                }

                await sendChat2Server();
                chatPromptInputEle.value = '';
            });
    }

    // change hint when models change
    {
        await libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
            if (op !== libs.KvOp.SET) {
                return;
            }

            const expectedKey = `KvKeyPrefixSessionConfig${(await activeSessionID())}`;
            if (key !== expectedKey) {
                return;
            }

            if (newVal.chat_switch.enable_talk) {
                return;
            }

            const sconfig = newVal;
            chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;
        }, 'setupChatInput_change_hint');
    }

    // bind input button
    chatPromptInputBtn
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            await sendChat2Server();
            chatPromptInputEle.value = '';
        });

    // bindImageUploadButton
    await autoToggleUserImageUploadBtn();
    await libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
        if (op !== libs.KvOp.SET) {
            return;
        }

        if (newVal.chat_switch.enable_talk) {
            return;
        }

        await autoToggleUserImageUploadBtn();
    }, 'setupChatInput_change_image_upload_btn');

    // restore pinned materials
    await restorePinnedMaterials();

    // bind input element's drag-drop
    {
        const dropfileModalEle = document.querySelector('#modal-dropfile.modal');
        const dropfileModal = new window.bootstrap.Modal(dropfileModalEle);

        const fileDragLeave = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();
        };

        const fileDragDropHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();

            if (!evt.dataTransfer || !evt.dataTransfer.items) {
                return;
            }

            for (let i = 0; i < evt.dataTransfer.items.length; i++) {
                const item = evt.dataTransfer.items[i];
                if (item.kind !== 'file') {
                    continue;
                }

                const file = item.getAsFile();
                if (!file) {
                    continue;
                }

                const fileExtension = `.${file.name.split('.').pop().toLowerCase()}`;
                switch (fileExtension) {
                case '.txt':
                case '.md':
                case '.doc':
                case '.docx':
                case '.pdf':
                case '.ppt':
                case '.pptx':
                    try {
                        ShowSpinner();
                        await uploadFileAsInputUrls(file, fileExtension);
                    } catch (err) {
                        console.error(`upload file failed: ${renderError(err)}`);
                    } finally {
                        HideSpinner();
                    }

                    break;
                default:
                    readFileForVision(file);
                }
            }
        };

        const fileDragOverHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            evt.dataTransfer.dropEffect = 'copy'; // Explicitly show this is a copy.
            dropfileModal.show();
        };

        chatPromptInputEle.addEventListener('paste', filePasteHandler);

        document.body.addEventListener('dragover', fileDragOverHandler);
        document.body.addEventListener('drop', fileDragDropHandler);
        // document.body.addEventListener('paste', filePasteHandler);

        dropfileModalEle.addEventListener('drop', fileDragDropHandler);
        dropfileModalEle.addEventListener('dragleave', fileDragLeave);
    }
}

/**
 * bind chat switchs
 */
async function setupChatSwitchs () {
    {
        chatContainer
            .querySelector('#switchChatEnableHttpsCrawler')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                const switchEle = libs.evtTarget(evt);
                const sconfig = await getChatSessionConfig();
                sconfig.chat_switch.disable_https_crawler = !switchEle.checked;

                // clear pinned https urls
                if (!switchEle.checked) {
                    await libs.KvSet(KvKeyPinnedMaterials, '');
                    await restorePinnedMaterials();
                }

                await saveChatSessionConfig(sconfig);
            });

        chatContainer
            .querySelector('#switchChatEnableGoogleSearch')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                const switchEle = libs.evtTarget(evt);
                const sconfig = await getChatSessionConfig();
                sconfig.chat_switch.enable_google_search = switchEle.checked;
                await saveChatSessionConfig(sconfig);
            });

        chatContainer
            .querySelector('#selectDrawNImage')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = libs.evtTarget(evt);
                const sconfig = await getChatSessionConfig();
                sconfig.chat_switch.draw_n_images = parseInt(switchEle.value);
                await saveChatSessionConfig(sconfig);
            });

        // chatContainer
        //     .querySelector('#switchChatEnableAutoSync')
        //     .addEventListener('change', async (evt) => {
        //         evt.stopPropagation();
        //         const switchEle = libs.evtTarget(evt);
        //         await libs.KvSet(KvKeyAutoSyncUserConfig, switchEle.checked);
        //     });

        // chatContainer
        //     .querySelector('#switchChatEnableAutoSync')
        //     .addEventListener('change', async (evt) => {
        //         evt.stopPropagation();
        //         const switchEle = libs.evtTarget(evt);
        //         await libs.KvSet(KvKeyAutoSyncUserConfig, switchEle.checked);
        //     });
        // let userConfigSyncer;
        // await libs.KvAddListener(KvKeyAutoSyncUserConfig, async (key, op, oldVal, newVal) => {
        //     if (op !== libs.KvOp.SET) {
        //         return;
        //     }

        //     // update ui
        //     const switchEle = chatContainer.querySelector('#switchChatEnableAutoSync');
        //     switchEle.checked = newVal;

        //     // update background syncer
        //     if (!newVal) {
        //         console.debug('stop user config syncer');
        //         if (userConfigSyncer) {
        //             clearTimeout(userConfigSyncer);
        //             userConfigSyncer = null;
        //         }

        //         return;
        //     }

        //     if (userConfigSyncer) {
        //         return;
        //     }

        //     console.debug('start user config syncer');
        //     // await syncUserConfig();
        //     userConfigSyncer = setTimeout(async () => {
        //         await syncUserConfig();
        //     }, 1800 * 1000);
        // });

        chatContainer
            .querySelector('#switchChatEnableTalking')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                const switchEle = libs.evtTarget(evt);
                const sconfig = await getChatSessionConfig();
                sconfig.chat_switch.enable_talk = switchEle.checked;
                await saveChatSessionConfig(sconfig);
            });

        // bind listener for all chat switchs
        await libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
            if (op !== libs.KvOp.SET) {
                return;
            }

            const expectedKey = `${KvKeyPrefixSessionConfig}${(await activeSessionID())}`;
            if (key !== expectedKey) {
                return;
            }

            const sconfig = newVal;
            chatContainer.querySelector('#switchChatEnableHttpsCrawler').checked = !sconfig.chat_switch.disable_https_crawler;
            chatContainer.querySelector('#switchChatEnableGoogleSearch').checked = sconfig.chat_switch.enable_google_search;
            chatContainer.querySelector('#switchChatEnableAllInOne').checked = sconfig.chat_switch.all_in_one;
            await bindTalkSwitchHandler(sconfig.chat_switch.enable_talk);

            // enable talk
        }, 'setupChatInput_change_chat_switchs');
    }
}

async function bindTalkSwitchHandler (newSelectedValue) {
    // update ui
    const switchEle = chatContainer.querySelector('#switchChatEnableTalking');
    switchEle.checked = newSelectedValue;

    // update background syncer
    if (newSelectedValue) {
        chatPromptInputEle = null;
        chatPromptInputBtn = null;

        chatContainer.querySelector('.user-input').innerHTML =
            '<button class="btn btn-outline-secondary" type="button" data-fn="record"><i class="bi bi-mic"></i></button>';
        await bindTalkBtnHandler();
        if (!audioStream) {
            audioStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        }
        return;
    }

    // close stream
    if (audioStream) {
        audioStream.getTracks().forEach(track => track.stop());
        audioStream = null;
    }

    // update chat input element.
    // if prompt input already exists, do nothing.
    if (!chatContainer.querySelector('.user-input .input.prompt')) {
        const ssconfig = await getChatSessionConfig();
        chatContainer.querySelector('.user-input').innerHTML = `
        <div class="input-group mb-3 user-input">
            <textarea dir="auto" class="form-control input prompt" placeholder="[${ssconfig.selected_model}] CTRL+Enter to send"></textarea>
            <button class="btn btn-outline-secondary send" type="button"><i class="bi bi-send"></i></button>
        </div>`;

        // reset chat input element
        await libs.waitElementReady('#chatContainer .user-input .input.prompt');
        chatPromptInputEle = chatContainer.querySelector('.user-input .input.prompt');
        chatPromptInputBtn = chatContainer.querySelector('.user-input .btn.send');

        await setupChatInput();
    }
}

async function bindTalkBtnHandler () {
    let mediaRecorder;
    let audioChunks = [];
    let startRecordingAt = Date.now();

    const startRecording = async (evt) => {
        console.debug('start recording');
        evt.stopPropagation();
        evt.preventDefault();
        const evtTarget = libs.evtTarget(evt);

        evtTarget.classList.add('active');
        startRecordingAt = Date.now();

        if (mediaRecorder && mediaRecorder.state === 'recording') {
            mediaRecorder.stop();
        }

        mediaRecorder = new MediaRecorder(audioStream);
        audioChunks = [];
        mediaRecorder.ondataavailable = event => {
            audioChunks.push(event.data);
        };

        mediaRecorder.start();

        mediaRecorder.onstop = async (evt) => {
            evt.stopPropagation();

            if (Date.now() - startRecordingAt < 500) {
                console.debug('discard recording because of too short');
                return;
            }

            if (audioChunks.length === 0) {
                console.debug('no audio data recorded');
                return;
            }

            try {
                ShowSpinner();

                const audioBlob = new Blob(audioChunks, { type: 'audio/wav' });
                const formData = new FormData();
                formData.append('file', audioBlob, 'user_audio.wav');
                formData.set('model', 'whisper-large-v3');
                formData.set('response_format', 'verbose_json');

                const ssonfig = await getChatSessionConfig();

                // transcript voice to txt
                console.log(`send voice to server, length=${audioBlob.size}`);
                const resp = await fetch('/oneapi/v1/audio/transcriptions', {
                    method: 'POST',
                    headers: {
                        Authorization: `Bearer ${ssonfig.api_token}`
                    },
                    body: formData
                });
                if (!resp.ok || resp.status !== 200) {
                    throw new Error(`${resp.status} ${await resp.text()}`);
                }

                const userPrompt = (await resp.json()).text;
                if (!userPrompt) {
                    console.debug('transcript server has not recognized any text');
                    HideSpinner();
                    return;
                }

                const chatID = await sendChat2Server(null, userPrompt);
                const storageActiveSessionKey = kvSessionKey(await activeSessionID());
                await libs.KvAddListener(storageActiveSessionKey, async (key, op, oldVal, newVal) => {
                    if (op !== libs.KvOp.SET) {
                        return;
                    }

                    if (key !== storageActiveSessionKey) {
                        return;
                    }

                    const startAt = Date.now();
                    while (Date.now() - startAt < 3000) {
                        for (const chatHis of newVal) {
                            if (chatHis.chatID === chatID && chatHis.role === RoleAI) {
                                const voiceBtn = chatContainer.querySelector(`#${chatID} .btn[data-fn="voice"]`);
                                if (!voiceBtn) {
                                    break;
                                }

                                const text = chatHis.rawContent;
                                await tts(chatID, text);

                                libs.KvRemoveListener(storageActiveSessionKey, 'bindTalkBtnHandler_tts');
                                return;
                            }
                        }

                        await libs.Sleep(100);
                    }
                }, 'bindTalkBtnHandler_tts');
            } catch (err) {
                showalert('danger', `record voice failed: ${renderError(err)}`);
                HideSpinner();
            } finally {
                audioChunks = [];
            }
        }
    }

    const stopRecording = async (evt) => {
        console.debug('stop recording');
        evt.stopPropagation();
        evt.preventDefault();
        const evtTarget = libs.evtTarget(evt);

        evtTarget.classList.remove('active');

        if (!mediaRecorder || mediaRecorder.state !== 'recording') {
            console.debug('recording is not started');
            return;
        }

        mediaRecorder.requestData()
        mediaRecorder.stop();
        mediaRecorder = null;
    }

    const recordButton = chatContainer
        .querySelector('.user-input .btn[data-fn="record"]');
    if (libs.IsTouchDevice()) {
        recordButton.addEventListener('touchstart', startRecording);
        recordButton.addEventListener('touchend', stopRecording);
    } else {
        recordButton.addEventListener('mousedown', startRecording);
        recordButton.addEventListener('mouseup', stopRecording);
    }
}

/**
 * bind paste event for chat input
 */
async function filePasteHandler (evt) {
    if (!evt.clipboardData || !evt.clipboardData.items) {
        return;
    }

    // do not skip default paste action
    // evt.stopPropagation();
    // evt.preventDefault();

    // There may be various types of rich text formats,
    // but here we only handle the images required for vision.
    let file;
    for (let i = 0; i < evt.clipboardData.items.length; i++) {
        const item = evt.clipboardData.items[i];

        if (item.kind === 'file') {
            file = item.getAsFile();
            if (!file) {
                continue;
            }

            // get file content as Blob
            readFileForVision(file, `paste-${libs.DateStr()}.png`);
        }

        // if (item.type === 'text/rtf' || item.type === 'text/html') {
        //     // remove rtf content that copy from word/ppt
        //     continue;
        // }

        // switch (item.kind) {
        // case 'string':
        //     // should paste to the position of cursor
        //     item.getAsString((val) => {
        //         if (document.activeElement === chatPromptInputEle) {
        //             const startPos = chatPromptInputEle.selectionStart;
        //             const endPos = chatPromptInputEle.selectionEnd;
        //             chatPromptInputEle.value = chatPromptInputEle.value.substring(0, startPos) +
        //                     val +
        //                     chatPromptInputEle.value.substring(endPos, chatPromptInputEle.value.length);
        //             chatPromptInputEle.selectionStart = startPos + val.length;
        //             chatPromptInputEle.selectionEnd = startPos + val.length;
        //         } else {
        //             chatPromptInputEle.value += val;
        //         }
        //     });
        //     break;
        // case 'file':
        //     file = item.getAsFile();
        //     if (!file) {
        //         continue;
        //     }

        //     // get file content as Blob
        //     readFileForVision(file, `paste-${libs.DateStr()}.png`);
        //     break;
        // default:
        //     continue;
        // }
    }
};

/**
 * upload user file then insert file url to the head of prompt input
 *
 * @param {file} file - file object
 */
async function uploadFileAsInputUrls (file, fileExt) {
    if (file.size > 1024 * 1024 * 20) {
        showalert('danger', 'File size exceeds the limit of 20MB');
        return;
    }

    const formData = new FormData();
    formData.append('file', file);
    formData.append('file_ext', fileExt);

    const sconfig = await getChatSessionConfig();
    const resp = await fetch('/files/chat', {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await libs.getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: formData,
        redirect: 'follow'
    });

    if (!resp.ok) {
        showalert('danger', `upload file failed: ${await resp.text()}`);
        return;
    }

    const data = await resp.json();
    if (!data || !data.url) {
        showalert('danger', 'upload file failed');
        return;
    }

    if (sconfig.chat_switch.disable_https_crawler) {
        sconfig.chat_switch.disable_https_crawler = false;
        await saveChatSessionConfig(sconfig);
    }

    const url = data.url;
    chatPromptInputEle.value = url + '\n' + chatPromptInputEle.value;
}

/** read file content and append to vision store
 *
 * @param {*} file - file object
 * @param {*} rewriteName - rewrite file name
 */
function readFileForVision (file, rewriteName) {
    const filename = rewriteName || file.name;

    // get file content as Blob
    const reader = new FileReader();
    reader.onload = async (e) => {
        const arrayBuffer = e.target.result;
        if (arrayBuffer.byteLength > 1024 * 1024 * 10) {
            showalert('danger', 'file size should less than 10M');
            return;
        }

        const byteArray = new Uint8Array(arrayBuffer);
        const chunkSize = 0xffff; // Use chunks to avoid call stack limit
        const chunks = [];
        for (let i = 0; i < byteArray.length; i += chunkSize) {
            chunks.push(String.fromCharCode.apply(null, byteArray.subarray(i, i + chunkSize)));
        }
        const base64String = btoa(chunks.join(''));

        // only support 5 image for current version
        if (chatVisionSelectedFileStore.length > 5) {
            showalert('warning', 'only support 5 images for current version');
            return;
        }

        // check duplicate
        for (const item of chatVisionSelectedFileStore) {
            if (item.contentB64 === base64String) {
                return;
            }
        }

        chatVisionSelectedFileStore.push({
            filename,
            contentB64: base64String
        });
        updateChatVisionSelectedFileStore();
    };

    reader.readAsArrayBuffer(file);
}

async function updateChatVisionSelectedFileStore () {
    const pinnedFiles = chatContainer.querySelector('.pinned-files');
    pinnedFiles.innerHTML = '';
    for (const item of chatVisionSelectedFileStore) {
        pinnedFiles.insertAdjacentHTML('beforeend', `<p data-key="${item.filename}"><i class="bi bi-trash"></i> ${item.filename}</p>`);
    }

    // click to remove pinned file
    chatContainer.querySelectorAll('.pinned-files .bi.bi-trash')
        .forEach((item) => {
            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const ele = libs.evtTarget(evt).closest('p');
                const key = ele.dataset.key;
                chatVisionSelectedFileStore = chatVisionSelectedFileStore.filter((item) => item.filename !== key);
                ele.parentNode.removeChild(ele);
            });
        });
}

/**
 * reload ai response
 *
 * @param {async function} overwriteSendChat2Server - overwrite sendChat2Server function
 */
async function reloadAiResp (chatID, overwriteSendChat2Server) {
    const chatEle = chatContainer.querySelector(`.chatManager .conservations .chats #${chatID}`);

    delete muRenderAfterAiRespForChatID[chatID];

    let newText = ''
    if (chatEle.querySelector('.role-human textarea')) {
        // read user input from click edit button
        newText = chatEle.querySelector('.role-human textarea').value;
    } else {
        // read user input from click reload button
        newText = chatEle.querySelector('.role-human .text-start pre').innerHTML;
    }

    const selecedModel = await OpenaiSelectedModel();

    // update chat data
    const chatData = await getChatData(chatID, RoleAI) || {};
    chatData.model = selecedModel;
    chatData.taskStatus = ChatTaskStatusWaiting;
    chatData.taskData = [];
    chatData.succeedImageUrls = [];
    await setChatData(chatID, RoleAI, chatData);

    chatEle.innerHTML = `
        <div class="container-fluid row role-human" data-chatid="${chatID}">
            <div class="col-auto icon">🤔️</div>
            <div class="col text-start"><pre>${newText}</pre></div>
            <div class="col-auto d-flex control">
                <i class="bi bi-pencil-square"></i>
                <i class="bi bi-trash"></i>
            </div>
        </div>
        <div class="container-fluid row role-ai" data-chatid="${chatID}">
            <div class="col-auto icon">${robotIcon}</div>
            <div class="col text-start ai-response"
                data-task-status="${ChatTaskStatusWaiting}"
                data-task-type="${chatData.taskType}"
                data-model="${selecedModel}">
                <p class="card-text placeholder-glow">
                    <span class="placeholder col-7"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-6"></span>
                    <span class="placeholder col-8"></span>
                </p>
            </div>
        </div>`;

    // chatEle.dataset.taskStatus = ChatTaskStatusWaiting;

    // bind delete and edit button
    chatEle.querySelector('.role-human .bi-trash')
        .addEventListener('click', deleteBtnHandler);
    chatEle.querySelector('.bi.bi-pencil-square')
        .addEventListener('click', bindEditHumanInput);

    if (overwriteSendChat2Server) {
        await overwriteSendChat2Server();
    } else {
        await sendChat2Server(chatID);
    }
};

/**
 * put attachments back to vision store when edit human input
 *
 * @param {string} chatID - chat id
 *
 * @returns {string} - attachHTML
 */
function putBackAttachmentsInUserInput (chatID) {
    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);

    // attach image to vision-selected-store when edit human input
    const attachEles = chatEle
        .querySelectorAll('.role-human .text-start img') || [];
    let attachHTML = '';
    attachEles.forEach((ele) => {
        const b64fileContent = ele.getAttribute('src').replace('data:image/png;base64,', '');
        const key = ele.dataset.name || `${libs.DateStr()}.png`;
        chatVisionSelectedFileStore.push({
            filename: key,
            contentB64: b64fileContent
        });

        attachHTML += `<img src="data:image/png;base64,${b64fileContent}" data-name="${key}">`;
    })
    updateChatVisionSelectedFileStore();

    return attachHTML;
}

/**
 * edit human input
 *
 * @param {Event} evt - event
 */
function bindEditHumanInput (evt) {
    evt.stopPropagation();
    const evtTarget = libs.evtTarget(evt);

    const chatID = evtTarget.closest('.role-human').dataset.chatid;
    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);
    const oldText = chatEle.innerHTML;
    let text = chatEle.querySelector('.role-human .text-start pre').innerHTML;

    const attachHTML = putBackAttachmentsInUserInput(chatID);

    text = libs.sanitizeHTML(text);
    chatContainer.querySelector(`#${chatID} .role-human`).innerHTML = `
        <textarea dir="auto" class="form-control" rows="3">${text}</textarea>
        <div class="btn-group" role="group">
            <button class="btn btn-sm btn-outline-secondary save" type="button">
                <i class="bi bi-check"></i>
                Save</button>
            <button class="btn btn-sm btn-outline-secondary cancel" type="button">
                <i class="bi bi-x"></i>
                Cancel</button>
        </div>`;

    chatEle.querySelector('.role-human .btn.save')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            // update storage with user's new prompt
            const newtext = chatEle.querySelector('.role-human textarea').value;
            await saveChats2Storage({
                chatID,
                attachHTML,
                role: RoleHuman,
                content: newtext
                // model: await OpenaiSelectedModel()
            });

            await reloadAiResp(chatID);
        });

    chatEle.querySelector('.role-human .btn.cancel')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            chatEle.innerHTML = oldText;

            // bind delete and edit button
            chatEle.querySelector('.role-human .bi-trash')
                .addEventListener('click', deleteBtnHandler);
            chatEle.querySelector('.bi.bi-pencil-square')
                .addEventListener('click', bindEditHumanInput);
        });
};

// bind delete button
const deleteBtnHandler = (evt) => {
    evt.stopPropagation();
    const evtTarget = libs.evtTarget(evt);

    const chatID = evtTarget.closest('.role-human').dataset.chatid;
    const chatEle = chatContainer.querySelector(`.chatManager .conservations .chats #${chatID}`);

    ConfirmModal('Are you sure to delete this chat?', async () => {
        chatEle.parentNode.removeChild(chatEle);
        removeChatInStorage(chatID);
    });
};

/**
 * Append chat to conservation container
 *
 * @param {boolean} isHistory - is history chat, default false. if true, will not append to storage
 * @param {Object} chatData - chat item
 *   @property {string} chatID - chat id
 *   @property {string} role - RoleHuman/RoleSystem/RoleAI
 *   @property {string} content - chat content in HTML
 *   @property {string} attachHTML - html to attach to chat
 *   @property {string} rawContent - raw ai response
 *   @property {string} reasoningContent - raw ai reasoning response
 *   @property {string} costUsd - cost in usd
 *   @property {string} model - model name
 *   @property {string} requestid - request id
 */
async function append2Chats (isHistory, chatData) {
    const chatID = chatData.chatID;
    const role = chatData.role;
    let content = chatData.content;
    let attachHTML = chatData.attachHTML || '';

    if (!chatID) {
        throw new Error('chatID is required');
    }

    let chatEleHtml;
    let chatOp = 'append';
    let waitAI = '';
    attachHTML = attachHTML || '';
    switch (role) {
    case RoleSystem:
        content = libs.escapeHtml(content);
        chatEleHtml = `
            <div class="container-fluid row role-human">
                <div class="col-auto icon">💻</div>
                <div class="col text-start"><pre>${content}</pre></div>
            </div>`;
        break;
    case RoleHuman:
        content = libs.escapeHtml(content);
        if (!isHistory) {
            waitAI = `
                <div class="container-fluid row role-ai" data-chatid="${chatID}">
                    <div class="col-auto icon">${robotIcon}</div>
                    <div class="col text-start ai-response"
                        data-task-type="${ChatTaskTypeChat}"
                        data-task-status="${ChatTaskStatusWaiting}">
                        <p dir="auto" class="card-text placeholder-glow">
                            <span class="placeholder col-7"></span>
                            <span class="placeholder col-4"></span>
                            <span class="placeholder col-4"></span>
                            <span class="placeholder col-6"></span>
                            <span class="placeholder col-8"></span>
                        </p>
                    </div>
                </div>`;
        }

        chatEleHtml = `
                <div id="${chatID}">
                    <div class="container-fluid row role-human" data-chatid="${chatID}">
                        <div class="col-auto icon">🤔️</div>
                        <div class="col text-start">
                            <pre>${content}</pre>
                            ${attachHTML}
                        </div>
                        <div class="col-auto d-flex control">
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-trash"></i>
                        </div>
                    </div>
                    ${waitAI}
                </div>`;
        break;
    case RoleAI:
        if (!content) {
            content = `
                <p dir="auto" class="card-text placeholder-glow">
                    <span class="placeholder col-7"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-6"></span>
                    <span class="placeholder col-8"></span>
                </p>`;
        }

        chatEleHtml = `
            <div class="container-fluid row role-ai" data-chatid="${chatID}">
                    <div class="col-auto icon">${robotIcon}</div>
                    <div class="col text-start ai-response"
                        data-task-type="${chatData.taskType || ChatTaskTypeChat}"
                        data-task-status="${chatData.taskStatus || ChatTaskStatusWaiting}">
                        ${content}
                    </div>
            </div>`;
        if (!isHistory) {
            chatOp = 'replace';
        }

        break;
    }

    if (chatOp === 'append') {
        if (role === RoleAI) {
            // ai response is always after human, so we need to find the last human chat,
            // and append ai response after it
            if (chatContainer.querySelector(`#${chatID}`)) {
                chatContainer.querySelector(`#${chatID}`)
                    .insertAdjacentHTML('beforeend', chatEleHtml);
            }
        } else {
            chatContainer.querySelector('.chatManager .conservations .chats')
                .insertAdjacentHTML('beforeend', chatEleHtml);
        }
    }

    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);
    if (chatOp === 'replace') {
        // replace html element of ai
        chatEle.querySelector('.role-ai')
            .outerHTML = chatEleHtml;
    }

    if (!isHistory && role === RoleHuman) {
        scrollToChat(chatEle);
    }

    // avoid duplicate event listener, only bind event listener for new chat
    if (role === RoleHuman) {
        // bind delete and edit button
        chatEle.querySelector('.role-human .bi-trash')
            .addEventListener('click', deleteBtnHandler);
        chatEle.querySelector('.bi.bi-pencil-square')
            .addEventListener('click', bindEditHumanInput);
    }
}

/**
 * get user's chat session config by sid
 *
 * @param {string} sid - session id, default is active session id
 */
async function getChatSessionConfig (sid) {
    if (!sid) {
        sid = await activeSessionID();
    }

    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    let sconfig = await libs.KvGet(skey);

    if (!sconfig) {
        console.info(`create new session config for session ${sid}`);
        sconfig = newSessionConfig();
        await saveChatSessionConfig(sconfig, sid);
    }

    return sconfig;
};

/**
 * save chat session config
 *
 * @param {Object} sconfig - session config
 * @param {string} sid - session id
 */
async function saveChatSessionConfig (sconfig, sid) {
    if (!sid) {
        sid = await activeSessionID();
    }

    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    await libs.KvSet(skey, sconfig);
};

/**
 * create new default session config
 *
 * @returns {Object} - new session config
 */
function newSessionConfig () {
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
            enable_google_search: false
        }
    }
}

/**
 * initialize every chat component by active session config
 */
async function updateConfigFromSessionConfig () {
    console.debug(`updateConfigFromSessionConfig for session ${(await activeSessionID())}`);

    const sconfig = await getChatSessionConfig();
    sconfig.selected_model = await OpenaiSelectedModel();

    // update config
    configContainer.querySelector('.input.api-token').value = sconfig.api_token;
    configContainer.querySelector('.input.api-base').value = sconfig.api_base;
    configContainer.querySelector('.input.contexts').value = sconfig.n_contexts;
    configContainer.querySelector('.input-group.contexts .contexts-val').innerHTML = sconfig.n_contexts;
    configContainer.querySelector('.input.max-token').value = sconfig.max_tokens;
    configContainer.querySelector('.input-group.max-token .max-token-val').innerHTML = sconfig.max_tokens;
    configContainer.querySelector('.input.temperature').value = sconfig.temperature;
    configContainer.querySelector('.input-group.temperature .temperature-val').innerHTML = sconfig.temperature;
    configContainer.querySelector('.input.presence_penalty').value = sconfig.presence_penalty;
    configContainer.querySelector('.input-group.presence_penalty .presence_penalty-val').innerHTML = sconfig.presence_penalty;
    configContainer.querySelector('.input.frequency_penalty').value = sconfig.frequency_penalty;
    configContainer.querySelector('.input-group.frequency_penalty .frequency_penalty-val').innerHTML = sconfig.frequency_penalty;
    configContainer.querySelector('.system-prompt .input').value = sconfig.system_prompt;

    // update chat input hint
    if (chatPromptInputEle) {
        chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;
    }

    // update chat controller
    chatContainer.querySelector('#switchChatEnableHttpsCrawler').checked = !sconfig.chat_switch.disable_https_crawler;
    chatContainer.querySelector('#switchChatEnableGoogleSearch').checked = sconfig.chat_switch.enable_google_search;
    chatContainer.querySelector('#switchChatEnableAllInOne').checked = sconfig.chat_switch.all_in_one;
    // chatContainer.querySelector('#switchChatEnableAutoSync').checked = await libs.KvGet(KvKeyAutoSyncUserConfig);
    chatContainer.querySelector('#selectDrawNImage').value = sconfig.chat_switch.draw_n_images;
    await bindTalkSwitchHandler(sconfig.chat_switch.enable_talk);

    // update selected model
    // set active status for models
    const selectedModel = sconfig.selected_model;
    document.querySelectorAll('#headerbar .navbar-nav a.dropdown-toggle')
        .forEach((elem) => {
            elem.classList.remove('active');
        });
    document
        .querySelectorAll('#headerbar .chat-models li a, ' +
            '#headerbar .qa-models li a, ' +
            '#headerbar .image-models li a'
        )
        .forEach((elem) => {
            elem.classList.remove('active');

            if (elem.dataset.model === selectedModel) {
                elem.classList.add('active');
                elem.closest('.dropdown').querySelector('a.dropdown-toggle').classList.add('active');
            }
        });
}

async function setupConfig () {
    await updateConfigFromSessionConfig();

    //  config_api_token_value
    {
        const apitokenInput = configContainer
            .querySelector('.input.api-token');
        apitokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.api_token = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);
        });
    }

    // bind api_base
    {
        const apibaseInput = configContainer
            .querySelector('.input.api-base');
        apibaseInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.api_base = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);
        });
    }

    //  config_chat_n_contexts
    {
        const maxtokenInput = configContainer
            .querySelector('.input.contexts');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.n_contexts = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.contexts .contexts-val').innerHTML = libs.evtTarget(evt).value;
        });
    }

    //  config_api_max_tokens
    {
        const maxtokenInput = configContainer
            .querySelector('.input.max-token');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.max_tokens = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.max-token .max-token-val').innerHTML = libs.evtTarget(evt).value;
        });
    }

    //  config_api_temperature
    {
        const maxtokenInput = configContainer
            .querySelector('.input.temperature');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`
            const sconfig = await libs.KvGet(skey);

            sconfig.temperature = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.temperature .temperature-val').innerHTML = libs.evtTarget(evt).value;
        });
    }

    //  config_api_presence_penalty
    {
        const maxtokenInput = configContainer
            .querySelector('.input.presence_penalty');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.presence_penalty = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.presence_penalty .presence_penalty-val').innerHTML = libs.evtTarget(evt).value;
        });
    }

    //  config_api_frequency_penalty
    {
        const maxtokenInput = configContainer
            .querySelector('.input.frequency_penalty');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.frequency_penalty = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.frequency_penalty .frequency_penalty-val').innerHTML = libs.evtTarget(evt).value;
        });
    }

    //  config_api_static_context
    {
        const staticConfigInput = configContainer
            .querySelector('.system-prompt .input');
        staticConfigInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await libs.KvGet(skey);

            sconfig.system_prompt = libs.evtTarget(evt).value;
            await libs.KvSet(skey, sconfig);
        });
    }

    // bind reset button
    {
        configContainer.querySelector('.btn.reset')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                ConfirmModal('Reset everything?', async () => {
                    localStorage.clear();
                    await libs.KvClear();
                    location.reload();
                });
            });
    }

    // bind clear-chats button
    {
        document.querySelectorAll('.btn.clear-chats')
            .forEach((ele) => {
                ele.addEventListener('click', async (evt) => {
                    evt.stopPropagation();

                    ConfirmModal('Clear current chat history but keep the session settings?', async () => {
                        clearSessionAndChats(evt, await activeSessionID());
                    });
                });
            });
    }

    // bind submit button
    {
        configContainer.querySelector('.btn.submit')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                location.reload();
            });
    }

    // bind sync key
    {
        const syncKeyEle = configContainer.querySelector('.input.sync-key');
        syncKeyEle
            .addEventListener('input', async (evt) => {
                evt.stopPropagation();
                const syncKey = libs.evtTarget(evt).value;
                await libs.KvSet(KvKeySyncKey, syncKey);
            });
        await libs.KvAddListener(KvKeySyncKey, async (key, op, oldVal, newVal) => {
            if (op !== libs.KvOp.SET) {
                return;
            }

            syncKeyEle.value = newVal;
        });

        // set default val
        if (!(await libs.KvGet(KvKeySyncKey))) {
            await libs.KvSet(KvKeySyncKey, `sync-${libs.RandomString(64)}`);
        }
        syncKeyEle.value = await libs.KvGet(KvKeySyncKey);
    }

    // bind upload & download configs
    {
        configContainer.querySelector('.btn[data-app-fn="upload-config"]')
            .addEventListener('click', async (evt) => {
                try {
                    ShowSpinner();
                    await uploadUserConfig(evt);
                    location.reload();
                } catch (err) {
                    console.error(err);
                    showalert('danger', `sync user config failed: ${renderError(err)}`);
                } finally {
                    HideSpinner();
                }
            });

        configContainer.querySelector('.btn[data-app-fn="download-config"]')
            .addEventListener('click', async (evt) => {
                try {
                    ShowSpinner();
                    await downloadUserConfig(evt);
                    location.reload();
                } catch (err) {
                    console.error(err);
                    showalert('danger', `sync user config failed: ${renderError(err)}`);
                } finally {
                    HideSpinner();
                }
            });

        // configContainer.querySelector('.btn-upload')
        //     .addEventListener('click', uploadUserConfig);

        // configContainer.querySelector('.btn-download')
        //     .addEventListener('click', downloadUserConfig);
    }

    libs.EnableTooltipsEverywhere();
}

// async function syncUserConfig (evt) {
//     await downloadUserConfig(evt);
//     await uploadUserConfig(evt);
// }

async function uploadUserConfig (evt) {
    console.debug('uploadUserConfig');
    evt && evt.stopPropagation();

    const data = {};
    await Promise.all((await libs.KvList()).map(async (key) => {
        data[key] = await libs.KvGet(key);
    }));

    const syncKey = await libs.KvGet(KvKeySyncKey);
    const resp = await fetch('/user/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-LAISKY-SYNC-KEY': syncKey
        },
        body: JSON.stringify(data)
    });

    if (resp.status !== 200) {
        throw new Error(`upload config failed: ${resp.status}`);
    }
}

async function downloadUserConfig (evt) {
    console.debug('downloadUserConfig');
    evt && evt.stopPropagation();

    const syncKey = await libs.KvGet(KvKeySyncKey);
    const resp = await fetch('/user/config', {
        method: 'GET',
        headers: {
            'X-LAISKY-SYNC-KEY': syncKey,
            'Cache-Control': 'no-cache'
        }
    });

    if (resp.status !== 200) {
        if (resp.status === 400) {
            return;
        }

        throw new Error(`download config failed: ${resp.status}`);
    }

    const data = await resp.json();
    for (const key in data) {
        // download non-exists key
        if (!(await libs.KvExists(key))) {
            await libs.KvSet(key, data[key]);
            continue;
        }

        // only update session config with different session name
        if (key.startsWith(KvKeyPrefixSessionConfig)) {
            const localConfig = await libs.KvGet(key);
            if (localConfig.session_name !== data[key].session_name) {
                await libs.KvSet(key, data[key]);
                continue;
            }
        }

        // incremental update local sessions chat history
        if (key.startsWith(KvKeyPrefixSessionHistory)) {
            let localHistory = await libs.KvGet(key);
            let iLocal = 0;
            let iRemote = 0;
            let localChatId = 0;
            let remoteChatId = 0;
            let localChatNum = 0;
            let remoteChatNum = 0;
            if (localHistory && Array.isArray(localHistory)) {
                while (iLocal < localHistory.length || iRemote < data[key].length) {
                    if (iLocal >= localHistory.length) {
                        if (iRemote < data[key].length) {
                            localHistory = localHistory.concat(data[key].slice(iRemote));
                        }
                        break;
                    }
                    if (iRemote >= data[key].length) {
                        break;
                    }

                    if (localHistory[iLocal]) {
                        localChatId = localHistory[iLocal].chatID;
                    } else {
                        iLocal++;
                        continue;
                    }
                    // latest version's chatid like: chat-1705899120122-NwN9sB
                    if (!localChatId || !localChatId.match(/chat-\d+-\w+/)) {
                        iLocal++;
                        continue;
                    }

                    localChatNum = parseInt(localChatId.split('-')[1]);

                    while (iRemote < data[key].length) {
                        if (data[key][iRemote]) {
                            remoteChatId = data[key][iRemote].chatID;
                        } else {
                            iRemote++;
                            continue;
                        }
                        if (!remoteChatId || !remoteChatId.match(/chat-\d+-\w+/)) {
                            localHistory.splice(iLocal - 1, 0, data[key][iRemote]);
                            iLocal++;
                            iRemote++;
                            continue;
                        }

                        remoteChatNum = parseInt(remoteChatId.split('-')[1]);
                        break;
                    }

                    if (iRemote >= data[key].length) {
                        break;
                    }

                    // skip same chat
                    if (localChatNum === remoteChatNum) {
                        while (iLocal < localHistory.length && localHistory[iLocal].chatID === localChatId) {
                            iLocal++;
                        }

                        while (iRemote < data[key].length && data[key][iRemote].chatID === remoteChatId) {
                            iRemote++;
                        }

                        continue;
                    }

                    // insert remote chat into local by chat num
                    if (localChatNum > remoteChatNum) {
                        // insert before
                        localHistory.splice(iLocal - 1, 0, data[key][iRemote]);
                        iRemote++;
                    }

                    iLocal++;
                }
            }

            await libs.KvSet(key, localHistory);
        }
    }
}

async function loadPromptShortcutsFromStorage () {
    let shortcuts = await libs.KvGet(KvKeyPromptShortCuts);
    if (!shortcuts) {
        // default prompts
        shortcuts = [
            {
                title: '中英互译',
                description: 'As an English-Chinese translator, your task is to accurately translate text between the two languages. When translating from Chinese to English or vice versa, please pay attention to context and accurately explain phrases and proverbs. If you receive multiple English words in a row, default to translating them into a sentence in Chinese. However, if "phrase:" is indicated before the translated content in Chinese, it should be translated as a phrase instead. Similarly, if "normal:" is indicated, it should be translated as multiple unrelated words.Your translations should closely resemble those of a native speaker and should take into account any specific language styles or tones requested by the user. Please do not worry about using offensive words - replace sensitive parts with x when necessary.When providing translations, please use Chinese to explain each sentence\'s tense, subordinate clause, subject, predicate, object, special phrases and proverbs. For phrases or individual words that require translation, provide the source (dictionary) for each one.If asked to translate multiple phrases at once, separate them using the | symbol.Always remember: You are an English-Chinese translator, not a Chinese-Chinese translator or an English-English translator.Please review and revise your answers carefully before submitting.'
            }
        ];
        await libs.KvSet(KvKeyPromptShortCuts, shortcuts);
    }

    return shortcuts;
}

async function EditFavSystemPromptHandler (evt) {
    evt.stopPropagation();
    const evtTarget = libs.evtTarget(evt);

    const badgetEle = evtTarget.closest('.badge');
    const saveSystemPromptModelEle = document.querySelector('#save-system-prompt.modal');
    const saveSystemPromptModal = new window.bootstrap.Modal(saveSystemPromptModelEle);

    const shortcut = {
        title: badgetEle.querySelector('.title').innerText,
        description: badgetEle.dataset.prompt
    }

    const titleInput = saveSystemPromptModelEle
        .querySelector('.modal-body input.title');
    titleInput.value = shortcut.title;

    const descriptionInput = saveSystemPromptModelEle
        .querySelector('.modal-body textarea.user-input');
    descriptionInput.value = shortcut.description;

    systemPromptModalCallback = async (evt) => {
        evt.stopPropagation();

        // trim and check empty
        titleInput.value = titleInput.value.trim();
        descriptionInput.value = descriptionInput.value.trim();
        if (titleInput.value === '') {
            titleInput.classList.add('border-danger');
            return;
        }
        if (descriptionInput.value === '') {
            descriptionInput.classList.add('border-danger');
            return;
        }

        const newShortcut = {
            title: titleInput.value,
            description: descriptionInput.value
        };

        // update in kv
        let shortcuts = await libs.KvGet(KvKeyPromptShortCuts);
        shortcuts = shortcuts.map((item) => {
            if (item.title === shortcut.title) {
                return newShortcut;
            }
            return item;
        });
        await libs.KvSet(KvKeyPromptShortCuts, shortcuts);

        // update badge in html
        badgetEle.dataset.prompt = newShortcut.description;
        badgetEle.querySelector('.title').innerText = newShortcut.title;

        saveSystemPromptModal.hide();
    };

    saveSystemPromptModal.show();
}

/**
 * append prompt shortcuts to html and kv
 *
 * @param {Object} shortcut - shortcut object
 * @param {bool} storage - whether to save to kv
 */
async function appendPromptShortcut (shortcut, storage = false) {
    const promptShortcutContainer = configContainer.querySelector('.prompt-shortcuts');

    // add to local storage
    if (storage) {
        const shortcuts = await loadPromptShortcutsFromStorage();
        shortcuts.push(shortcut);
        await libs.KvSet(KvKeyPromptShortCuts, shortcuts);
    }

    // new element
    const ele = document.createElement('span');
    ele.classList.add('badge', 'text-bg-info');
    ele.dataset.prompt = shortcut.description;
    ele.dataset.title = shortcut.title;
    ele.innerHTML = `<i class="title">${shortcut.title}</i>  <i class="bi bi-pencil-square"></i><i class="bi bi-trash"></i>`;

    // add edit click event
    ele.querySelector('i.bi-pencil-square')
        .addEventListener('click', EditFavSystemPromptHandler);

    // add delete click event
    ele.querySelector('i.bi-trash')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            ConfirmModal('delete saved prompt', async () => {
                libs.evtTarget(evt).parentElement.remove();

                let shortcuts = await libs.KvGet(KvKeyPromptShortCuts);
                shortcuts = shortcuts.filter((item) => item.title !== shortcut.title);
                await libs.KvSet(KvKeyPromptShortCuts, shortcuts);
            })
        });

    // add click event
    // replace system prompt
    ele.addEventListener('click', async (evt) => {
        evt.stopPropagation();
        const evtTarget = libs.evtTarget(evt);

        const promptInput = configContainer.querySelector('.system-prompt .input');
        const badgetEle = evtTarget.closest('.badge');
        const prompt = badgetEle.dataset.prompt;

        await OpenaiChatStaticContext(prompt);
        promptInput.value = prompt;
    });

    // add to html
    promptShortcutContainer.appendChild(ele);
}

async function setupPromptManager () {
    // restore shortcuts from kv
    {
        // bind default prompt shortcuts
        configContainer
            .querySelector('.prompt-shortcuts .badge')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptInput = configContainer.querySelector('.system-prompt .input');
                promptInput.value = libs.evtTarget(evt).dataset.prompt;
                await OpenaiChatStaticContext(libs.evtTarget(evt).dataset.prompt);
            })

        const shortcuts = await loadPromptShortcutsFromStorage();
        await Promise.all(shortcuts.map(async (shortcut) => {
            await appendPromptShortcut(shortcut, false);
        }));
    }

    // bind star prompt
    const saveSystemPromptModelEle = document.querySelector('#save-system-prompt.modal');
    const saveSystemPromptModal = new window.bootstrap.Modal(saveSystemPromptModelEle);
    {
        configContainer
            .querySelector('.system-prompt .bi.save-prompt')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptInput = configContainer
                    .querySelector('.system-prompt .input');

                saveSystemPromptModelEle
                    .querySelector('.modal-body textarea.user-input')
                    .innerHTML = promptInput.value;

                saveSystemPromptModal.show();
            });
    }

    // bind prompt market modal
    {
        configContainer
            .querySelector('.system-prompt .bi.open-prompt-market')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptMarketModalEle = document.querySelector('#prompt-market.modal');
                const promptMarketModal = new window.bootstrap.Modal(promptMarketModalEle);
                promptMarketModal.show();
            });
    }

    // bind save button in system-prompt modal
    {
        systemPromptModalCallback = async (evt) => {
            evt.stopPropagation();
            const titleInput = saveSystemPromptModelEle
                .querySelector('.modal-body input.title');
            const descriptionInput = saveSystemPromptModelEle
                .querySelector('.modal-body textarea.user-input');

            // trim space
            titleInput.value = titleInput.value.trim();
            descriptionInput.value = descriptionInput.value.trim();

            // if title is empty, set input border to red
            if (titleInput.value === '') {
                titleInput.classList.add('border-danger');
                return;
            }

            const shortcut = {
                title: titleInput.value,
                description: descriptionInput.value
            };

            appendPromptShortcut(shortcut, true);

            // clear input
            titleInput.value = '';
            descriptionInput.value = '';
            titleInput.classList.remove('border-danger');
            saveSystemPromptModal.hide();
        }
    }

    // fill chat prompts market
    const promptMarketModal = document.querySelector('#prompt-market');
    const promptInput = promptMarketModal.querySelector('textarea.prompt-content');
    const promptTitle = promptMarketModal.querySelector('input.prompt-title');
    {
        window.chatPrompts.forEach((prompt) => {
            const ele = document.createElement('span');
            ele.classList.add('badge', 'text-bg-info');
            ele.dataset.description = prompt.description;
            ele.dataset.title = prompt.title;
            ele.innerHTML = ` ${prompt.title}  <i class="bi bi-plus-circle"></i>`;

            // add click event
            // replace system prompt
            ele.addEventListener('click', async (evt) => {
                evt.stopPropagation();

                promptInput.value = libs.evtTarget(evt).dataset.description;
                promptTitle.value = libs.evtTarget(evt).dataset.title;
            });

            promptMarketModal.querySelector('.prompt-labels').appendChild(ele);
        });
    }

    // bind chat prompts market add button
    {
        promptMarketModal.querySelector('.modal-body .save')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                // trim and check empty
                promptTitle.value = promptTitle.value.trim();
                promptInput.value = promptInput.value.trim();
                if (promptTitle.value === '') {
                    promptTitle.classList.add('border-danger');
                    return;
                }
                if (promptInput.value === '') {
                    promptInput.classList.add('border-danger');
                    return;
                }

                const shortcut = {
                    title: promptTitle.value,
                    description: promptInput.value
                };

                appendPromptShortcut(shortcut, true);

                promptTitle.value = '';
                promptInput.value = '';
                promptTitle.classList.remove('border-danger');
                promptInput.classList.remove('border-danger');
            });
    }
}

/**
 * setup private dataset modal
 */
async function setupPrivateDataset () {
    const pdfchatModalEle = document.querySelector('#modal-pdfchat');

    // bind header's custom qa button
    {
        // bind pdf-file modal
        const pdfFileModalEle = document.querySelector('#modal-pdfchat');
        const pdfFileModal = new window.bootstrap.Modal(pdfFileModalEle);

        document
            .querySelector('#headerbar .qa-models a[data-model="qa-custom"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                pdfFileModal.show();
            });
    }

    // bind datakey to kv
    {
        const datakeyEle = pdfchatModalEle
            .querySelector('div[data-field="data-key"] input');

        datakeyEle.value = await libs.KvGet(KvKeyCustomDatasetPassword);

        // set default datakey
        if (!datakeyEle.value) {
            datakeyEle.value = libs.RandomString(16);
            await libs.KvSet(KvKeyCustomDatasetPassword, datakeyEle.value);
        }

        datakeyEle
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                await libs.KvSet(KvKeyCustomDatasetPassword, libs.evtTarget(evt).value);
            });
    }

    // bind file upload
    {
        // when user choosen file, get file name of
        // pdfchatModalEle.querySelector('div[data-field="pdffile"] input').files[0]
        // and set to dataset-name input
        pdfchatModalEle
            .querySelector('div[data-field="pdffile"] input')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();

                if (libs.evtTarget(evt).files.length === 0) {
                    return;
                }

                let filename = libs.evtTarget(evt).files[0].name;
                const fileext = filename.substring(filename.lastIndexOf('.')).toLowerCase();

                if (['.pdf', '.md', '.ppt', '.pptx', '.doc', '.docx'].indexOf(fileext) === -1) {
                    // remove choosen
                    pdfchatModalEle
                        .querySelector('div[data-field="pdffile"] input').value = '';

                    showalert('warning', 'currently only support pdf file');
                    return;
                }

                // remove extension and non-ascii charactors
                filename = filename.substring(0, filename.lastIndexOf('.'));
                filename = filename.replace(/[^a-zA-Z0-9]/g, '_');

                pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input')
                    .value = filename;
            });

        // bind upload button
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="upload"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                if (pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files.length === 0) {
                    showalert('warning', 'please choose a pdf file before upload');
                    return;
                }

                const sconfig = await getChatSessionConfig();

                // build post form
                const form = new FormData();
                form.append('file', pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files[0]);
                form.append('file_key', pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input').value);
                form.append('data_key', pdfchatModalEle
                    .querySelector('div[data-field="data-key"] input').value);
                // and auth token to header
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                headers.append('X-Laisky-Api-Base', sconfig.api_base);

                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'POST',
                        headers,
                        body: form
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    showalert('success', 'upload dataset success, please wait few minutes to process');
                } catch (err) {
                    showalert('danger', `upload dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }
            });
    }

    // bind delete datasets buttion
    const bindDatasetDeleteBtn = () => {
        const datasets = pdfchatModalEle
            .querySelectorAll('div[data-field="dataset"] .dataset-item .bi-trash');

        if (datasets === null || datasets.length === 0) {
            return;
        }

        datasets.forEach((ele) => {
            ele.addEventListener('click', async (evt) => {
                evt.stopPropagation();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                // headers.append("X-PDFCHAT-PASSWORD", await libs.KvGet(KvKeyCustomDatasetPassword));

                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'DELETE',
                        headers,
                        body: JSON.stringify({
                            datasets: [libs.evtTarget(evt).closest('.dataset-item').getAttribute('data-filename')]
                        })
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }
                    await resp.json();
                } catch (err) {
                    showalert('danger', `delete dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                // remove dataset item
                libs.evtTarget(evt).closest('.dataset-item').remove();
            });
        });
    }

    // bind list datasets
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="refresh"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-PDFCHAT-PASSWORD', await libs.KvGet(KvKeyCustomDatasetPassword));

                let body;
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'GET',
                        cache: 'no-cache',
                        headers
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                const datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let datasetsHTML = '';

                // add processing files
                // show processing files in grey and progress bar
                body.datasets.forEach((dataset) => {
                    switch (dataset.taskStatus) {
                    case ChatTaskStatusDone:
                        datasetsHTML += `
                                <div class="d-flex justify-content-between align-items-center dataset-item" data-filename="${dataset.name}">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch">
                                                <input dir="auto" class="form-check-input" type="checkbox">
                                                <label class="form-check-label" for="flexSwitchCheckChecked">${dataset.name}</label>
                                            </div>
                                        </div>
                                        <div class="col-5">
                                        </div>
                                        <div class="col-2 text-end">
                                            <i class="bi bi-trash"></i>
                                        </div>
                                    </div>
                                </div>`;
                        break;
                    case ChatTaskStatusProcessing:
                        datasetsHTML += `
                                <div class="d-flex justify-content-between align-items-center dataset-item" data-filename="${dataset.name}">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch">
                                                <input dir="auto" class="form-check-input" type="checkbox" disabled>
                                                <label class="form-check-label" for="flexSwitchCheckChecked">${dataset.name}</label>
                                            </div>
                                        </div>
                                        <div class="col-5">
                                            <div class="progress" role="progressbar" aria-label="Example with label" aria-valuenow="${dataset.progress}" aria-valuemin="0" aria-valuemax="100">
                                                <div class="progress-bar" style="width: ${dataset.progress}%">wait few minutes</div>
                                            </div>
                                        </div>
                                        <div class="col-2 text-end">
                                            <i class="bi bi-trash"></i>
                                        </div>
                                    </div>
                                </div>`;
                        break;
                    }
                });

                datasetListEle.innerHTML = datasetsHTML;

                // selected binded datasets
                body.selected.forEach((dataset) => {
                    datasetListEle
                        .querySelector(`div[data-filename="${dataset}"] input[type="checkbox"]`)
                        .checked = true;
                })

                bindDatasetDeleteBtn();
            });
    }

    // bind list chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="list-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                new window.bootstrap.Dropdown(libs.evtTarget(evt).closest('.dropdown')).hide();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-PDFCHAT-PASSWORD', await libs.KvGet(KvKeyCustomDatasetPassword));

                let body;
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/ctx/list', {
                        method: 'GET',
                        cache: 'no-cache',
                        headers
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                const datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let chatbotsHTML = '';

                body.chatbots.forEach((chatbot) => {
                    let selectedHTML = '';
                    if (chatbot === body.current) {
                        selectedHTML = 'checked';
                    }

                    chatbotsHTML += `
                        <div class="d-flex justify-content-between align-items-center chatbot-item" data-name="${chatbot}">
                            <div class="container-fluid row">
                                <div class="col-5">
                                    <div class="form-check form-switch">
                                        <input dir="auto" class="form-check-input" type="checkbox" ${selectedHTML}>
                                        <label class="form-check-label" for="flexSwitchCheckChecked">${chatbot}</label>
                                    </div>
                                </div>
                                <div class="col-5">
                                </div>
                                <div class="col-2 text-end">
                                    <i class="bi bi-trash"></i>
                                </div>
                            </div>
                        </div>`
                });

                datasetListEle.innerHTML = chatbotsHTML;

                // bind active new selected chatbot
                datasetListEle
                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                    .forEach((ele) => {
                        ele.addEventListener('change', async (evt) => {
                            evt.stopPropagation();
                            const evtTarget = libs.evtTarget(evt);

                            if (!evtTarget.checked) {
                                // at least one chatbot should be selected
                                evtTarget.checked = true;
                                return;
                            } else {
                                // uncheck other chatbot
                                datasetListEle
                                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                                    .forEach((ele) => {
                                        if (ele !== evtTarget) {
                                            ele.checked = false;
                                        }
                                    });
                            }

                            const headers = new Headers();
                            headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                            headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                            headers.append('X-Laisky-Api-Base', sconfig.api_base);

                            try {
                                ShowSpinner();
                                const chatbotName = libs.evtTarget(evt).closest('.chatbot-item').getAttribute('data-name');
                                const resp = await fetch('/ramjet/gptchat/ctx/active', {
                                    method: 'POST',
                                    headers,
                                    body: JSON.stringify({
                                        data_key: await libs.KvGet(KvKeyCustomDatasetPassword),
                                        chatbot_name: chatbotName
                                    })
                                });

                                if (!resp.ok || resp.status !== 200) {
                                    throw new Error(`${resp.status} ${await resp.text()}`);
                                }

                                // const body = await resp.json();
                                showalert('success', `active chatbot success, you can chat with ${chatbotName} now`);
                            } catch (err) {
                                showalert('danger', `active chatbot failed, ${err.message}`);
                                throw err;
                            } finally {
                                HideSpinner();
                            }
                        });
                    });
            });
    }

    // bind share chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="share-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const evtTarget = libs.evtTarget(evt);

                new window.bootstrap.Dropdown(evtTarget.closest('.dropdown')).hide();

                const checkedChatbotEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"] .chatbot-item input[type="checkbox"]:checked');
                if (!checkedChatbotEle) {
                    showalert('danger', 'please click [Chatbot List] first');
                    return;
                }

                const chatbotName = checkedChatbotEle.closest('.chatbot-item').getAttribute('data-name');

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-Laisky-Api-Base', sconfig.api_base);

                let respBody
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/ctx/share', {
                        method: 'POST',
                        headers,
                        body: JSON.stringify({
                            chatbot_name: chatbotName,
                            data_key: await libs.KvGet(KvKeyCustomDatasetPassword)
                        })
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    respBody = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                // open new tab page
                const sharedChatbotUrl = `${location.origin}/?chatmodel=qa-shared&uid=${respBody.uid}&chatbot_name=${respBody.chatbot_name}`;
                showalert('info', `open ${sharedChatbotUrl}`);
                open(sharedChatbotUrl, '_blank');
            });
    }

    // build custom chatbot
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="build-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const evtTarget = libs.evtTarget(evt);

                new window.bootstrap.Dropdown(evtTarget.closest('.dropdown')).hide();

                const selectedDatasets = [];
                pdfchatModalEle
                    .querySelectorAll('div[data-field="dataset"] .dataset-item input[type="checkbox"]')
                    .forEach((ele) => {
                        if (ele.checked) {
                            selectedDatasets.push(
                                ele.closest('.dataset-item').getAttribute('data-filename'));
                        }
                    });

                if (selectedDatasets.length === 0) {
                    showalert('warning', 'please select at least one dataset, click [List Dataset] button to fetch dataset list');
                    return;
                }

                // ask chatbot's name
                SingleInputModal('build bot', 'chatbot name', async (botname) => {
                    // botname should be 1-32 ascii characters
                    if (!botname.match(/^[a-zA-Z0-9_-]{1,32}$/)) {
                        showalert('warning', 'chatbot name should be 1-32 ascii characters');
                        return;
                    }

                    const sconfig = await getChatSessionConfig();
                    const headers = new Headers();
                    headers.append('Content-Type', 'application/json');
                    headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                    headers.append('X-Laisky-User-Id', await libs.getSHA1(sconfig.api_token));
                    headers.append('X-Laisky-Api-Base', sconfig.api_base);

                    try { // build chatbot
                        ShowSpinner()
                        const resp = await fetch('/ramjet/gptchat/ctx/build', {
                            method: 'POST',
                            headers,
                            body: JSON.stringify({
                                chatbot_name: botname,
                                datasets: selectedDatasets,
                                data_key: pdfchatModalEle
                                    .querySelector('div[data-field="data-key"] input').value
                            })
                        });

                        if (!resp.ok || resp.status !== 200) {
                            throw new Error(`${resp.status} ${await resp.text()}`);
                        }

                        showalert('success', 'build dataset success, you can chat now');
                    } catch (err) {
                        showalert('danger', `build dataset failed, ${err.message}`);
                        throw err;
                    } finally {
                        HideSpinner();
                    }
                });
            });
    }
}
