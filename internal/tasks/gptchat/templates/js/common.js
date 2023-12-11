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
    StorageKeySystemPrompt = "config_api_static_context",
    StorageKeyPinnedMaterials = "config_api_pinned_materials",
    StorageKeyAllowedModels = "config_chat_models";

const KvKeyPrefixSessionHistory = "chat_user_session_",
    KvKeyPrefixSessionConfig = "chat_user_session_config_";

window.IsChatModel = (model) => {
    return ChatModels.includes(model);
};

window.IsQaModel = (model) => {
    return QaModels.includes(model);
};

window.IsCompletionModel = (model) => {
    return CompletionModels.includes(model);
};

window.IsImageModel = (model) => {
    return ImageModels.includes(model);
};

window.IsChatModelAllowed = (model) => {
    let allowed_models = GetLocalStorage(StorageKeyAllowedModels);
    if (!allowed_models) {
        return false;
    }

    return allowed_models.includes(model);
}

window.ShowSpinner = () => {
    document.getElementById("spinner").toggleAttribute("hidden", false);
};
window.HideSpinner = () => {
    document.getElementById("spinner").toggleAttribute("hidden", true);
};


window.OpenaiAPI = () => {
    switch (window.OpenaiTokenType()) {
        case OpenaiTokenTypeProxy:
            return window.data.openai.proxy;
        case OpenaiTokenTypeDirect:
            return window.data.openai.direct;
    }
};

window.OpenaiUserIdentify = () => {
    t = window.OpenaiToken();
    return t;
};

window.OpenaiTokenType = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["token_type"];
};

/**
 * Generates a random string of the specified length.
 * @param {number} length - The length of the string to generate.
 * @returns {string} - The generated random string.
 */
window.RandomString = (length) => {
    const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length));
    }

    return result;
}

window.OpenaiToken = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig(),
     apikey;

    // get token from url params first
    {
        apikey = new URLSearchParams(window.location.search).get("apikey");

        if (apikey) {
            // fix: sometimes url.searchParams.delete() works too quickly,
            // that let another caller rewrite apikey to FREE-TIER,
            // so we delay 1s to delete apikey from url params.
            window.setTimeout(() => {
                let v = new URLSearchParams(window.location.search).get("apikey");
                if (!v) {
                    return;
                }

                // remove apikey from url params
                let url = new URL(window.location.href);
                url.searchParams.delete("apikey");
                window.history.pushState({}, document.title, url);
            }, 1000);
        }
    }

    // get token from localstorage
    if (!apikey) {
        let sid = activeSessionID(),
            skey = `KvKeyPrefixSessionConfig${sid}`,
            sconfig = window.KvGet(skey) || newSessionConfig();
        apikey = sconfig["token_type"];
    }

    sconfig["api_token"] = apikey;
    window.KvSet(skey, sconfig);
};

window.OpenaiSelectedModel = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["selected_model"];
}

window.OpenaiMaxTokens = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["max_tokens"];
};

window.OpenaiTemperature = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["temperature"];
};

window.OpenaiPresencePenalty = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["presence_penalty"];
};

window.OpenaiFrequencyPenalty = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["frequency_penalty"];
};

window.ChatNContexts = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["n_contexts"];
};

window.OpenaiChatStaticContext = () => {
    let sid = activeSessionID(),
        skey = `KvKeyPrefixSessionConfig${sid}`,
        sconfig = window.KvGet(skey) || newSessionConfig();
    return sconfig["system_prompt"];
};


window.SingleInputModal = (title, message, callback) => {
    const modal = document.getElementById("singleInputModal");
    window.singleInputCallback = async () => {
        try {
            window.ShowSpinner();
            await callback(modal.querySelector(".modal-body input").value)
        } finally {
            window.HideSpinner();
        }
    };

    modal.querySelector(".modal-title").innerHTML = title;
    modal.querySelector(".modal-body label.form-label").innerHTML = message;
    window.singleInputModal.show();
};

// show modal to confirm,
// callback will be called if user click yes
//
// params:
//   - title: modal title
//   - callback: async callback function
window.ConfirmModal = (title, callback) => {
    window.deleteCheckCallback = async () => {
        try {
            window.ShowSpinner();
            await callback()
        } finally {
            window.HideSpinner();
        }
    };
    document.getElementById("deleteCheckModal").querySelector(".modal-title").innerHTML = title;
    window.deleteCheckModal.show();
};

(function () {
    (async function main() {
        window.OpenaiToken();
        await dataMigrate();
        await setupHeader();
        setupConfirmModal();
        setupSingleInputModal();

        await setupChatJs();
    })();
})();

async function dataMigrate() {
    // move config from localstorage to session config
    {
        let sid = activeSessionID(),
            skey = `KvKeyPrefixSessionConfig${sid}`,
            sconfig = window.KvGet(skey);

        if (!sconfig) {
            sconfig = newSessionConfig();

            sconfig["token_type"] = GetLocalStorage("config_api_token_value");
            sconfig["max_tokens"] = GetLocalStorage("config_api_max_tokens");
            sconfig["temperature"] = GetLocalStorage("config_api_temperature");
            sconfig["presence_penalty"] = GetLocalStorage("config_api_presence_penalty");
            sconfig["frequency_penalty"] = GetLocalStorage("config_api_frequency_penalty");
            sconfig["n_contexts"] = GetLocalStorage("config_api_n_contexts");
            sconfig["system_prompt"] = GetLocalStorage("config_api_static_context");
            sconfig["selected_model"] = GetLocalStorage("config_chat_model");

            window.KvSet(skey, sconfig);
        }
    }
}


function setupSingleInputModal() {
    window.singleInputCallback = null;
    window.singleInputModal = new bootstrap.Modal(document.getElementById("singleInputModal"));
    document.getElementById("singleInputModal")
        .querySelector(".modal-body .yes")
        .addEventListener("click", async (e) => {
            e.preventDefault();

            if (window.singleInputCallback) {
                await window.singleInputCallback();
            }

            window.singleInputModal.hide();
        });
}


function setupConfirmModal() {
    window.deleteCheckCallback = null;
    window.deleteCheckModal = new bootstrap.Modal(document.getElementById("deleteCheckModal"));
    document.getElementById("deleteCheckModal")
        .querySelector(".modal-body .yes")
        .addEventListener("click", async (e) => {
            e.preventDefault();

            if (window.deleteCheckCallback) {
                await window.deleteCheckCallback();
            }

            window.deleteCheckModal.hide();
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
        let selectedModel = window.OpenaiSelectedModel();

        // get users' models
        let headers = new Headers();
        headers.append("Authorization", "Bearer " + window.OpenaiToken());
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

        window.SetLocalStorage(StorageKeyAllowedModels, data.allowed_models);
        allowedModels = data.allowed_models;

        if (!data.allowed_models.includes(selectedModel)) {
            selectedModel = data.allowed_models[0];
            SetLocalStorage("config_chat_model", selectedModel);
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

        // set selected model
        // add active to class
        document.querySelectorAll("#headerbar .navbar-nav a.dropdown-toggle")
            .forEach((elem) => {
                elem.classList.remove("active");
            });
        document
            .querySelectorAll("#headerbar .chat-models li a, "
                + "#headerbar .qa-models li a, "
                + "#headerbar .image-models li a"
            )
            .forEach((elem) => {
                elem.classList.remove("active");

                if (elem.dataset.model == selectedModel) {
                    elem.classList.add("active");
                    elem.closest(".dropdown").querySelector("a.dropdown-toggle").classList.add("active");
                }
            });

        // listen click events
        let modelElems = document
            .querySelectorAll("#headerbar .chat-models li a, "
                + "#headerbar .qa-models li a, "
                + "#headerbar .image-models li a"
            );
        modelElems.forEach((elem) => {
            elem.addEventListener("click", (evt) => {
                evt.preventDefault();
                modelElems.forEach((elem) => {
                    elem.classList.remove("active");
                });

                evt.target.classList.add("active");
                let model = evt.target.dataset.model;
                SetLocalStorage("config_chat_model", model);

                // add active to class
                document.querySelectorAll("#headerbar .navbar-nav a.dropdown-toggle")
                    .forEach((elem) => {
                        elem.classList.remove("active");
                    });
                evt.target.closest(".dropdown").querySelector("a.dropdown-toggle").classList.add("active");

                // add hint to input text
                chatPromptInputEle.attributes.placeholder.value = `[${model}] CTRL+Enter to send`;
            });
        });
    }
}
