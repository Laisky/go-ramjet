"use strict";

const OpenaiTokenTypeProxy = "proxy",
    OpenaiTokenTypeDirect = "direct";

(function () {
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
