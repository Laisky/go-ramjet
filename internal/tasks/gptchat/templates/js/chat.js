'use strict';

const libs = window.libs;

const robotIcon = '🤖️';

const ChatModelTurbo35 = 'gpt-3.5-turbo';
// const ChatModelTurbo35V1106 = 'gpt-3.5-turbo-1106';
// const ChatModelTurbo35V0125 = 'gpt-3.5-turbo-0125';
// const ChatModelTurbo35_16K = "gpt-3.5-turbo-16k";
// const ChatModelTurbo35_0613 = "gpt-3.5-turbo-0613";
// const ChatModelTurbo35_0613_16K = "gpt-3.5-turbo-16k-0613";
// const ChatModelGPT4 = "gpt-4";
const ChatModelGPT4Turbo = 'gpt-4-turbo';
const ChatModelGPT4O = 'gpt-4o';
const ChatModelDeepSeekChat = 'deepseek-chat';
const ChatModelDeepSeekCoder = 'deepseek-coder';
// const ChatModelGPT4Turbo1106 = 'gpt-4-1106-preview';
// const ChatModelGPT4Turbo0125 = 'gpt-4-0125-preview';
// const ChatModelGPT4Vision = 'gpt-4-vision-preview';
// const ChatModelClaude1 = 'claude-instant-1';
// const ChatModelClaude2 = 'claude-2';
const ChatModelClaude3Opus = 'claude-3-opus';
const ChatModelClaude3Sonnet = 'claude-3-sonnet';
const ChatModelClaude3Haiku = 'claude-3-haiku';
// const ChatModelGPT4_0613 = "gpt-4-0613";
// const ChatModelGPT4_32K = "gpt-4-32k";
// const ChatModelGPT4_0613_32K = "gpt-4-32k-0613";
const ChatModelGeminiPro = 'gemini-pro';
const ChatModelGeminiProVision = 'gemini-pro-vision';
// const ChatModelGroqLlama2With70B4K = 'llama2-70b-4096';
// const ChatModelGroqMixtral8x7B32K = 'mixtral-8x7b-32768';
const ChatModelGroqGemma7b = 'gemma-7b-it';
const ChatModelGroqllama3With70B = 'llama3-70b-8192';
const ChatModelGroqllama3With8B = 'llama3-8b-8192';
const QAModelBasebit = 'qa-bbt-xego';
const QAModelSecurity = 'qa-security';
const QAModelImmigrate = 'qa-immigrate';
const QAModelCustom = 'qa-custom';
const QAModelShared = 'qa-shared';
const CompletionModelDavinci3 = 'text-davinci-003';
const ImageModelDalle2 = 'dall-e-2';
const ImageModelDalle3 = 'dall-e-3';
const ImageModelSdxlTurbo = 'sdxl-turbo';
const ImageModelImg2Img = 'img-to-img';

// casual chat models

const ChatModels = [
    ChatModelTurbo35,
    // ChatModelTurbo35V1106,
    // ChatModelTurbo35V0125,
    // ChatModelGPT4,
    ChatModelGPT4Turbo,
    ChatModelGPT4O,
    ChatModelDeepSeekChat,
    ChatModelDeepSeekCoder,
    // ChatModelGPT4Turbo1106,
    // ChatModelGPT4Turbo0125,
    // ChatModelClaude1,
    // ChatModelClaude2,
    ChatModelClaude3Opus,
    ChatModelClaude3Sonnet,
    ChatModelClaude3Haiku,
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    ChatModelGroqGemma7b,
    ChatModelGroqllama3With70B,
    ChatModelGroqllama3With8B,
    // ChatModelGPT4Vision,
    ChatModelGeminiPro,
    ChatModelGeminiProVision
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
    ChatModelGeminiProVision,
    ChatModelClaude3Opus,
    ChatModelClaude3Sonnet,
    ChatModelClaude3Haiku,
    ImageModelSdxlTurbo,
    ImageModelImg2Img
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
    ImageModelSdxlTurbo,
    ImageModelImg2Img
];
const CompletionModels = [
    CompletionModelDavinci3
];
const FreeModels = [
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    ChatModelGroqGemma7b,
    ChatModelGroqllama3With70B,
    ChatModelGroqllama3With8B,
    ChatModelTurbo35,
    ChatModelDeepSeekChat,
    ChatModelDeepSeekCoder,
    // ChatModelTurbo35V0125,
    ChatModelGeminiPro,
    ChatModelGeminiProVision,
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    ImageModelSdxlTurbo,
    ImageModelImg2Img
];
const AllModels = [].concat(ChatModels, QaModels, ImageModels, CompletionModels);

// custom dataset's end-to-end password
const KvKeyPinnedMaterials = 'config_api_pinned_materials';
const KvKeyAllowedModels = 'config_chat_models';
const KvKeyCustomDatasetPassword = 'config_chat_dataset_key';
const KvKeyPromptShortCuts = 'config_prompt_shortcuts';
const KvKeyPrefixSessionHistory = 'chat_user_session_';
const KvKeyPrefixSessionConfig = 'chat_user_config_';
const KvKeyPrefixSelectedSession = 'config_selected_session';
const KvKeySyncKey = 'config_sync_key';
const KvKeyAutoSyncUserConfig = 'config_auto_sync_user_config';
const KvKeyVersionDate = 'config_version_date';

const RoleHuman = 'user';
const RoleSystem = 'system';
const RoleAI = 'assistant';

const chatContainer = document.getElementById('chatContainer');
const configContainer = document.getElementById('hiddenChatConfigSideBar');
const chatPromptInputEle = chatContainer.querySelector('.user-input .input.prompt');
const chatPromptInputBtn = chatContainer.querySelector('.user-input .btn.send');

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
let globalAIRespSSE, globalAIRespEle, globalAIRespHeartBeatTimer;

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
    let selectedModel = sconfig.selected_model || ChatModelTurbo35;

    if (!AllModels.includes(selectedModel)) {
        selectedModel = ChatModelTurbo35;
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

    await setupModals();
    await dataMigrate();
    await setupHeader();
    setupSingleInputModal();

    await checkUpgrade();
    await setupChatJs();
};
main();

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

        // set alpha to where the image is been drawn to yellow
        for (let i = 0; i < imageData.data.length; i += 4) {
            if (imageData.data[i] >= 250 && imageData.data[i + 1] >= 250 && imageData.data[i + 2] <= 5) {
                maskData[i + 3] = 0; // set alpha to 0
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

        const formData = new FormData();
        formData.append('image', rawImgBlob, 'image.png');
        formData.append('mask', maskBlob, 'mask.png');
        formData.append('prompt', prompt);
        formData.append('model', ImageModelDalle2);
        formData.append('response_format', 'b64_json');

        imgEditModal.hide();

        try {
            ShowSpinner();
            await reloadAiResp(chatID, async () => {
                const sconfig = await getChatSessionConfig();
                const resp = await fetch('/oneapi/v1/images/edits', {
                    method: 'POST',
                    body: formData,
                    headers: {
                        Authorization: `Bearer ${sconfig.api_token}`
                    }
                });
                const respData = await resp.json();
                const respImageData = respData.data[0].b64_json;
                const content = `<div class="ai-resp-image">
                        <div class="hover-btns">
                            <i class="bi bi-pencil-square"></i>
                        </div>
                        <img src="data:image/png;base64,${respImageData}">
                    </div>`

                await append2Chats({
                    chatID,
                    role: RoleAI,
                    model: ImageModelDalle2,
                    content
                });
                await appendChats2Storage({
                    role: RoleAI,
                    chatID,
                    model: ImageModelDalle2,
                    content
                });
            });
        } catch (e) {
            abortAIResp(e);
        } finally {
            HideSpinner();
        }
    };
}

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

            await libs.KvSet(skey, sconfig);
        }
    }

    // set api token from url params
    {
        const apikey = new URLSearchParams(location.search).get('apikey');

        if (apikey) {
            // remove apikey from url params
            const url = new URL(location.href);
            url.searchParams.delete('apikey');
            window.history.pushState({}, document.title, url);
            sconfig.api_token = apikey;
            await libs.KvSet(skey, sconfig);
        }
    }

    // list all session configs
    await Promise.all((await libs.KvList()).map(async (key) => {
        if (!key.startsWith(KvKeyPrefixSessionConfig)) {
            return;
        }

        let eachSconfig = await libs.KvGet(key);
        if (!eachSconfig) {
            eachSconfig = newSessionConfig();
        }

        // set default api_token
        if (!eachSconfig.api_token || eachSconfig.api_token === 'DEFAULT_PROXY_TOKEN') {
            eachSconfig.api_token = 'FREETIER-' + libs.RandomString(32);
        }
        // set default api_base
        if (!eachSconfig.api_base) {
            eachSconfig.api_base = 'https://api.openai.com';
        }

        // set default chat controller
        if (!eachSconfig.chat_switch) {
            eachSconfig.chat_switch = {
                all_in_one: false,
                disable_https_crawler: true,
                enable_google_search: false
            };
        }
        if (eachSconfig.chat_switch.all_in_one === undefined) {
            eachSconfig.chat_switch.all_in_one = false;
        }

        // change model
        if (!eachSconfig.selected_model || !AllModels.includes(eachSconfig.selected_model)) {
            eachSconfig.selected_model = ChatModelTurbo35;
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
        }));

        await libs.KvSet(key, chats);
    }));
}

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
            ConfirmModal(`New version found ${localVer} -> ${currentVer}, reload page to upgrade?`, async () => {
                location.reload();
            });
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

/** setup header bar
 *
 */
async function setupHeader () {
    const headerBarEle = document.getElementById('headerbar');
    let allowedModels = [];
    const sconfig = await getChatSessionConfig();

    // setup chat models
    {
        // set default chat model
        let selectedModel = await OpenaiSelectedModel();

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

        const modelsContainer = document.querySelector('#headerbar .chat-models');
        let modelsEle = '';
        const respData = await response.json();
        if (respData.allowed_models.includes('*')) {
            respData.allowed_models = Array.from(AllModels);
        } else {
            respData.allowed_models.push(QAModelCustom, QAModelShared);
        }
        respData.allowed_models = respData.allowed_models.filter((model) => {
            return AllModels.includes(model);
        });

        respData.allowed_models.sort();
        await libs.KvSet(KvKeyAllowedModels, respData.allowed_models);
        allowedModels = respData.allowed_models;

        if (!allowedModels.includes(selectedModel)) {
            selectedModel = '';
            AllModels.forEach((model) => {
                if (selectedModel !== '' || !allowedModels.includes(model)) {
                    return;
                }

                if (model.startsWith('gpt-') || model.startsWith('gemini-')) {
                    selectedModel = model;
                }
            });

            const sconfig = await getChatSessionConfig();
            sconfig.selected_model = selectedModel;
            await saveChatSessionConfig(sconfig);
        }

        // add hint to input text
        // chatPromptInputEle.attributes
        //     .placeholder.value = `[${selectedModel}] CTRL+Enter to send`;

        const unsupportedModels = [];
        respData.allowed_models.forEach((model) => {
            if (!ChatModels.includes(model)) {
                unsupportedModels.push(model);
                return;
            }

            if (FreeModels.includes(model)) {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model}</a></li>`;
            } else {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model} <i class="bi bi-coin"></i></a></li>`;
            }
        });
        modelsContainer.innerHTML = modelsEle;
    }

    // FIXME
    // if (unsupportedModels.length > 0) {
    //     showalert("warning", `there are some models enabled for your account, but not supported in the frontend, `
    //         + `maybe you need refresh your browser. if this warning still exists, `
    //         + `please contact us via <a href="mailto:chat-support@laisky.com">chat-support@laisky.com</a>. unsupported models: ${unsupportedModels.join(", ")}`);
    // }

    // setup chat qa models
    {
        const qaModelsContainer = headerBarEle.querySelector('.dropdown-menu.qa-models');
        let modelsEle = '';

        const allowedQaModels = [QAModelCustom, QAModelShared];
        window.data.qa_chat_models.forEach((item) => {
            allowedQaModels.push(item.name);
        });

        allowedModels.forEach((model) => {
            if (!QaModels.includes(model) || !allowedQaModels.includes(model)) {
                return;
            }

            if (FreeModels.includes(model)) {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model}</a></li>`;
            } else {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model} <i class="bi bi-coin"></i></a></li>`;
            }
        });
        qaModelsContainer.innerHTML = modelsEle;
    }

    // setup chat image models
    {
        const imageModelsContainer = headerBarEle.querySelector('.dropdown-menu.image-models');
        let modelsEle = '';
        allowedModels.forEach((model) => {
            if (!ImageModels.includes(model)) {
                return;
            }

            if (FreeModels.includes(model)) {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model}</a></li>`;
            } else {
                modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model} <i class="bi bi-coin"></i></a></li>`;
            }
        });
        imageModelsContainer.innerHTML = modelsEle;
    }

    // listen click events
    const modelElems = document
        .querySelectorAll('#headerbar .chat-models li a, ' +
            '#headerbar .qa-models li a, ' +
            '#headerbar .image-models li a'
        );
    modelElems.forEach((elem) => {
        elem.addEventListener('click', async (evt) => {
            evt.preventDefault();
            modelElems.forEach((elem) => {
                elem.classList.remove('active');
            })

            evt.target.classList.add('active');
            const selectedModel = evt.target.dataset.model;

            const sconfig = await getChatSessionConfig();
            sconfig.selected_model = selectedModel;
            await saveChatSessionConfig(sconfig);

            // add active to class
            document.querySelectorAll('#headerbar .navbar-nav a.dropdown-toggle')
                .forEach((elem) => {
                    elem.classList.remove('active');
                });
            evt.target.closest('.dropdown').querySelector('a.dropdown-toggle').classList.add('active');

            // add hint to input text
            chatPromptInputEle.attributes.placeholder.value = `[${selectedModel}] CTRL+Enter to send`;
        });
    });
}

// eslint-disable-next-line no-unused-vars
async function setupChatJs () {
    await setupSessionManager();
    await setupConfig();
    await setupChatInput();
    await setupPromptManager();
    await setupPrivateDataset();
    setupGlobalAiRespHeartbeatTimer();
    setInterval(fetchImageDrawingResultBackground, 3000);
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

        if (Date.now() - globalAIRespHeartBeatTimer > 1000 * 60) {
            console.warn('no heartbeat for 60s, abort AI resp');
            await abortAIResp('no heartbeat for 60s, abort AI resp automatically');
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
    let activeSession = document.querySelector('#sessionManager .card-body button.active');
    if (activeSession) {
        return parseInt(activeSession.dataset.session);
    }

    activeSession = await libs.KvGet(KvKeyPrefixSelectedSession);
    if (activeSession) {
        if (!document.querySelector(`#sessionManager .card-body button[data-session="${activeSession}"]`)) {
            // if session not exists on tabs, choose first tab's session as active session
            const firstSessionBtn = document.querySelector('#sessionManager .card-body button');
            if (firstSessionBtn) {
                activeSession = parseInt(firstSessionBtn.dataset.session || '1');
                await libs.KvSet(KvKeyPrefixSelectedSession, activeSession);
            }
        }

        return parseInt(activeSession);
    }

    return 1;
}

async function listenSessionSwitch (evt) {
    evt = libs.evtTarget(evt);
    if (!evt.classList.contains('list-group-item')) {
        evt = evt.closest('.list-group-item');
    }
    const activeSid = parseInt(evt.dataset.session);

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
            #sessionManager .sessions .list-group-item,
            #chatContainer .sessions .list-group-item
        `)
        .forEach((item) => {
            if (parseInt(item.dataset.session) === activeSid) {
                item.classList.add('active');
            } else {
                item.classList.remove('active');
            }
        })

    // restore session history
    chatContainer.querySelector('.conservations .chats').innerHTML = '';
    await Promise.all(Array.from(await sessionChatHistory(activeSid)).map(async (item) => {
        append2Chats({
            chatID: item.chatID,
            role: item.role,
            content: item.content,
            isHistory: true,
            attachHTML: item.attachHTML,
            rawContent: item.rawContent,
            model: item.model,
            costUsd: item.costUsd
        });
        if (item.role === RoleAI) {
            await renderAfterAiResp(item.chatID);
        }
    }));

    await libs.KvSet(KvKeyPrefixSelectedSession, activeSid);
    await updateConfigFromSessionConfig();
    await autoToggleUserImageUploadBtn();
    libs.EnableTooltipsEverywhere();
}

/**
 * Fetches the image drawing result background for the AI response and displays it in the chat container.
 * @returns {Promise<void>}
 */
async function fetchImageDrawingResultBackground () {
    const elements = chatContainer
        .querySelectorAll('.role-ai .ai-response[data-task-type="image"][data-status="waiting"]') || [];

    await Promise.all(Array.from(elements).map(async (item) => {
        if (item.dataset.status !== 'waiting') {
            return;
        }

        // const taskId = item.dataset.taskId;
        const imageUrls = JSON.parse(item.dataset.imageUrls) || [];
        const chatId = item.closest('.role-ai').dataset.chatid;

        try {
            await Promise.all(imageUrls.map(async (imageUrl) => {
                // check any err msg
                const errFileUrl = imageUrl.slice(0, imageUrl.lastIndexOf('-')) + '.err.txt';
                const errFileResp = await fetch(`${errFileUrl}?rr=${libs.RandomString(12)}`, {
                    method: 'GET',
                    cache: 'no-cache'
                });
                if (errFileResp.ok || errFileResp.status === 200) {
                    const errText = await errFileResp.text();
                    item.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${errText}</pre>`;
                    checkIsImageAllSubtaskDone(item, imageUrl, false);
                    await appendChats2Storage({
                        role: RoleAI,
                        chatID: chatId,
                        model: item.dataset.model,
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
                checkIsImageAllSubtaskDone(item, imageUrl, true);
            }))
        } catch (err) {
            console.warn('fetch img result, ' + err);
        };
    }));
}

/** append chat to chat container
 *
 * @param {element} item ai respnse
 * @param {string} imageUrl current subtask's image url
 * @param {boolean} succeed is current subtask succeed
 */
function checkIsImageAllSubtaskDone (item, imageUrl, succeed) {
    let processingImageUrls = JSON.parse(item.dataset.imageUrls) || [];
    if (!processingImageUrls.includes(imageUrl)) {
        return;
    }

    const chatID = item.closest('.role-ai').dataset.chatid;

    // remove current subtask from imageUrls(tasks)
    processingImageUrls = processingImageUrls.filter((url) => url !== imageUrl)
    item.dataset.imageUrls = JSON.stringify(processingImageUrls);

    const succeedImageUrls = JSON.parse(item.dataset.succeedImageUrls || '[]');
    if (succeed) {
        succeedImageUrls.push(imageUrl);
        item.dataset.succeedImageUrls = JSON.stringify(succeedImageUrls);
    } else { // task failed
        processingImageUrls = [];
        item.dataset.imageUrls = JSON.stringify(processingImageUrls);
    }

    if (processingImageUrls.length === 0 && succeedImageUrls.length > 0) {
        item.dataset.status = 'done';
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
                evt = libs.evtTarget(evt);
                const sid = parseInt(evt.closest('.session').dataset.session);
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

            // if there is only one session, don't delete it
            if (document.querySelectorAll('#sessionManager .sessions .session').length === 1) {
                return;
            }

            const activeSid = await activeSessionID();
            const deleteSid = parseInt(libs.evtTarget(evt).closest('.session').dataset.session);
            ConfirmModal('Are you sure to delete this session?', async () => {
                await libs.KvDel(`${KvKeyPrefixSessionHistory}${deleteSid}`);
                await libs.KvDel(`${KvKeyPrefixSessionConfig}${deleteSid}`);
                document
                    .querySelector(`#sessionManager .sessions [data-session="${deleteSid}"]`).remove();
                chatContainer
                    .querySelector(`.sessions [data-session="${deleteSid}"]`).remove();

                if (activeSid === deleteSid) {
                    // current active session has been deleted, so need to switch to new session
                    const newSid = await activeSessionID();
                    await changeSession(newSid);
                }
            });
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
                    `<div class="list-group">
                        <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                            <div class="col">${sessionName}</div>
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-trash col-auto"></i>
                        </button>
                    </div>`);
            chatContainer
                .querySelector('.sessions')
                .insertAdjacentHTML(
                    'beforeend',
                    `<div class="list-group">
                        <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                            <div class="col">${sessionName}</div>
                        </button>
                    </div>`);
        }));

        // restore conservation history
        await Promise.all(Array.from(await activeSessionChatHistory()).map(async (item) => {
            append2Chats({
                chatID: item.chatID,
                role: item.role,
                content: item.content,
                isHistory: true,
                attachHTML: item.attachHTML,
                rawContent: item.rawContent,
                model: item.model,
                costUsd: item.costUsd
            });
            if (item.role === RoleAI) {
                await renderAfterAiResp(item.chatID);
            }
        }));
    }

    // add widget to scroll bottom
    {
        document.querySelector('#chatContainer .chatManager .card-footer .scroll-down')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                scrollChatToDown();
            });
    }

    // new session
    {
        document
            .querySelector('#sessionManager .btn.new-session')
            .addEventListener('click', async (evt) => {
                let maxSessionID = 0;
                (await libs.KvList()).forEach((key) => {
                    if (key.startsWith(KvKeyPrefixSessionHistory)) {
                        const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));
                        if (sessionID > maxSessionID) {
                            maxSessionID = sessionID;
                        }
                    }
                });

                // deactive all sessions
                document.querySelectorAll(`
                    #sessionManager .sessions .list-group-item.active,
                    #chatContainer .sessions .list-group-item.active
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
                        `<div class="list-group">
                            <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                <div class="col">${newSessionID}</div>
                                <i class="bi bi-pencil-square"></i>
                                <i class="bi bi-trash col-auto"></i>
                            </button>
                        </div>`);
                chatContainer
                    .querySelector('.sessions')
                    .insertAdjacentHTML(
                        'beforeend',
                        `<div class="list-group">
                            <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                <div class="col">${newSessionID}</div>
                            </button>
                        </div>`);

                // save new session history and config
                await libs.KvSet(kvSessionKey(newSessionID), []);
                const oldSessionConfig = await getChatSessionConfig();
                const sconfig = newSessionConfig();

                // keep old session's api token and api base
                sconfig.api_token = oldSessionConfig.api_token;
                sconfig.api_base = oldSessionConfig.api_base;

                await libs.KvSet(`${KvKeyPrefixSessionConfig}${newSessionID}`, sconfig);
                await libs.KvSet(KvKeyPrefixSelectedSession, newSessionID);

                // bind session switch listener for new session
                document
                    .querySelector(`
                        #sessionManager .sessions [data-session="${newSessionID}"],
                        #chatContainer .sessions [data-session="${newSessionID}"]
                    `)
                    .addEventListener('click', listenSessionSwitch);

                bindSessionEditBtn();
                bindSessionDeleteBtn();
                await updateConfigFromSessionConfig();
            })
    }

    // bind session switch
    {
        document
            .querySelectorAll(`
                #sessionManager .sessions .list-group .session,
                #chatContainer .sessions .list-group .session
            `)
            .forEach((item) => {
                item.addEventListener('click', listenSessionSwitch);
            })
    }

    bindSessionEditBtn();
    bindSessionDeleteBtn();
}

// remove chat in storage by chatid
async function removeChatInStorage (chatid) {
    if (!chatid) {
        throw new Error('chatid is required');
    }

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    let session = await activeSessionChatHistory();

    // remove all chats with the same chatid
    session = session.filter((item) => item.chatID !== chatid);

    await libs.KvSet(storageActiveSessionKey, session);
}

/** append or update chat history by chatID and role
 *
 * @param {object} chatItem - chat item
 *   @property {string} chatID - chat id
 *   @property {string} role - user or assistant
 *   @property {string} content - rendered chat content
 *   @property {string} attachHTML - chat content's attach html
 *   @property {string} rawContent - chat content's raw content
 *   @property {string} costUsd - chat cost in USD
 *   @property {string} model - chat model
*/
async function appendChats2Storage (chatItem) {
    if (!chatItem.chatID) {
        throw new Error('chatID is required');
    }

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    const session = await activeSessionChatHistory();

    // if chat is already in history, find and update it.
    let found = false;
    session.forEach((item, idx) => {
        if (item.chatID === chatItem.chatID && item.role === chatItem.role) {
            found = true;
            item.content = chatItem.content;
            item.attachHTML = chatItem.attachHTML;
            item.rawContent = chatItem.rawContent;
            item.model = chatItem.model;
            item.costUsd = chatItem.costUsd || item.costUsd;
        }
    });

    // if ai response is not in history, add it after user's chat which has same chatID
    if (!found && chatItem.role === RoleAI) {
        session.forEach((item, idx) => {
            if (item.chatID === chatItem.chatID) {
                found = true;
                if (item.role !== RoleAI) {
                    session.splice(idx + 1, 0, {
                        role: RoleAI,
                        chatID: chatItem.chatID,
                        content: chatItem.content,
                        attachHTML: chatItem.attachHTML,
                        rawContent: chatItem.rawContent,
                        model: chatItem.model,
                        costUsd: chatItem.costUsd
                    });
                }
            }
        });
    }

    // if chat is not in history, add it
    if (!found) {
        session.push({
            role: chatItem.role,
            chatID: chatItem.chatID,
            content: chatItem.content,
            attachHTML: chatItem.attachHTML,
            rawContent: chatItem.rawContent,
            model: chatItem.model,
            costUsd: chatItem.costUsd
        });
    }

    // save session chat history
    await libs.KvSet(storageActiveSessionKey, session);
}

function scrollChatToDown () {
    libs.ScrollDown(document.querySelector('html'));
    libs.ScrollDown(chatContainer.querySelector('.chatManager .conservations'));
}

function scrollToChat (chatEle) {
    chatEle.scrollIntoView({ behavior: 'smooth', block: 'end' });
}

/**
*
* Get the last N chat messages, which will be sent to the AI as context.
*
* @param {number} N - The number of messages to retrieve.
* @param {string} ignoredChatID - If ignoredChatID is not null, the chat with this chatid will be ignored.
* @returns {Array} An array of chat messages.
*/
async function getLastNChatMessages (N, ignoredChatID) {
    console.debug('getLastNChatMessages', N, ignoredChatID);

    const systemPrompt = await OpenaiChatStaticContext();
    const selectedModel = await OpenaiSelectedModel();

    if (selectedModel === ChatModelGeminiPro || N <= 1) {
        // one-api's gemoni-pro do not support context
        return [{
            role: RoleSystem,
            content: systemPrompt
        }];
    }

    const latestMessages = [];
    const historyMessages = await activeSessionChatHistory();
    let nHuman = 1;
    let latestRole = RoleHuman;
    for (let i = historyMessages.length - 1; i >= 0; i--) {
        const role = historyMessages[i].role;
        let content = historyMessages[i].rawContent || historyMessages[i].content;

        if (latestRole && latestRole === role) {
            // if latest role is same as current role, break
            console.warn(`latest role is same as current role, skip, latestRole=${latestRole}`);
            continue;
        }

        if (role !== RoleHuman && role !== RoleAI) {
            // exclude system message
            continue;
        }

        if (ignoredChatID && ignoredChatID === historyMessages[i].chatID) {
            // This is a reload request with edited chat,
            // ignore chat with same chatid to avoid duplicate context.
            continue;
        }

        if (role === RoleAI && content.includes('🔥Someting in trouble')) {
            // if AI response is error, replace it with a error message.
            // claude does not accept empty content.
            content = 'there is an error during AI response, please try again.';
        }

        latestRole = role;
        // insert at the beginning, only keep role and content
        latestMessages.unshift({
            role,
            content
        });

        if (role === RoleHuman) {
            nHuman++;
            if (nHuman >= N) {
                break;
            }
        }
    }

    if (systemPrompt) {
        latestMessages.unshift({
            role: RoleSystem,
            content: systemPrompt
        });
    }

    return latestMessages;
}

function lockChatInput () {
    chatContainer.querySelectorAll('.role-human .form-control btn.save').forEach((item) => {
        item.classList.add('disabled');
    });
    chatContainer.querySelectorAll('.ai-response .operator .btn').forEach((item) => {
        item.classList.add('disabled');
    });
    chatPromptInputBtn.classList.add('disabled');
}
function unlockChatInput () {
    chatContainer.querySelectorAll('.role-human .form-control btn.save').forEach((item) => {
        item.classList.remove('disabled');
    });
    chatContainer.querySelectorAll('.ai-response .operator .btn').forEach((item) => {
        item.classList.remove('disabled');
    });
    chatPromptInputBtn.classList.remove('disabled');
}
function isAllowChatPrompInput () {
    return !chatPromptInputBtn.classList.contains('disabled');
}

function parseChatResp (chatmodel, payload) {
    if (!payload.choices || payload.choices.length === 0) {
        payload.choices = [{
            delta: {
                content: '',
                text: ''
            }
        }];
    }

    if (IsChatModel(chatmodel) || IsQaModel(chatmodel)) {
        return payload.choices[0].delta.content || '';
    } else if (IsCompletionModel(chatmodel)) {
        return payload.choices[0].text || '';
    } else {
        showalert('error', `Unknown chat model ${chatmodel}`);
    }
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
                const container = libs.evtTarget(evt).closest('.pinned-refs');
                const ele = libs.evtTarget(evt).closest('p');
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

/**
 * Sends an txt2image prompt to the server for the selected model and updates the current AI response element with the task information.
 * @param {string} chatID - The chat ID.
 * @param {string} selectedModel - The selected image model.
 * @param {HTMLElement} currentAIRespEle - The current AI response element to update with the task information.
 * @param {string} prompt - The image prompt to send to the server.
 * @throws {Error} Throws an error if the selected model is unknown or if the response from the server is not ok.
 */
async function sendTxt2ImagePrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;

    switch (selectedModel) {
    case ImageModelDalle3:
        url = '/images/generations';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
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
            prompt
        })
    });
    if (!resp.ok || resp.status !== 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.model = selectedModel;
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    let attachHTML = '';
    respData.image_urls.forEach((url) => {
        attachHTML += `<div class="ai-resp-image">
            <div class="hover-btns">
                <i class="bi bi-pencil-square"></i>
            </div>
            <img src="${url}">
        </div>`;
    })

    // save img to storage no matter it's done or not
    await appendChats2Storage({
        role: RoleAI,
        chatID,
        model: selectedModel,
        content: attachHTML
    });
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

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.model = selectedModel;
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

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

    await appendChats2Storage({
        role: RoleAI,
        chatID,
        model: selectedModel,
        content: attachHTML
    });
}

async function sendImg2ImgPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
    case ImageModelImg2Img:
        url = '/images/generations/lcm';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    if (chatVisionSelectedFileStore.length === 0) {
        throw new Error('no image selected');
    }
    const imageBase64 = chatVisionSelectedFileStore[0].contentB64;

    // insert image to user input & hisotry
    await appendImg2UserInput(chatID, imageBase64, `${libs.DateStr()}.png`);

    chatVisionSelectedFileStore = [];
    updateChatVisionSelectedFileStore();

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
            prompt,
            image_base64: imageBase64
        })
    });
    if (!resp.ok || resp.status !== 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.model = selectedModel;
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // save img to storage no matter it's done or not
    let attachHTML = '';
    respData.image_urls.forEach((url) => {
        attachHTML += `<img src="${url}">`;
    })

    await appendChats2Storage({
        role: RoleAI,
        chatID,
        model: selectedModel,
        content: attachHTML
    });
}

/**
 * Sends a prompt to the server for the selected model and updates the current AI response element with the task information.
 *
 * @param {string} model - The selected model.
 * @param {string} prompt - The prompt to send to the server.
 * @returns chat/complete/image/qa/unknown
 */
async function detectPromptTaskType (model, prompt) {
    if (IsChatModel(model)) {
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
                        model: ChatModelTurbo35,
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
                console.warn(`failed to request llm to detect prompt task type: ${err}`)
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
    await appendChats2Storage({
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

async function sendChat2Server (chatID) {
    let selectedModel = await OpenaiSelectedModel();
    let reqPrompt;
    if (!chatID) { // if chatID is empty, it's a new request
        chatID = newChatID();
        reqPrompt = libs.TrimSpace(chatPromptInputEle.value || '');

        chatPromptInputEle.value = '';
        // if prompt is empty, just ignore it.
        //
        // it is unable to just send image without text,
        // because claude will return error if prompt is empty.
        if (reqPrompt === '') {
            return;
        }

        append2Chats({
            chatID,
            role: RoleHuman,
            content: reqPrompt,
            model: selectedModel,
            isHistory: false
        });
        await appendChats2Storage({
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
    globalAIRespEle.dataset.aiRawResp = '';
    globalAIRespEle.dataset.respExtras = '';
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
        if (chatVisionSelectedFileStore.length !== 0) {
            if (!VisionModels.includes(selectedModel)) {
                // if selected model is not vision model, just ignore it
                chatVisionSelectedFileStore = [];
                updateChatVisionSelectedFileStore();
                return;
            }

            messages[messages.length - 1].files = [];
            for (const item of chatVisionSelectedFileStore) {
                messages[messages.length - 1].files.push({
                    type: 'image',
                    name: item.filename,
                    content: item.contentB64
                });

                // insert image to user input & hisotry
                await appendImg2UserInput(chatID, item.contentB64, item.filename);
            }

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
                return;
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
                return;
            }

            globalAIRespEle.dataset.respExtras = encodeURIComponent(`
                    <p style="margin-bottom: 0;">
                        <button class="btn btn-info" type="button" data-bs-toggle="collapse" data-bs-target="#chatRef-${chatID}" aria-expanded="false" aria-controls="chatRef-${chatID}" style="font-size: 0.6em">
                            > toggle reference
                        </button>
                    </p>`);

            if (data.url) {
                globalAIRespEle.dataset.respExtras += encodeURIComponent(`
                    <div>
                        <div class="collapse" id="chatRef-${chatID}">
                            <div class="card card-body">${combineRefs(data.url)}</div>
                        </div>
                    </div>`);
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
            const model = ChatModelTurbo35; // rewrite chat model

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
            await abortAIResp(err);
            return;
        }

        break;
    case 'image':
        if (!IsImageModel(selectedModel) && sconfig.chat_switch.all_in_one) {
            selectedModel = ImageModelDalle3;
        }

        try {
            switch (selectedModel) {
            case ImageModelDalle3:
                await sendTxt2ImagePrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            case ImageModelImg2Img:
                await sendImg2ImgPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            case ImageModelSdxlTurbo:
                await sendSdxlturboPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            default:
                throw new Error(`unknown image model: ${selectedModel}`);
            }
        } catch (err) {
            await abortAIResp(err);
        } finally {
            unlockChatInput();
        }

        return;
    default:
        globalAIRespEle.innerHTML = '<p>🔥Someting in trouble...</p>' +
                '<pre style="background-color: #f8e8e8; text-wrap: pretty;">' +
                `unimplemented model: ${libs.sanitizeHTML(selectedModel)}</pre>`;
        await appendChats2Storage({
            role: RoleAI,
            chatID,
            model: selectedModel,
            content: globalAIRespEle.innerHTML
        });
        unlockChatInput();
        return;
    }

    if (!reqBody) {
        return;
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

    globalAIRespSSE.addEventListener('message', async (evt) => {
        evt.stopPropagation();
        globalAIRespHeartBeatTimer = Date.now();

        // set request id to ai response element
        if (!globalAIRespEle.dataset.reqeustid) {
            const ids = evt.headers['x-oneapi-request-id'] || [];
            if (ids.length > 0) {
                globalAIRespEle.dataset.reqeustid = ids[0];
            }
        }

        let isChatRespDone = false;
        if (evt.data === '[DONE]') {
            isChatRespDone = true;
        } else if (evt.data === '[HEARTBEAT]') {
            return;
        }

        // remove prefix [HEARTBEAT]
        evt.data = evt.data.replace(/^\[HEARTBEAT\]+/, '');

        if (!isChatRespDone) {
            try {
                const payload = JSON.parse(evt.data);
                const respContent = parseChatResp(selectedModel, payload);

                if (payload.choices[0].finish_reason) {
                    isChatRespDone = true;
                }

                switch (globalAIRespEle.dataset.status) {
                case 'waiting':
                    globalAIRespEle.dataset.status = 'writing';

                    if (respContent) {
                        globalAIRespEle.innerHTML = respContent;
                        globalAIRespEle.dataset.aiRawResp += encodeURIComponent(respContent);
                    } else {
                        globalAIRespEle.innerHTML = '';
                    }

                    break;
                case 'writing':
                    if (respContent) {
                        globalAIRespEle.dataset.aiRawResp += encodeURIComponent(respContent);
                        globalAIRespEle.innerHTML = await libs.Markdown2HTML(decodeURIComponent(globalAIRespEle.dataset.aiRawResp));
                    }

                    scrollToChat(globalAIRespEle);
                    break;
                }
            } catch (err) {
                await abortAIResp(err);
            }
        }

        if (isChatRespDone) {
            if (globalAIRespSSE) {
                globalAIRespSSE.close();
                globalAIRespSSE = null;
                unlockChatInput();
            }

            console.debug(`chat response done for chat ${chatID}`);
            await renderAfterAiResp(chatID, true);
        }
    })

    globalAIRespSSE.onerror = async (err) => {
        await abortAIResp(err);
    };
    globalAIRespSSE.stream();
}

/**
 * do render and save chat after ai response finished
 */
async function renderAfterAiResp (chatID, saveStorage = false) {
    const aiRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    if (!aiRespEle) {
        console.warn(`can not find ai-response element for chatid=${chatID}`);
        return;
    }

    const aiRawResp = decodeURIComponent(aiRespEle.dataset.aiRawResp || '');
    const respExtras = decodeURIComponent(aiRespEle.dataset.respExtras || '');
    if (aiRawResp && aiRawResp !== 'undefined') {
        aiRespEle.innerHTML = await libs.Markdown2HTML(aiRawResp);
        aiRespEle.innerHTML += respExtras;
    }

    // setup prism
    {
        // add line number
        aiRespEle.querySelectorAll('pre').forEach((item) => {
            item.classList.add('line-numbers');
        });
    }

    // should save html before prism formatted,
    // because prism.js do not support formatted html.
    const markdownContent = aiRespEle.innerHTML;

    try {
        window.mermaid && await window.mermaid.run({ querySelector: 'pre.mermaid' });
    } catch (err) {
        console.error('mermaid run error:', err);
    }

    aiRespEle.insertAdjacentHTML('beforeend', `<div class="info"><i class="model">${aiRespEle.dataset.model || ''}</i></div>`);

    // add cost tips
    let costUsd = aiRespEle.dataset.costUsd;
    const sconfig = await getChatSessionConfig();
    if (sconfig.api_token.startsWith('laisky-') ||
        sconfig.api_token.startsWith('FREETIER-')) {
        if (!costUsd && aiRespEle.dataset.reqeustid) {
            // do not block the main thread
            const resp = await fetch(`/oneapi/api/cost/request/${aiRespEle.dataset.reqeustid}`)
            if (resp.ok) {
                costUsd = (await resp.json()).cost_usd;
            }

            if (costUsd) {
                aiRespEle.dataset.costUsd = costUsd;
                aiRespEle.querySelector('div.info')
                    .insertAdjacentHTML('beforeend', `<i class="cost">$${costUsd}</i>`);
            }
        } else if (costUsd) {
            aiRespEle.dataset.costUsd = costUsd;
            aiRespEle.querySelector('div.info')
                .insertAdjacentHTML('beforeend', `<i class="cost">$${costUsd}</i>`);
        }
    }

    window.Prism.highlightAllUnder(aiRespEle);
    libs.EnableTooltipsEverywhere();

    // in the scenario of reload chat, the chatEle is already in view,
    // no need to scroll and save to storage
    if (saveStorage) {
        scrollToChat(aiRespEle);
        await appendChats2Storage({
            role: RoleAI,
            chatID,
            costUsd,
            model: aiRespEle.dataset.model,
            content: markdownContent,
            attachHTML: respExtras,
            rawContent: aiRawResp
        });
    }

    bindImageOperationInAiResp(chatID);
    addOperateBtnBelowAiResponse(chatID);
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
            evt = libs.evtTarget(evt);

            // read image data to base64 encoded str
            const imgUrl = evt.closest('.ai-resp-image').querySelector('img').src;
            showImageEditModal(chatID, imgUrl);
        });
    }
}

/**
 * add operate buttons to ai response element after ai response finished
 *
 * @param {*} chatID - chat id
 */
function addOperateBtnBelowAiResponse (chatID) {
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

    // add copy button
    divContainer.insertAdjacentHTML('beforeend', `
        <button type="button" class="btn btn-success" data-bs-toggle="tooltip" data-bs-placement="top" title="copy raw" data-fn="copy">
            <i class="bi bi-copy"></i>
        </button>
    `)
    divContainer.querySelector('button[data-fn="copy"]')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            evt = libs.evtTarget(evt);

            // aiRespEle.dataset.copyBinded = true;
            let copyContent = '';
            if (!evt.closest('.ai-response') || !evt.closest('.ai-response').dataset.aiRawResp) {
                console.warn(`can not find ai response or ai raw response for copy, chatid=${chatID}`);
            } else {
                copyContent = decodeURIComponent(evt.closest('.ai-response').dataset.aiRawResp);
            }

            libs.Copy2Clipboard(copyContent);
        });

    // add reload button
    divContainer.insertAdjacentHTML('beforeend', `
        <button type="button" class="btn btn-secondary" data-bs-toggle="tooltip" data-bs-placement="top" aria-label="reload" data-bs-original-title="reload" data-fn="reload">
            <i class="bi bi-arrow-clockwise" data-fn="reload"></i>
        </button>
    `)
    divContainer.querySelector('button[data-fn="reload"]')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            // hide tooltip manually
            libs.DisableTooltipsEverywhere();

            const chatID = evt.target.closest('.role-ai').dataset.chatid;
            // put image back to vision store
            putBackAttachmentsInUserInput(chatID);

            await reloadAiResp(chatID);
        });

    libs.EnableTooltipsEverywhere();
}

function combineRefs (arr) {
    let markdown = '';
    for (const val of arr) {
        if (val.startsWith('https') || val.startsWith('http')) {
            // markdown += `- <${val}>\n`;
            markdown += `<li><a href="${val}">${decodeURIComponent(val)}</li>`;
        } else { // sometimes refs are just plain text, not url
            // markdown += `- \`${val}\`\n`;
            markdown += `<li><p>${val}</p></li>`;
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

    console.error(`abort AI resp: ${err}`);
    if (globalAIRespSSE) {
        globalAIRespSSE.close();
        globalAIRespSSE = null;
        unlockChatInput();
    }

    if (!globalAIRespEle) {
        console.warn('globalAIRespEle is not set for abortAIResp');
        return;
    }

    const chatID = globalAIRespEle.closest('.role-ai').dataset.chatid;
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

    if (globalAIRespEle.dataset.status === 'waiting') {
        globalAIRespEle.dataset.aiRawResp = encodeURIComponent(`<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${libs.RenderStr2HTML(errMsg)}</pre>`);
    } else {
        globalAIRespEle.dataset.respExtras += encodeURIComponent(`<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${libs.RenderStr2HTML(errMsg)}</pre>`);
    }

    await renderAfterAiResp(chatID, true);
    // scrollToChat(globalAIRespEle);
    // await appendChats2Storage(RoleAI, chatID, globalAIRespEle.innerHTML);
}

async function bindUserInputSelectFilesBtn () {
    chatContainer.querySelector('.user-input .btn.upload')
        .addEventListener('click', async (evt) => {
            // click to select images
            evt.stopPropagation();

            const inputEle = document.createElement('input');
            inputEle.type = 'file';
            inputEle.multiple = true;
            inputEle.accept = 'image/*';

            inputEle.addEventListener('change', async (evt) => {
                const files = libs.evtTarget(evt).files;
                for (const file of files) {
                    readFileForVision(file);
                }
            });

            inputEle.click();
        });
}

/** auto display or hide user input select files button according to selected model
 *
 */
async function autoToggleUserImageUploadBtn () {
    const sconfig = await getChatSessionConfig();
    const isVision = VisionModels.includes(sconfig.selected_model);

    const btnEle = chatContainer.querySelector('.user-input .btn.upload');
    if ((isVision && btnEle) || (!isVision && !btnEle)) {
        // everything is ok
        return;
    }

    const uploadEleHtml = '<button class="btn btn-outline-secondary upload" type="button"><i class="bi bi-images"></i></button>';
    if (isVision) {
        chatPromptInputBtn.insertAdjacentHTML('beforebegin', uploadEleHtml);
        bindUserInputSelectFilesBtn();
    } else {
        btnEle.remove();
    }
}

async function setupChatInput () {
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
        libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
            if (op !== libs.KvOp.SET) {
                return;
            }

            const expectedKey = `KvKeyPrefixSessionConfig${(await activeSessionID())}`;
            if (key !== expectedKey) {
                return;
            }

            const sconfig = newVal;
            chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;
        });
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
    libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
        if (op !== libs.KvOp.SET) {
            return;
        }

        await autoToggleUserImageUploadBtn();
    });

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
                        console.error(`upload file failed: ${err}`);
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
        document.body.addEventListener('paste', filePasteHandler);

        dropfileModalEle.addEventListener('drop', fileDragDropHandler);
        dropfileModalEle.addEventListener('dragleave', fileDragLeave);
    }

    // bind chat switch
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
                    await await restorePinnedMaterials();
                }

                await saveChatSessionConfig(sconfig);
            });

        chatContainer
            .querySelector('#switchChatEnableGoogleSearch')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = libs.evtTarget(evt)
                const sconfig = await getChatSessionConfig()
                sconfig.chat_switch.enable_google_search = switchEle.checked
                await saveChatSessionConfig(sconfig)
            });

        chatContainer
            .querySelector('#switchChatEnableAllInOne')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = libs.evtTarget(evt)
                const sconfig = await getChatSessionConfig()
                sconfig.chat_switch.all_in_one = switchEle.checked
                await saveChatSessionConfig(sconfig)
            });

        chatContainer
            .querySelector('#switchChatEnableAutoSync')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = libs.evtTarget(evt)
                await libs.KvSet(KvKeyAutoSyncUserConfig, switchEle.checked);
            });
        let userConfigSyncer;
        libs.KvAddListener(KvKeyAutoSyncUserConfig, async (key, op, oldVal, newVal) => {
            if (op !== libs.KvOp.SET) {
                return;
            }

            // update ui
            const switchEle = chatContainer.querySelector('#switchChatEnableAutoSync');
            switchEle.checked = newVal;

            // update background syncer
            if (!newVal) {
                console.debug('stop user config syncer');
                if (userConfigSyncer) {
                    clearTimeout(userConfigSyncer);
                    userConfigSyncer = null;
                }

                return;
            }

            if (userConfigSyncer) {
                return;
            }

            console.debug('start user config syncer');
            // await syncUserConfig();
            userConfigSyncer = setTimeout(async () => {
                await syncUserConfig();
            }, 1800 * 1000);
        });

        // bind listener for all chat switchs
        libs.KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
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
        });
    }
}

// read paste file
async function filePasteHandler (evt) {
    if (!evt.clipboardData || !evt.clipboardData.items) {
        return;
    }

    for (let i = 0; i < evt.clipboardData.items.length; i++) {
        const item = evt.clipboardData.items[i];
        if (item.kind !== 'file') {
            continue;
        }

        const file = item.getAsFile();
        if (!file) {
            continue;
        }

        evt.stopPropagation();
        evt.preventDefault();

        // get file content as Blob
        readFileForVision(file, `paste-${libs.DateStr()}.png`);
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
        body: formData
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

    let newText = ''
    if (chatEle.querySelector('.role-human textarea')) {
        // read user input from click edit button
        newText = chatEle.querySelector('.role-human textarea').value;
    } else {
        // read user input from click reload button
        newText = chatEle.querySelector('.role-human .text-start pre').innerHTML;
    }

    chatEle.innerHTML = `
        <div class="container-fluid row role-human" data-chatid="${chatID}">
            <div class="col-auto icon">🤔️</div>
            <div class="col text-start"><pre>${newText}</pre></div>
            <div class="col-auto d-flex control">
                <i class="bi bi-pencil-square"></i>
                <i class="bi bi-trash"></i>
            </div>
        </div>
        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
            <div class="col-auto icon">${robotIcon}</div>
            <div class="col text-start ai-response" data-status="waiting">
                <p class="card-text placeholder-glow">
                    <span class="placeholder col-7"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-6"></span>
                    <span class="placeholder col-8"></span>
                </p>
            </div>
        </div>`;

    chatEle.querySelector('.role-ai').dataset.status = 'waiting';

    // bind delete and edit button
    chatEle.querySelector('.role-human .bi-trash')
        .addEventListener('click', deleteBtnHandler);
    chatEle.querySelector('.bi.bi-pencil-square')
        .addEventListener('click', editHumanInputHandler);

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
 */
function putBackAttachmentsInUserInput (chatID) {
    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);

    // attach image to vision-selected-store when edit human input
    const attachEles = chatEle
        .querySelectorAll('.role-human .text-start img') || [];
    attachEles.forEach((ele) => {
        const b64fileContent = ele.getAttribute('src').replace('data:image/png;base64,', '');
        const key = ele.dataset.name || `${libs.DateStr()}.png`;
        chatVisionSelectedFileStore.push({
            filename: key,
            contentB64: b64fileContent
        });
        // attachHTML += `<img src="data:image/png;base64,${b64fileContent}" data-name="${key}">`;
    })
    updateChatVisionSelectedFileStore();
}

/**
 * edit human input
 *
 * @param {Event} evt - event
 */
function editHumanInputHandler (evt) {
    evt.stopPropagation();
    const chatID = evt.target.closest('.role-human').dataset.chatid;

    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);

    const oldText = chatEle.innerHTML;
    let text = chatEle.querySelector('.role-human .text-start pre').innerHTML;
    // let attachHTML = '';

    putBackAttachmentsInUserInput(chatID);

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

    const saveBtn = chatEle.querySelector('.role-human .btn.save');
    const cancelBtn = chatEle.querySelector('.role-human .btn.cancel');
    saveBtn.addEventListener('click', async (evt) => {
        evt.stopPropagation();
        await reloadAiResp(chatID);
    });

    cancelBtn.addEventListener('click', async (evt) => {
        evt.stopPropagation();
        chatEle.innerHTML = oldText;

        // bind delete and edit button
        chatEle.querySelector('.role-human .bi-trash')
            .addEventListener('click', deleteBtnHandler);
        chatEle.querySelector('.bi.bi-pencil-square')
            .addEventListener('click', editHumanInputHandler);
    });
};

// bind delete button
const deleteBtnHandler = (evt) => {
    evt.stopPropagation();
    const chatID = evt.target.closest('.role-human').dataset.chatid;
    const chatEle = chatContainer.querySelector(`.chatManager .conservations .chats #${chatID}`);

    ConfirmModal('Are you sure to delete this chat?', async () => {
        chatEle.parentNode.removeChild(chatEle);
        removeChatInStorage(chatID);
    });
};

/**
 * Append chat to conservation container
 *
 * @param {Object} chatItem - chat item
 *   @property {string} chatID - chat id
 *   @property {string} role - RoleHuman/RoleSystem/RoleAI
 *   @property {string} content - chat content
 *   @property {boolean} isHistory - is history chat, default false. if true, will not append to storage
 *   @property {string} attachHTML - html to attach to chat
 *   @property {string} rawAiResp - raw ai response
 *   @property {string} costUsd - cost in usd
 *   @property {string} model - model name
 */
async function append2Chats (chatItem) {
    const chatID = chatItem.chatID;
    const role = chatItem.role;
    let content = chatItem.content;
    const isHistory = chatItem.isHistory || false;
    let attachHTML = chatItem.attachHTML || '';
    const rawAiResp = chatItem.rawAiResp || '';
    const costUsd = chatItem.costUsd || '';
    const model = chatItem.model || '';

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
                        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                            <div class="col-auto icon">${robotIcon}</div>
                            <div class="col text-start ai-response" data-status="waiting" data-model="${model}">
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
        chatEleHtml = `
                <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="col-auto icon">${robotIcon}</div>
                        <div class="col text-start ai-response" data-status="waiting" data-ai-raw-resp="${encodeURIComponent(rawAiResp)}" data-resp-extras="${encodeURIComponent(attachHTML)}" data-cost-usd="${costUsd}" data-model="${model}">
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
            .addEventListener('click', editHumanInputHandler);
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
        max_tokens: 500,
        temperature: 1,
        presence_penalty: 0,
        frequency_penalty: 0,
        n_contexts: 6,
        system_prompt: "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can.",
        selected_model: ChatModelTurbo35,
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
    chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;

    // update chat controller
    chatContainer.querySelector('#switchChatEnableHttpsCrawler')
        .checked = !sconfig.chat_switch.disable_https_crawler;
    chatContainer.querySelector('#switchChatEnableGoogleSearch')
        .checked = sconfig.chat_switch.enable_google_search;
    chatContainer.querySelector('#switchChatEnableAllInOne')
        .checked = sconfig.chat_switch.all_in_one;
    chatContainer.querySelector('#switchChatEnableAutoSync')
        .checked = await libs.KvGet(KvKeyAutoSyncUserConfig);

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
        configContainer.querySelector('.btn.clear-chats')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                ConfirmModal('Clear all chat records in the current session?', async () => {
                    clearSessionAndChats(evt, await activeSessionID());
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
        libs.KvAddListener(KvKeySyncKey, async (key, op, oldVal, newVal) => {
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
        configContainer.querySelector('.btn[data-app-fn="cloud-sync"]')
            .addEventListener('click', async (evt) => {
                try {
                    ShowSpinner();
                    await syncUserConfig(evt);
                    location.reload();
                } catch (err) {
                    console.error(err);
                    showalert('danger', `sync user config failed: ${err}`);
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

async function syncUserConfig (evt) {
    await downloadUserConfig(evt);
    await uploadUserConfig(evt);
}

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
            while (iLocal < localHistory.length || iRemote < data[key].length) {
                if (iLocal >= localHistory.length) {
                    localHistory = localHistory.concat(data[key].slice(iRemote));
                    break;
                }
                if (iRemote >= data[key].length) {
                    break;
                }

                localChatId = localHistory[iLocal].chatID;
                // latest version's chatid like: chat-1705899120122-NwN9sB
                if (!localChatId || !localChatId.match(/chat-\d+-\w+/)) {
                    iLocal++;
                    continue;
                }

                localChatNum = parseInt(localChatId.split('-')[1]);

                while (iRemote < data[key].length) {
                    remoteChatId = data[key][iRemote].chatID;
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

    const badgetEle = evt.target.closest('.badge');
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

        const promptInput = configContainer.querySelector('.system-prompt .input');
        const badgetEle = evt.target.closest('.badge');
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
                    switch (dataset.status) {
                    case 'done':
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
                    case 'processing':
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

                            if (!libs.evtTarget(evt).checked) {
                                // at least one chatbot should be selected
                                libs.evtTarget(evt).checked = true;
                                return;
                            } else {
                                // uncheck other chatbot
                                datasetListEle
                                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                                    .forEach((ele) => {
                                        if (ele !== libs.evtTarget(evt)) {
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
                new window.bootstrap.Dropdown(libs.evtTarget(evt).closest('.dropdown')).hide();

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
                new window.bootstrap.Dropdown(libs.evtTarget(evt).closest('.dropdown')).hide();

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
