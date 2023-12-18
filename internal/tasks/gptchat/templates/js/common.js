"use strict";

const OpenaiTokenTypeProxy = "proxy",
    OpenaiTokenTypeDirect = "direct";

const ChatModelTurbo35 = "gpt-3.5-turbo",
    ChatModelTurbo35_16K = "gpt-3.5-turbo-16k",
    // ChatModelTurbo35_0613 = "gpt-3.5-turbo-0613",
    // ChatModelTurbo35_0613_16K = "gpt-3.5-turbo-16k-0613",
    // ChatModelGPT4 = "gpt-4",
    ChatModelGPT4Turbo = "gpt-4-1106-preview",
    ChatModelGPT4Vision = "gpt-4-vision-preview",
    // ChatModelGPT4_0613 = "gpt-4-0613",
    ChatModelGPT4_32K = "gpt-4-32k",
    // ChatModelGPT4_0613_32K = "gpt-4-32k-0613",
    ChatModelGeminiPro = "gemini-pro",
    QAModelBasebit = "qa-bbt-xego",
    QAModelSecurity = "qa-security",
    QAModelImmigrate = "qa-immigrate",
    QAModelCustom = "qa-custom",
    QAModelShared = "qa-shared",
    CompletionModelDavinci3 = "text-davinci-003",
    ImageModelDalle2 = "dall-e-3",
    ImageModelSdxlTurbo = "sdxl-turbo",
    ImageModelImg2Img = "img-to-img";

// casual chat models

const ChatModels = [
    ChatModelTurbo35,
    // ChatModelGPT4,
    ChatModelGPT4Turbo,
    ChatModelGPT4Vision,
    ChatModelGeminiPro,
    CompletionModelDavinci3,
    ChatModelTurbo35_16K,
    // ChatModelTurbo35_0613,
    // ChatModelTurbo35_0613_16K,
    // ChatModelGPT4_0613,
    ChatModelGPT4_32K,
    // ChatModelGPT4_0613_32K,
],
    QaModels = [
        QAModelBasebit,
        QAModelSecurity,
        QAModelImmigrate,
        QAModelCustom,
        QAModelShared,
    ],
    ImageModels = [
        ImageModelDalle2,
        ImageModelSdxlTurbo,
        ImageModelImg2Img,
    ],
    CompletionModels = [
        CompletionModelDavinci3,
    ],
    AllModels = [].concat(ChatModels, QaModels, ImageModels, CompletionModels);

const StorageKeyPromptShortCuts = "config_prompt_shortcuts",
    // custom dataset's end-to-end password
    StorageKeyCustomDatasetPassword = "config_chat_dataset_key",
    StorageKeyPinnedMaterials = "config_api_pinned_materials",
    StorageKeyAllowedModels = "config_chat_models";

// should not has same prefix
const KvKeyPrefixSessionHistory = "chat_user_session_",
    KvKeyPrefixSessionConfig = "chat_user_config_";

var IsChatModel = (model) => {
    return ChatModels.includes(model);
};

var IsQaModel = (model) => {
    return QaModels.includes(model);
};

var IsCompletionModel = (model) => {
    return CompletionModels.includes(model);
};

var IsImageModel = (model) => {
    return ImageModels.includes(model);
};

var IsChatModelAllowed = (model) => {
    let allowed_models = GetLocalStorage(StorageKeyAllowedModels);
    if (!allowed_models) {
        return false;
    }

    return allowed_models.includes(model);
}

var ShowSpinner = () => {
    document.getElementById("spinner").toggleAttribute("hidden", false);
};
var HideSpinner = () => {
    document.getElementById("spinner").toggleAttribute("hidden", true);
};


var OpenaiAPI = async () => {
    switch (await OpenaiTokenType()) {
        case OpenaiTokenTypeProxy:
            return data.openai.proxy;
        case OpenaiTokenTypeDirect:
            return data.openai.direct;
    }
};

var OpenaiUserIdentify = async () => {
    t = (await OpenaiToken());
    return t;
};

var OpenaiTokenType = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["token_type"];
};

/**
 * Generates a random string of the specified length.
 * @param {number} length - The length of the string to generate.
 * @returns {string} - The generated random string.
 */
var RandomString = (length) => {
    const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length));
    }

    return result;
}

var OpenaiToken = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey),
        apikey;

    // get token from url params first
    {
        apikey = new URLSearchParams(location.search).get("apikey");

        if (apikey) {
            // fix: sometimes url.searchParams.delete() works too quickly,
            // that let another caller rewrite apikey to FREE-TIER,
            // so we delay 1s to delete apikey from url params.
            setTimeout(() => {
                let v = new URLSearchParams(location.search).get("apikey");
                if (!v) {
                    return;
                }

                // remove apikey from url params
                let url = new URL(location.href);
                url.searchParams.delete("apikey");
                window.history.pushState({}, document.title, url);
            }, 500);
        }
    }

    // get token from storage
    if (!apikey) {
        apikey = sconfig["api_token"] || "FREETIER-" + RandomString(32);
    }

    sconfig["api_token"] = apikey;
    await KvSet(skey, sconfig);
    return apikey;
};

var OpenaiApiBase = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["api_base"] || "https://api.openai.com";
};

var OpenaiSelectedModel = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["selected_model"] || ChatModelTurbo35;
}

var OpenaiMaxTokens = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["max_tokens"] || 500;
};

var OpenaiTemperature = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["temperature"];
};

var OpenaiPresencePenalty = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["presence_penalty"] || 0;
};

var OpenaiFrequencyPenalty = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["frequency_penalty"] || 0;
};

var ChatNContexts = async () => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);
    return sconfig["n_contexts"] || 3;
};

/** get or set chat static context
 *
 * @param {string} prompt
 * @returns {string} prompt
 */
var OpenaiChatStaticContext = async (prompt) => {
    let sid = activeSessionID(),
        skey = `${KvKeyPrefixSessionConfig}${sid}`,
        sconfig = await KvGet(skey);

    if (prompt) {
        sconfig["system_prompt"] = prompt;
        await KvSet(skey, sconfig);
    }

    return sconfig["system_prompt"] || "";
};


var SingleInputModal = (title, message, callback) => {
    const modal = document.getElementById("singleInputModal");
    singleInputCallback = async () => {
        try {
            ShowSpinner();
            await callback(modal.querySelector(".modal-body input").value)
        } finally {
            HideSpinner();
        }
    };

    modal.querySelector(".modal-title").innerHTML = title;
    modal.querySelector(".modal-body label.form-label").innerHTML = message;
    singleInputModal.show();
};

// show modal to confirm,
// callback will be called if user click yes
//
// params:
//   - title: modal title
//   - callback: async callback function
var ConfirmModal = (title, callback) => {
    deleteCheckCallback = async () => {
        try {
            ShowSpinner();
            await callback()
        } finally {
            HideSpinner();
        }
    };
    document.getElementById("deleteCheckModal").querySelector(".modal-title").innerHTML = title;
    deleteCheckModal.show();
};


window.AppEntrypoint = async () => {
    await dataMigrate();
    (await OpenaiToken());
    await setupHeader();
    setupConfirmModal();
    setupSingleInputModal();

    await setupChatJs();
};

async function dataMigrate() {
    // set openai token
    {
        let sconfig = await KvGet(`${KvKeyPrefixSessionConfig}${activeSessionID()}`) || newSessionConfig();
        if (!sconfig["api_token"] || sconfig["api_token"] == "DEFAULT_PROXY_TOKEN") {
            sconfig["api_token"] = "FREETIER-" + RandomString(32);
            await KvSet(`${KvKeyPrefixSessionConfig}${activeSessionID()}`, sconfig);
        }
    }

    // update legacy chat history, add chatID to each chat
    {
        await Promise.all(Object.keys(localStorage).map(async (key) => {
            if (!key.startsWith(KvKeyPrefixSessionHistory)) {
                return;
            }

            // move from localstorage to kv
            // console.log("move from localstorage to kv: ", key);
            await KvSet(key, JSON.parse(localStorage[key]));
            localStorage.removeItem(key);
        }));
    }

    // move config from localstorage to session config
    {
        let sid = activeSessionID(),
            skey = `${KvKeyPrefixSessionConfig}${sid}`,
            sconfig = await KvGet(skey);

        if (!sconfig) {
            sconfig = newSessionConfig();

            sconfig["api_token"] = GetLocalStorage("config_api_token_value") || sconfig["api_token"];
            sconfig["token_type"] = GetLocalStorage("config_api_token_type") || sconfig["token_type"];
            sconfig["max_tokens"] = GetLocalStorage("config_api_max_tokens") || sconfig["max_tokens"];
            sconfig["temperature"] = GetLocalStorage("config_api_temperature") || sconfig["temperature"];
            sconfig["presence_penalty"] = GetLocalStorage("config_api_presence_penalty") || sconfig["presence_penalty"];
            sconfig["frequency_penalty"] = GetLocalStorage("config_api_frequency_penalty") || sconfig["frequency_penalty"];
            sconfig["n_contexts"] = GetLocalStorage("config_api_n_contexts") || sconfig["n_contexts"];
            sconfig["system_prompt"] = GetLocalStorage("config_api_static_context") || sconfig["system_prompt"];
            sconfig["selected_model"] = GetLocalStorage("config_chat_model") || sconfig["selected_model"];

            await KvSet(skey, sconfig);
        }
    }
}

var singleInputCallback, singleInputModal;

function setupSingleInputModal() {
    singleInputCallback = null;
    singleInputModal = new bootstrap.Modal(document.getElementById("singleInputModal"));
    document.getElementById("singleInputModal")
        .querySelector(".modal-body .yes")
        .addEventListener("click", async (e) => {
            e.preventDefault();

            if (singleInputCallback) {
                await singleInputCallback();
            }

            singleInputModal.hide();
        });
}

/**
 * setup confirm modal callback, shoule be an async function
 */
var deleteCheckCallback,
    /**
     * global shared modal to act as confirm dialog
     */
    deleteCheckModal;

function setupConfirmModal() {
    deleteCheckModal = new bootstrap.Modal(document.getElementById("deleteCheckModal"));
    document.getElementById("deleteCheckModal")
        .querySelector(".modal-body .yes")
        .addEventListener("click", async (e) => {
            e.preventDefault();

            if (deleteCheckCallback) {
                await deleteCheckCallback();
            }

            deleteCheckModal.hide();
        });
}


/** setup header bar
 *
 */
async function setupHeader() {
    let headerBarEle = document.getElementById("headerbar"),
        allowedModels = [];

    // setup chat models
    {
        // set default chat model
        let selectedModel = await OpenaiSelectedModel();

        // get users' models
        let headers = new Headers();
        headers.append("Authorization", "Bearer " + (await OpenaiToken()));
        const response = await fetch("/user/me", {
            method: "GET",
            cache: "no-cache",
            headers: headers,
        });

        if (response.status != 200) {
            throw new Error("failed to get user info, please refresh your browser.");
        }

        let modelsContainer = document.querySelector("#headerbar .chat-models");
        const data = await response.json()
        let modelsEle = "";
        if (data.allowed_models.includes("*")) {
            data.allowed_models = AllModels;
        }

        SetLocalStorage(StorageKeyAllowedModels, data.allowed_models);
        allowedModels = data.allowed_models;

        if (!data.allowed_models.includes(selectedModel)) {
            selectedModel = data.allowed_models[0];

            let sid = activeSessionID(),
                skey = `${KvKeyPrefixSessionConfig}${sid}`,
                sconfig = await KvGet(skey);
            sconfig["selected_model"] = selectedModel;
            await KvSet(skey, sconfig);
        }

        // add hint to input text
        chatPromptInputEle.attributes
            .placeholder.value = `[${selectedModel}] CTRL+Enter to send`;

        let unsupportedModels = [];
        data.allowed_models.forEach((model) => {
            if (!ChatModels.includes(model)) {
                unsupportedModels.push(model);
                return;
            }

            modelsEle += `<li><a class="dropdown-item" href="#" data-model="${model}">${model}</a></li>`;
        });
        modelsContainer.innerHTML = modelsEle;

        // FIXME
        // if (unsupportedModels.length > 0) {
        //     showalert("warning", `there are some models enabled for your account, but not supported in the frontend, `
        //         + `maybe you need refresh your browser. if this warning still exists, `
        //         + `please contact us via <a href="mailto:chat-support@laisky.com">chat-support@laisky.com</a>. unsupported models: ${unsupportedModels.join(", ")}`);
        // }

        // setup chat qa models
        {
            let qaModelsContainer = headerBarEle.querySelector(".dropdown-menu.qa-models");
            allowedModels.forEach((model) => {
                if (!QaModels.includes(model)) {
                    return;
                }

                let li = document.createElement("li");
                let a = document.createElement("a");
                a.href = "#";
                a.classList.add("dropdown-item");
                a.dataset.model = model;
                a.textContent = model;
                li.appendChild(a);
                qaModelsContainer.appendChild(li);
            });
        }

        // setup chat image models
        {
            let imageModelsContainer = headerBarEle.querySelector(".dropdown-menu.image-models");
            imageModelsContainer.innerHTML = "";
            allowedModels.forEach((model) => {
                if (!ImageModels.includes(model)) {
                    return;
                }

                let li = document.createElement("li");
                let a = document.createElement("a");
                a.href = "#";
                a.classList.add("dropdown-item");
                a.dataset.model = model;
                a.textContent = model;
                li.appendChild(a);
                imageModelsContainer.appendChild(li);
            });
        }

        // listen click events
        let modelElems = document
            .querySelectorAll("#headerbar .chat-models li a, "
                + "#headerbar .qa-models li a, "
                + "#headerbar .image-models li a"
            );
        modelElems.forEach((elem) => {
            elem.addEventListener("click", async (evt) => {
                evt.preventDefault();
                modelElems.forEach((elem) => {
                    elem.classList.remove("active");
                });

                evt.target.classList.add("active");
                let selectedModel = evt.target.dataset.model;

                let sid = activeSessionID(),
                    skey = `${KvKeyPrefixSessionConfig}${sid}`,
                    sconfig = await KvGet(skey);
                sconfig["selected_model"] = selectedModel;
                await KvSet(skey, sconfig);

                // add active to class
                document.querySelectorAll("#headerbar .navbar-nav a.dropdown-toggle")
                    .forEach((elem) => {
                        elem.classList.remove("active");
                    });
                evt.target.closest(".dropdown").querySelector("a.dropdown-toggle").classList.add("active");

                // add hint to input text
                chatPromptInputEle.attributes.placeholder.value = `[${selectedModel}] CTRL+Enter to send`;
            });
        });
    }
}
