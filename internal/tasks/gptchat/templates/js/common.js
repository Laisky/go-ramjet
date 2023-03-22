"use strict";

const OpenaiTokenTypeProxy = "proxy",
    OpenaiTokenTypeDirect = "direct";

const ChatModelTurbo35 = "gpt-3.5-turbo",
    ChatModelGPT4 = "gpt-4",
    CompletionModelDavinci3 = "text-davinci-003";

window.ready(() => {
    (function main() {
        checkVersion();
        setupHeader();
    })();


    function checkVersion() {
        SetLocalStorage("global_version", Version);
        if (((new Date()).getTime() - (new Date(GetLocalStorage("global_version"))).getTime()) > 86400000) { // 1 day
            window.location.reload();
        }
    }

    function setupHeader() {
        // setup chat models
        {
            // set default chat model
            if (!GetLocalStorage("config_chat_model")) {
                SetLocalStorage("config_chat_model", ChatModelTurbo35);
            }

            let modelElems = document.querySelectorAll("#headerbar .chat-models li a, .complete-models li a");

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
        let v = window.GetLocalStorage("config_api_static_context");
        if (!v) {
            v = "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question and if possible, reference the context as much as you can."
            window.SetLocalStorage("config_api_static_context", v);
        }

        return v;
    };
});
