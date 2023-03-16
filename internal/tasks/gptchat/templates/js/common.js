"use strict";

const Version = "1.1.0";

const OpenaiTokenTypeProxy = "proxy",
    OpenaiTokenTypeDirect = "direct";

const ChatModelTurbo35 = "gpt-3.5-turbo",
    ChatModelGPT4 = "gpt-4",
    CompletionModelDavinci3 = "text-davinci-003";

(function () {
    (function main() {
        checkVersion();
        setupHeader();
    })();

    function checkVersion() {
        let latestVer = GetLocalStorage("global_version");
        if (latestVer !== Version) {
            SetLocalStorage("global_version", Version);
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

    window.OpenaiChatStaticContext = () => {
        let v = window.GetLocalStorage("config_api_static_context");
        if (!v) {
            v = ""
            window.SetLocalStorage("config_api_static_context", v);
        }

        return v;
    };
})()
