"use strict";

const OpenaiTokenTypeProxy = "proxy",
    OpenaiTokenTypeDirect = "direct";

const ChatModelTurbo35 = "gpt-3.5-turbo",
    ChatModelGPT4 = "gpt-4",
    QAModelBasebit = "qa-bbt-xego",
    QAModelSecurity = "qa-security",
    QAModelCustom = "qa-custom",
    CompletionModelDavinci3 = "text-davinci-003";

const StorageKeyPromptShortCuts = "config_prompt_shortcuts",
    // custom dataset's end-to-end password
    StorageKeyCustomDatasetPassword = "config_chat_dataset_key",
    StorageKeySystemPrompt = "config_api_static_context";

(function () {

    (function main() {
        checkVersion();
        setupHeader();
        setupConfirmModal();
        setupSingleInputModal();
    })();
})();


function checkVersion() {
    SetLocalStorage("version", Version);
    let lastReloadAt = GetLocalStorage("last_reload_at") || Version;
    if (((new Date()).getTime() - (new Date(lastReloadAt)).getTime()) > 86400000) { // 1 day
        SetLocalStorage("last_reload_at", (new Date()).toISOString());
        // window.location.reload();
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

function setupConfirmModal() {
    window.deleteCheckCallback = null;
    window.deleteCheckModal = new bootstrap.Modal(document.getElementById("deleteCheckModal"));
    document.getElementById("deleteCheckModal")
        .querySelector(".modal-body .yes")
        .addEventListener("click",async (e) => {
            e.preventDefault();

            if (window.deleteCheckCallback) {
                await window.deleteCheckCallback();
            }

            window.deleteCheckModal.hide();
        });
}

function setupHeader() {
    // setup chat qa models
    {
        let qaModels = window.data["qa_chat_models"] || [],
            headerBarEle = document.getElementById("headerbar"),
            qaModelsContainer = headerBarEle.querySelector(".dropdown-menu.qa-models");

        qaModels.forEach((model) => {
            let li = document.createElement("li");
            let a = document.createElement("a");
            a.href = "#";
            a.classList.add("dropdown-item");
            a.dataset.model = model.name;
            a.textContent = model.name;
            li.appendChild(a);
            qaModelsContainer.appendChild(li);
        });
    }

    // setup chat models
    {
        // set default chat model
        if (!GetLocalStorage("config_chat_model")) {
            SetLocalStorage("config_chat_model", ChatModelTurbo35);
        }

        let modelElems = document.querySelectorAll("#headerbar .chat-models li a, .qa-models li a");

        // set active
        let model = GetLocalStorage("config_chat_model");
        modelElems.forEach((elem) => {
            if (elem.dataset.model === model) {
                elem.classList.add("active");
            }
        });

        // listen click events
        modelElems.forEach((elem) => {
            elem.addEventListener("click", (e) => {
                e.preventDefault();
                modelElems.forEach((elem) => {
                    elem.classList.remove("active");
                });

                e.target.classList.add("active");
                let model = e.target.dataset.model;
                SetLocalStorage("config_chat_model", model);
            });
        });
    }
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
    let t = window.GetLocalStorage("config_api_token_type");
    if (!t) {
        t = OpenaiTokenTypeProxy;
        window.SetLocalStorage("config_api_token_type", t);
    }

    return t;
};

window.OpenaiToken = () => {
    let v = window.GetLocalStorage("config_api_token_value");
    if (!v) {
        v = "DEFAULT_PROXY_TOKEN"
        window.SetLocalStorage("config_api_token_value", v);
    }

    return v
};

window.OpenaiMaxTokens = () => {
    let v = window.GetLocalStorage("config_api_max_tokens");
    if (!v) {
        v = "500";
        window.SetLocalStorage("config_api_max_tokens", v);
    }

    return v;
};

window.OpenaiTemperature = () => {
    let v = window.GetLocalStorage("config_api_temperature");
    if (!v) {
        v = "1";
        window.SetLocalStorage("config_api_temperature", v);
    }

    return v;
};

window.OpenaiPresencePenalty = () => {
    let v = window.GetLocalStorage("config_api_presence_penalty");
    if (!v) {
        v = "0";
        window.SetLocalStorage("config_api_presence_penalty", v);
    }

    return v;
};

window.OpenaiFrequencyPenalty = () => {
    let v = window.GetLocalStorage("config_api_frequency_penalty");
    if (!v) {
        v = "0";
        window.SetLocalStorage("config_api_frequency_penalty", v);
    }

    return v;
};

window.ChatNContexts = () => {
    let v = window.GetLocalStorage("config_chat_n_contexts");
    if (!v) {
        v = "3";
        window.SetLocalStorage("config_chat_n_contexts", v);
    }

    return v;
};

window.OpenaiChatStaticContext = () => {
    let v = window.GetLocalStorage(StorageKeySystemPrompt);
    if (!v) {
        v = "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can."
        window.SetLocalStorage(StorageKeySystemPrompt, v);
    }

    return v;
};
