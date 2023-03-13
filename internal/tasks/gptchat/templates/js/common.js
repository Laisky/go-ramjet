"use strict";

const OpenaiTokenTypeProxy = "proxy",
OpenaiTokenTypeDirect = "direct";

(function(){
    window.OpenaiAPI = () => {
        switch (window.OpenaiTokenType()) {
            case OpenaiTokenTypeProxy:
                return window.data.openai.proxy;
            case OpenaiTokenTypeDirect:
                return window.data.openai.direct;
        }
    };

    window.OpenaiTokenType = () => {
        return window.GetLocalStorage("config_api_token_type") || OpenaiTokenTypeProxy;
    };

    window.OpenaiToken = () => {
        return window.GetLocalStorage("config_api_token_value") || "DEFAULT_PROXY_TOKEN";
    };

    window.OpenaiMaxTokens = () => {
        return window.GetLocalStorage("config_api_max_tokens") || "500";
    };

    window.OpenaiChatStaticContext = () => {
        return window.GetLocalStorage("config_api_static_context") || "";
    };
})()
