'use strict'

const ChatModelTurbo35V1106 = 'gpt-3.5-turbo-1106'
// ChatModelTurbo35 = "gpt-3.5-turbo",
// ChatModelTurbo35_16K = "gpt-3.5-turbo-16k",
// ChatModelTurbo35_0613 = "gpt-3.5-turbo-0613",
// ChatModelTurbo35_0613_16K = "gpt-3.5-turbo-16k-0613",
// ChatModelGPT4 = "gpt-4",
const ChatModelGPT4Turbo = 'gpt-4-1106-preview'
const ChatModelGPT4Vision = 'gpt-4-vision-preview'
// ChatModelGPT4_0613 = "gpt-4-0613",
// ChatModelGPT4_32K = "gpt-4-32k",
// ChatModelGPT4_0613_32K = "gpt-4-32k-0613",
const ChatModelGeminiPro = 'gemini-pro'
const ChatModelGeminiProVision = 'gemini-pro-vision'
const QAModelBasebit = 'qa-bbt-xego'
const QAModelSecurity = 'qa-security'
const QAModelImmigrate = 'qa-immigrate'
const QAModelCustom = 'qa-custom'
const QAModelShared = 'qa-shared'
const CompletionModelDavinci3 = 'text-davinci-003'
const ImageModelDalle2 = 'dall-e-3'
const ImageModelSdxlTurbo = 'sdxl-turbo'
const ImageModelImg2Img = 'img-to-img'

// casual chat models

const ChatModels = [
    // ChatModelTurbo35,
    ChatModelTurbo35V1106,
    // ChatModelGPT4,
    ChatModelGPT4Turbo,
    ChatModelGPT4Vision,
    ChatModelGeminiPro,
    ChatModelGeminiProVision
    // ChatModelTurbo35_16K,
    // ChatModelTurbo35_0613,
    // ChatModelTurbo35_0613_16K,
    // ChatModelGPT4_0613,
    // ChatModelGPT4_32K,
    // ChatModelGPT4_0613_32K,
]
const QaModels = [
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    QAModelCustom,
    QAModelShared
]
const ImageModels = [
    ImageModelDalle2,
    ImageModelSdxlTurbo,
    ImageModelImg2Img
]
const CompletionModels = [
    CompletionModelDavinci3
]
const FreeModels = [
    // ChatModelTurbo35,
    ChatModelTurbo35V1106,
    ChatModelGeminiPro,
    ChatModelGeminiProVision,
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    ImageModelSdxlTurbo,
    ImageModelImg2Img
]
const AllModels = [].concat(ChatModels, QaModels, ImageModels, CompletionModels)

// custom dataset's end-to-end password
const KvKeyPinnedMaterials = 'config_api_pinned_materials'
const KvKeyAllowedModels = 'config_chat_models'
const KvKeyCustomDatasetPassword = 'config_chat_dataset_key'
const KvKeyPromptShortCuts = 'config_prompt_shortcuts'
const KvKeyPrefixSessionHistory = 'chat_user_session_'
const KvKeyPrefixSessionConfig = 'chat_user_config_'
const KvKeyPrefixSelectedSession = 'config_selected_session'

const IsChatModel = (model) => {
    return ChatModels.includes(model)
}

const IsQaModel = (model) => {
    return QaModels.includes(model)
}

const IsCompletionModel = (model) => {
    return CompletionModels.includes(model)
}

const IsImageModel = (model) => {
    return ImageModels.includes(model)
}

const IsChatModelAllowed = async (model) => {
    const allowedModels = await KvGet(KvKeyAllowedModels)
    if (!allowedModels) {
        return false
    }

    return allowedModels.includes(model)
}

const ShowSpinner = () => {
    document.getElementById('spinner').toggleAttribute('hidden', false)
}
const HideSpinner = () => {
    document.getElementById('spinner').toggleAttribute('hidden', true)
}

/**
 * Generates a random string of the specified length.
 * @param {number} length - The length of the string to generate.
 * @returns {string} - The generated random string.
 */
const RandomString = (length) => {
    const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
    let result = ''
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length))
    }

    return result
}

const OpenaiToken = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    let apikey

    // get token from url params first
    {
        apikey = new URLSearchParams(location.search).get('apikey')

        if (apikey) {
            // fix: sometimes url.searchParams.delete() works too quickly,
            // that let another caller rewrite apikey to FREE-TIER,
            // so we delay 1s to delete apikey from url params.
            setTimeout(() => {
                const v = new URLSearchParams(location.search).get('apikey')
                if (!v) {
                    return
                }

                // remove apikey from url params
                const url = new URL(location.href)
                url.searchParams.delete('apikey')
                window.history.pushState({}, document.title, url)
            }, 500)
        }
    }

    // get token from storage
    if (!apikey) {
        apikey = sconfig.api_token || 'FREETIER-' + RandomString(32)
    }

    sconfig.api_token = apikey
    await KvSet(skey, sconfig)
    return apikey
}

const OpenaiApiBase = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.api_base || 'https://api.openai.com'
}

const OpenaiSelectedModel = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.selected_model || ChatModelTurbo35V1106
}

const OpenaiMaxTokens = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.max_tokens || 500
}

const OpenaiTemperature = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.temperature
}

const OpenaiPresencePenalty = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.presence_penalty || 0
}

const OpenaiFrequencyPenalty = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.frequency_penalty || 0
}

const ChatNContexts = async () => {
    const sid = await activeSessionID()
    const skey = `${KvKeyPrefixSessionConfig}${sid}`
    const sconfig = await KvGet(skey)
    return sconfig.n_contexts || 6
}

/** get or set chat static context
 *
 * @param {string} prompt
 * @returns {string} prompt
 */
const OpenaiChatStaticContext = async (prompt) => {
    const sid = await activeSessionID();
    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    const sconfig = await KvGet(skey);

    if (prompt) {
        sconfig.system_prompt = prompt;
        await KvSet(skey, sconfig);
    }

    return sconfig.system_prompt || '';
}

const SingleInputModal = (title, message, callback, defaultVal) => {
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
const ConfirmModal = (title, callback) => {
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

window.AppEntrypoint = async () => {
    await dataMigrate();
    await setupHeader();
    setupConfirmModal();
    setupSingleInputModal();

    await setupChatJs();
};

async function dataMigrate() {
    const sid = await activeSessionID();
    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    let sconfig = await KvGet(skey);

    // set selected session
    if (!KvGet(KvKeyPrefixSelectedSession)) {
        KvSet(KvKeyPrefixSelectedSession, sid);
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
                const val = GetLocalStorage(oldKey);
                if (!val) {
                    return;
                }

                const newKey = storageVals[oldKey];
                await KvSet(newKey, val);
                localStorage.removeItem(oldKey);
            }));

        // move session config
        if (!sconfig) {
            sconfig = newSessionConfig();

            sconfig.api_token = GetLocalStorage('config_api_token_value') || sconfig.api_token;
            sconfig.token_type = GetLocalStorage('config_api_token_type') || sconfig.token_type;
            sconfig.max_tokens = GetLocalStorage('config_api_max_tokens') || sconfig.max_tokens;
            sconfig.temperature = GetLocalStorage('config_api_temperature') || sconfig.temperature;
            sconfig.presence_penalty = GetLocalStorage('config_api_presence_penalty') || sconfig.presence_penalty;
            sconfig.frequency_penalty = GetLocalStorage('config_api_frequency_penalty') || sconfig.frequency_penalty;
            sconfig.n_contexts = GetLocalStorage('config_api_n_contexts') || sconfig.n_contexts;
            sconfig.system_prompt = GetLocalStorage('config_api_static_context') || sconfig.system_prompt;
            sconfig.selected_model = GetLocalStorage('config_chat_model') || sconfig.selected_model;

            await KvSet(skey, sconfig);
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
            await KvSet(skey, sconfig);
        }
    }

    // list all session configs
    await Promise.all((await KvList()).map(async (key) => {
        if (!key.startsWith(KvKeyPrefixSessionConfig)) {
            return;
        }

        const sconfig = await KvGet(key);

        // set default api_token
        if (!sconfig.api_token || sconfig.api_token == 'DEFAULT_PROXY_TOKEN') {
            sconfig.api_token = 'FREETIER-' + RandomString(32);
        }
        // set default api_base
        if (!sconfig.api_base) {
            sconfig.api_base = 'https://api.openai.com';
        }

        // set default chat controller
        if (!sconfig.chat_switch) {
            sconfig.chat_switch = {
                disable_https_crawler: false
            }
        }

        console.debug('migrate session config: ', key, sconfig);
        await KvSet(key, sconfig);
    }))

    // update legacy chat history, add chatID to each chat
    {
        await Promise.all(Object.keys(localStorage).map(async (key) => {
            if (!key.startsWith(KvKeyPrefixSessionHistory)) {
                return
            }

            // move from localstorage to kv
            // console.log("move from localstorage to kv: ", key);
            await KvSet(key, JSON.parse(localStorage[key]))
            localStorage.removeItem(key)
        }))
    }
}

let singleInputCallback, singleInputModal

function setupSingleInputModal() {
    singleInputCallback = null
    singleInputModal = new bootstrap.Modal(document.getElementById('singleInputModal'))
    document.getElementById('singleInputModal')
        .querySelector('.modal-body .yes')
        .addEventListener('click', async (e) => {
            e.preventDefault()

            if (singleInputCallback) {
                await singleInputCallback()
            }

            singleInputModal.hide()
        })
}

/**
 * setup confirm modal callback, shoule be an async function
 */
let deleteCheckCallback,
    /**
     * global shared modal to act as confirm dialog
     */
    deleteCheckModal

function setupConfirmModal() {
    deleteCheckModal = new bootstrap.Modal(document.getElementById('deleteCheckModal'))
    document.getElementById('deleteCheckModal')
        .querySelector('.modal-body .yes')
        .addEventListener('click', async (e) => {
            e.preventDefault()

            if (deleteCheckCallback) {
                await deleteCheckCallback()
            }

            deleteCheckModal.hide()
        })
}

/** setup header bar
 *
 */
async function setupHeader() {
    const headerBarEle = document.getElementById('headerbar')
    let allowedModels = []
    const sconfig = await getChatSessionConfig()

    // setup chat models
    {
        // set default chat model
        let selectedModel = await OpenaiSelectedModel()

        // get users' models
        const headers = new Headers()
        headers.append('Authorization', 'Bearer ' + sconfig.api_token)
        const response = await fetch('/user/me', {
            method: 'GET',
            cache: 'no-cache',
            headers
        })

        if (response.status != 200) {
            throw new Error('failed to get user info, please refresh your browser.')
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
        await KvSet(KvKeyAllowedModels, respData.allowed_models);
        allowedModels = respData.allowed_models;

        if (!allowedModels.includes(selectedModel)) {
            selectedModel = '';
            AllModels.forEach((model) => {
                if (selectedModel != '' || !allowedModels.includes(model)) {
                    return;
                }

                if (model.startsWith('gpt-') || model.startsWith('gemini-')) {
                    selectedModel = model;
                }
            });

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);
            sconfig.selected_model = selectedModel;
            await KvSet(skey, sconfig);
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

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);
            sconfig.selected_model = selectedModel;
            await KvSet(skey, sconfig);

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
