"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";

window.ready(() => {
    const OpenaiChatPricingPerKTokens = {
        "gpt-3.5-turbo": {
            prompt: 0.002,
            completion: 0.002,
        },
        "gpt-4-8k": {
            prompt: 0.03,
            completion: 0.06,
        },
        "gpt-4-32k": {
            prompt: 0.06,
            completion: 0.12,
        },
        "davinci": {
            prompt: 0.02,
            completion: 0.02,
        },
    };

    let chatContainer = document.getElementById("chatContainer"),
        configContainer = document.getElementById("hiddenChatConfigSideBar"),
        chatPromptInput = chatContainer.querySelector(".input.prompt"),
        chatPromptInputBtn = chatContainer.querySelector(".btn.send"),

        currentAIRespSSE, currentAIRespEle;

    (function main() {
        setupLocalStorage();
        setupConfig();
        setupSessionManager();
        setupChatInput();
        setupPromptManager();
    })();


    function setupLocalStorage() {
        if (localStorage.getItem("chat_user_session_1")) {
            return
        }

        // purge localstorage
        localStorage.clear();
    }


    function storageSessionKey(sessionID) {
        return "chat_user_session_" + sessionID;
    }

    function sessionChatHistory(sessionID) {
        return window.GetLocalStorage(storageSessionKey(sessionID)) || new Array;
    }

    function activeSessionChatHistory() {
        let sid = activeSessionID();
        if (!sid) {
            return new Array;
        }

        return sessionChatHistory(sid);
    }

    function activeSessionID() {
        let activeSession = chatContainer.querySelector(".sessionManager .card-body button.active");
        if (activeSession) {
            return activeSession.dataset.session;
        }

        return null;
    }

    function listenSessionSwitch(evt) {
        // deactive all sessions
        chatContainer
            .querySelectorAll(".sessionManager .sessions .list-group-item.active")
            .forEach((item) => {
                item.classList.remove("active");
            });
        evt.target.classList.add("active");

        // restore session hisgoty
        let sessionID = evt.target.dataset.session;
        chatContainer.querySelector(".conservations").innerHTML = "";
        sessionChatHistory(sessionID).forEach((item) => {
            append2Chats(item.role, item.content, true, item.prompt, item.chatID);
        });
    }

    function setupSessionManager() {
        // bind remove all sessions
        {
            chatContainer
                .querySelector(".sessionManager .btn.purge")
                .addEventListener("click", (evt) => {
                    let allkeys = Object.keys(localStorage);
                    allkeys.forEach((key) => {
                        if (key.startsWith("chat_user_session_")) {
                            localStorage.removeItem(key);
                        }
                    });

                    chatContainer.querySelector(".chatManager .conservations").innerHTML = "";
                    chatContainer.querySelector(".sessionManager .sessions").innerHTML = `
                    <div class="list-group" style="border-radius: 0%;">
                        <button type="button" class="list-group-item list-group-item-action session active" aria-current="true" data-session="1">
                            Session 1
                        </button>
                    </div>`;
                    chatContainer
                        .querySelector(".sessionManager .sessions .session")
                        .addEventListener("click", listenSessionSwitch);

                    window.SetLocalStorage(storageSessionKey(1), []);
                });
        }


        // restore all sessions from localStorage
        {
            let anyHistorySession = false;
            Object.keys(localStorage).forEach((key, idx) => {
                if (!key.startsWith("chat_user_session_")) {
                    return;
                }

                anyHistorySession = true;
            })

            if (!anyHistorySession) {
                window.SetLocalStorage("chat_user_session_1", []);
            }

            let firstSession = true;
            Object.keys(localStorage).forEach((key) => {
                if (!key.startsWith("chat_user_session_")) {
                    return;
                }

                anyHistorySession = true;
                let sessionID = parseInt(key.replace("chat_user_session_", ""));

                let active = "";
                if (firstSession) {
                    firstSession = false;
                    active = "active";
                }

                chatContainer
                    .querySelector(".sessionManager .sessions")
                    .insertAdjacentHTML(
                        "beforeend",
                        `<div class="list-group" style="border-radius: 0%;">
                            <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                                Session ${sessionID}
                            </button>
                        </div>`);
            });

            // restore conservation history
            activeSessionChatHistory().forEach((item) => {
                append2Chats(item.role, item.content, true, item.prompt, item.chatID);
            });

            window.EnableTooltipsEverywhere();
        }

        // new session
        {
            chatContainer
                .querySelector(".sessionManager .btn.new-session")
                .addEventListener("click", (evt) => {
                    let maxSessionID = 0;
                    Object.keys(localStorage).forEach((key) => {
                        if (key.startsWith("chat_user_session_")) {
                            let sessionID = parseInt(key.replace("chat_user_session_", ""));
                            if (sessionID > maxSessionID) {
                                maxSessionID = sessionID;
                            }
                        }
                    });

                    // deactive all sessions
                    chatContainer.querySelectorAll(".sessionManager .sessions .list-group-item.active").forEach((item) => {
                        item.classList.remove("active");
                    });

                    // add new active session
                    chatContainer
                        .querySelector(".chatManager .conservations").innerHTML = "";
                    let newSessionID = maxSessionID + 1;
                    chatContainer
                        .querySelector(".sessionManager .sessions")
                        .insertAdjacentHTML(
                            "afterbegin",
                            `<div class="list-group" style="border-radius: 0%;">
                                <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                    Session ${newSessionID}
                                </button>
                            </div>`);
                    window.SetLocalStorage(storageSessionKey(newSessionID), []);

                    // bind session switch listener for new session
                    chatContainer
                        .querySelector(`.sessionManager .sessions [data-session="${newSessionID}"]`)
                        .addEventListener("click", listenSessionSwitch);
                });
        }

        // bind session switch
        {
            chatContainer
                .querySelectorAll(".sessionManager .sessions .list-group .session")
                .forEach((item) => {
                    item.addEventListener("click", listenSessionSwitch);
                });
        }

    }



    function appendChats2Storage(role, content, prompt, chatid) {
        let storageActiveSessionKey = storageSessionKey(activeSessionID()),
            history = activeSessionChatHistory();

        history.push({
            role: role,
            content: content,
            prompt: prompt,
            chatID: chatid,
        });
        window.SetLocalStorage(storageActiveSessionKey, history);
    }


    function scrollChatToDown() {
        window.ScrollDown(chatContainer.querySelector(".chatManager .conservations"));
    }

    function getLastNChatMessages(N) {
        let messages = activeSessionChatHistory().filter((ele) => {
            return ele.role == RoleHuman;
        });

        messages = messages.slice(-N);
        if (GetLocalStorage("config_api_static_context")) {
            messages = [{
                role: RoleSystem,
                content: GetLocalStorage("config_api_static_context")
            }].concat(messages);
        }

        return messages;
    }

    function lockChatInput() {
        chatPromptInputBtn.classList.add("disabled");
        document.querySelectorAll("#chatContainer .conservations .role-ai .btn.reload").forEach((item) => {
            item.classList.add("disabled");
        });
    }
    function unlockChatInput() {
        chatPromptInputBtn.classList.remove("disabled");
        document.querySelectorAll("#chatContainer .conservations .role-ai .btn.reload").forEach((item) => {
            item.classList.remove("disabled");
        });
    }
    function isAllowChatPrompInput() {
        return !chatPromptInputBtn.classList.contains("disabled");
    }

    function parseChatResp(chatmodel, payload) {
        switch (chatmodel) {
            case ChatModelTurbo35:
            case ChatModelGPT4:
                return payload.choices[0].delta.content || "";
                break;
            case CompletionModelDavinci3:
                return payload.choices[0].text || "";
                break;
        }
    }


    function sendChat2Server(chatID) {
        let isReload = false,
            reqPromp;
        if (!chatID) {
            reqPromp = chatPromptInput.value || "";
            chatPromptInput.value = "";
            reqPromp = window.TrimSpace(reqPromp);
            if (reqPromp == "") {
                return;
            }

            chatID = append2Chats(RoleHuman, reqPromp);
            appendChats2Storage(RoleHuman, reqPromp);
        } else { // if chatID is not empty, it's a reload request
            reqPromp = chatContainer
                .querySelector(`.chatManager .conservations #${chatID}`)
                .dataset.prompt;
            isReload = true;
        }

        currentAIRespEle = chatContainer
            .querySelector(`.chatManager .conservations #${chatID} .ai-response`);
        currentAIRespEle = currentAIRespEle;
        lockChatInput();

        let chatmodel = (GetLocalStorage("config_chat_model") || ChatModelTurbo35);
        switch (chatmodel) {
            case ChatModelTurbo35:
            case ChatModelGPT4:
                let messages,
                    nContexts = parseInt(window.ChatNContexts());

                messages = getLastNChatMessages(nContexts);
                if (isReload) {
                    messages.push({
                        role: RoleHuman,
                        content: reqPromp
                    });
                    // messages = messages.slice(nContexts);
                }

                currentAIRespSSE = new SSE(window.OpenaiAPI(), {
                    headers: {
                        "Content-Type": "application/json",
                        "Authorization": "Bearer " + window.OpenaiToken(),
                        "X-Authorization-Type": window.OpenaiTokenType(),
                    },
                    method: "POST",
                    payload: JSON.stringify({
                        model: chatmodel,
                        stream: true,
                        max_tokens: parseInt(window.OpenaiMaxTokens()),
                        temperature: parseFloat(window.OpenaiTemperature()),
                        presence_penalty: parseFloat(window.OpenaiPresencePenalty()),
                        frequency_penalty: parseFloat(window.OpenaiFrequencyPenalty()),
                        messages: messages,
                        stop: ["\n\n"]
                    })
                });

                break;
            case CompletionModelDavinci3:
                currentAIRespSSE = new SSE(window.OpenaiAPI(), {
                    headers: {
                        "Content-Type": "application/json",
                        "Authorization": "Bearer " + window.OpenaiToken(),
                        "X-Authorization-Type": window.OpenaiTokenType(),
                    },
                    method: "POST",
                    payload: JSON.stringify({
                        model: chatmodel,
                        stream: true,
                        max_tokens: parseInt(window.OpenaiMaxTokens()),
                        temperature: parseFloat(window.OpenaiTemperature()),
                        presence_penalty: parseFloat(window.OpenaiPresencePenalty()),
                        frequency_penalty: parseFloat(window.OpenaiFrequencyPenalty()),
                        prompt: reqPromp,
                        stop: ["\n\n"]
                    })
                });

                break;
            case QAModelBasebit:
                // {
                //     "question": "XFS 是干啥的",
                //     "text": " XFS is a simple CLI tool that can be used to create volumes/mounts and perform simple filesystem operations.\n",
                //     "url": "http://xego-dev.basebit.me/doc/xfs/support/xfs2_cli_instructions/"
                // }
                let url;
                window.data['qa_chat_models'].forEach((item) => {
                    if (item['name'] == chatmodel) {
                        url = item['url'];
                    }
                });

                fetch(`${url}?q=${encodeURIComponent(reqPromp)}`, {
                    method: "GET",
                    headers: {
                        "Content-Type": "application/json",
                        "Authorization": "Bearer " + window.OpenaiToken(),
                        "X-Authorization-Type": window.OpenaiTokenType(),
                    }
                })
                    .then(async (resp) => {
                        let data = await resp.json();
                        if (data && data.text) {
                            let rawHTMLResp = `${data.text}\n\n📖: <pre>${data.url.replace(/, /g, "\n")}</pre>`
                            currentAIRespEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
                            appendChats2Storage(RoleAI, currentAIRespEle.innerHTML, reqPromp, chatID);
                            currentAIRespEle.scrollIntoView({ behavior: "smooth" });
                        }
                    })
                    .catch((err) => {
                        abortAIResp(err);
                    })
                    .finally(() => {
                        unlockChatInput();
                    });

                return
        }


        let rawHTMLResp = "";
        currentAIRespSSE.addEventListener("message", (evt) => {
            evt.stopPropagation();

            // console.log("got: ", evt.data);
            let isChatRespDone = false;
            if (evt.data == "[DONE]") {
                isChatRespDone = true
            }

            if (!isChatRespDone) {
                let payload = JSON.parse(evt.data),
                    respContent = parseChatResp(chatmodel, payload);

                if (payload.choices[0].finish_reason) {
                    isChatRespDone = true;
                }

                switch (currentAIRespEle.dataset.status) {
                    case "waiting":
                        currentAIRespEle.dataset.status = "writing";

                        if (respContent) {
                            currentAIRespEle.innerHTML = respContent;
                            rawHTMLResp += respContent;
                        } else {
                            currentAIRespEle.innerHTML = "";
                        }

                        break
                    case "writing":
                        if (respContent) {
                            rawHTMLResp += respContent;
                            currentAIRespEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
                        }

                        if (!isReload) {
                            scrollChatToDown();
                        }

                        break
                }
            }

            if (isChatRespDone) {
                currentAIRespSSE.close();
                currentAIRespSSE = null;

                let markdownConverter = new window.showdown.Converter();
                currentAIRespEle.innerHTML = window.Markdown2HTML(rawHTMLResp);

                Prism.highlightAll();
                window.EnableTooltipsEverywhere();

                if (!isReload) {
                    scrollChatToDown();
                }

                if (!isReload) {
                    appendChats2Storage(RoleAI, currentAIRespEle.innerHTML, reqPromp, chatID);
                } else {
                    replaceChatInStorage(RoleAI, chatID, currentAIRespEle.innerHTML);
                }

                unlockChatInput();
            }
        });

        currentAIRespSSE.onerror = (err) => {
            abortAIResp(err);
        };
        currentAIRespSSE.stream();
    }

    function replaceChatInStorage(role, chatID, content) {
        let storageKey = storageSessionKey(activeSessionID()),
            chats = window.GetLocalStorage(storageKey) || [];

        chats.forEach((item) => {
            if (item.chatID == chatID && item.role == role) {
                item.content = content;
            }
        });

        window.SetLocalStorage(storageKey, chats);
    }

    function abortAIResp(err) {
        if (currentAIRespSSE) {
            currentAIRespSSE.close();
            currentAIRespSSE = null;
        }

        let errMsg;
        try {
            errMsg = JSON.parse(err.data);
        } catch (e) {
            errMsg = e.toString();
        }

        if (currentAIRespEle.dataset.status == "waiting") {
            currentAIRespEle.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8;">${window.RenderStr2HTML(errMsg)}</pre>`;
        } else {
            currentAIRespEle.innerHTML += `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8;">${window.RenderStr2HTML(errMsg)}</pre>`;
        }

        // window.ScrollDown(chatContainer.querySelector(".chatManager .conservations"));
        currentAIRespEle.scrollIntoView({ behavior: "smooth" });
        unlockChatInput();
    }


    function setupChatInput() {
        // bind input press enter
        {
            let isComposition = false;
            chatPromptInput.
                addEventListener("compositionstart", (evt) => {
                    evt.stopPropagation();
                    isComposition = true;
                })
            chatPromptInput.
                addEventListener("compositionend", (evt) => {
                    evt.stopPropagation();
                    isComposition = false;
                })


            chatPromptInput.
                addEventListener("keydown", (evt) => {
                    evt.stopPropagation();
                    if (evt.key != 'Enter'
                        || isComposition
                        || (evt.key == 'Enter' && !(evt.ctrlKey || evt.metaKey || evt.altKey || evt.shiftKey))
                        || !isAllowChatPrompInput()) {
                        return;
                    }

                    sendChat2Server();
                    chatPromptInput.value = "";
                })
        }

        // bind input button
        chatPromptInputBtn.
            addEventListener("click", (evt) => {
                evt.stopPropagation();
                sendChat2Server();
                chatPromptInput.value = "";
            })
    }

    // append chat to conservation container
    //
    // @param {string} role - RoleHuman/RoleSystem/RoleAI
    // @param {string} text - chat text
    // @param {boolean} isHistory - is history chat, default false. if true, will not append to storage
    //
    // @return {string} conservationID - conservation id
    function append2Chats(role, text, isHistory = false, prompt = "", chatID) {
        let chatEle;

        if (!chatID) {
            chatID = `chat-${window.RandomString()}`
        }

        let reloadBtnHTML = `
            <div class="row d-flex align-items-center justify-content-center">
                <button class="btn btn-sm btn-outline-secondary reload" type="button" data-bs-toggle="tooltip" data-bs-placement="top" title="reload response base on latest context and chat model">
                    <i class="bi bi-repeat"></i>
                    Reload</button>
            </div>`;

        switch (role) {
            case RoleSystem:
                text = window.escapeHtml(text);

                chatEle = `
                    <div class="container-fluid row role-human">
                        <div class="col-1">💻</div>
                        <div class="col-11 text-start"><pre>${text}</pre></div>
                    </div>`
                break
            case RoleHuman:
                text = window.escapeHtml(text);

                let waitAI = "";
                if (!isHistory) {
                    waitAI = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" id="${chatID}" data-prompt="${text}">
                        <div class="row">
                            <div class="col-1">🤖️</div>
                            <div class="col-11 text-start ai-response" data-status="waiting">
                                <p class="card-text placeholder-glow">
                                    <span class="placeholder col-7"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-6"></span>
                                    <span class="placeholder col-8"></span>
                                </p>
                            </div>
                        </div>
                        ${reloadBtnHTML}
                    </div>`
                }

                chatEle = `
                    <div class="container-fluid row role-human">
                        <div class="col-1">🤔️</div>
                        <div class="col-11 text-start"><pre>${text}</pre></div>
                    </div>${waitAI}`
                break
            case RoleAI:
                if (prompt) {
                    chatEle = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" id="${chatID}" data-prompt="${prompt}">
                        <div class="row">
                            <div class="col-1">🤖️</div>
                            <div class="col-11 text-start ai-response" data-status="writing">${text}</div>
                        </div>
                        ${reloadBtnHTML}
                    </div>`
                } else {
                    chatEle = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" id="${chatID}">
                        <div class="col-1">🤖️</div>
                        <div class="col-11 text-start ai-response" data-status="writing">${text}</div>
                    </div>`
                }

                break
        }

        chatContainer.querySelector(".chatManager .conservations").
            insertAdjacentHTML("beforeend", chatEle);


        // bind reload button
        let reloadBtn = chatContainer.querySelector(`#${chatID} .btn.reload`);
        if (reloadBtn) {
            reloadBtn.addEventListener("click", (evt) => {
                evt.stopPropagation();
                document.querySelector(`#${chatID} .ai-response`).innerHTML = `
                <p class="card-text placeholder-glow">
                    <span class="placeholder col-7"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-4"></span>
                    <span class="placeholder col-6"></span>
                    <span class="placeholder col-8"></span>
                </p>`;

                sendChat2Server(chatID);
            });
        }

        return chatID;
    }


    function setupConfig() {
        let tokenTypeParent = configContainer.
            querySelector(".input-group.token-type");

        // set token type
        {
            let selectItems = tokenTypeParent
                .querySelectorAll("a.dropdown-item");
            switch (window.OpenaiTokenType()) {
                case "proxy":
                    configContainer
                        .querySelector(".token-type .show-val").innerHTML = "proxy";
                    ActiveElementsByData(selectItems, "value", "proxy");
                    break;
                case "direct":
                    configContainer
                        .querySelector(".token-type .show-val").innerHTML = "direct";
                    ActiveElementsByData(selectItems, "value", "direct");
                    break;
            }

            // bind evt listener for choose different token type
            selectItems.forEach((ele) => {
                ele.addEventListener("click", (evt) => {
                    // evt.stopPropagation();
                    configContainer
                        .querySelector(".token-type .show-val")
                        .innerHTML = evt.target.dataset.value;
                    window.SetLocalStorage("config_api_token_type", evt.target.dataset.value);
                })
            });
        }

        //  config_api_token_value
        {
            let apitokenInput = configContainer
                .querySelector(".input.api-token");
            apitokenInput.value = window.OpenaiToken();
            apitokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_token_value", evt.target.value);
            })
        }

        //  config_chat_n_contexts
        {
            let maxtokenInput = configContainer
                .querySelector(".input.contexts");
            maxtokenInput.value = window.ChatNContexts();
            configContainer.querySelector(".input-group.contexts .contexts-val").innerHTML = window.ChatNContexts();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_chat_n_contexts", evt.target.value);
                configContainer.querySelector(".input-group.contexts .contexts-val").innerHTML = evt.target.value;
            })
        }

        //  config_api_max_tokens
        {
            let maxtokenInput = configContainer
                .querySelector(".input.max-token");
            maxtokenInput.value = window.OpenaiMaxTokens();
            configContainer.querySelector(".input-group.max-token .max-token-val").innerHTML = window.OpenaiMaxTokens();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_max_tokens", evt.target.value);
                configContainer.querySelector(".input-group.max-token .max-token-val").innerHTML = evt.target.value;
            })
        }

        //  config_api_temperature
        {
            let maxtokenInput = configContainer
                .querySelector(".input.temperature");
            maxtokenInput.value = window.OpenaiTemperature();
            configContainer.querySelector(".input-group.temperature .temperature-val").innerHTML = window.OpenaiTemperature();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_temperature", evt.target.value);
                configContainer.querySelector(".input-group.temperature .temperature-val").innerHTML = evt.target.value;
            })
        }

        //  config_api_presence_penalty
        {
            let maxtokenInput = configContainer
                .querySelector(".input.presence_penalty");
            maxtokenInput.value = window.OpenaiPresencePenalty();
            configContainer.querySelector(".input-group.presence_penalty .presence_penalty-val").innerHTML = window.OpenaiPresencePenalty();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_presence_penalty", evt.target.value);
                configContainer.querySelector(".input-group.presence_penalty .presence_penalty-val").innerHTML = evt.target.value;
            })
        }

        //  config_api_frequency_penalty
        {
            let maxtokenInput = configContainer
                .querySelector(".input.frequency_penalty");
            maxtokenInput.value = window.OpenaiFrequencyPenalty();
            configContainer.querySelector(".input-group.frequency_penalty .frequency_penalty-val").innerHTML = window.OpenaiFrequencyPenalty();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_frequency_penalty", evt.target.value);
                configContainer.querySelector(".input-group.frequency_penalty .frequency_penalty-val").innerHTML = evt.target.value;
            })
        }

        //  config_api_static_context
        {
            let staticConfigInput = configContainer
                .querySelector(".input.static-prompt");
            staticConfigInput.value = window.OpenaiChatStaticContext();
            staticConfigInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_static_context", evt.target.value);
            })
        }

        // bind reset button
        {
            configContainer.querySelector(".btn.reset")
                .addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    localStorage.clear();
                    location.reload();
                })
        }

        // bind submit button
        {
            configContainer.querySelector(".btn.submit")
                .addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    location.reload();
                })

        }

        window.EnableTooltipsEverywhere();
    }

    function loadPromptShortcutsFromStorage() {
        let shortcuts = window.GetLocalStorage(StorageKeyPromptShortCuts);
        if (!shortcuts) {
            shortcuts = [
                {
                    title: "chat",
                    description: "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can.",
                },
                {
                    title: "中英互译",
                    description: 'As an English-Chinese translator, your task is to accurately translate text between the two languages. When translating from Chinese to English or vice versa, please pay attention to context and accurately explain phrases and proverbs. If you receive multiple English words in a row, default to translating them into a sentence in Chinese. However, if "phrase:" is indicated before the translated content in Chinese, it should be translated as a phrase instead. Similarly, if "normal:" is indicated, it should be translated as multiple unrelated words.Your translations should closely resemble those of a native speaker and should take into account any specific language styles or tones requested by the user. Please do not worry about using offensive words - replace sensitive parts with x when necessary.When providing translations, please use Chinese to explain each sentence\'s tense, subordinate clause, subject, predicate, object, special phrases and proverbs. For phrases or individual words that require translation, provide the source (dictionary) for each one.If asked to translate multiple phrases at once, separate them using the | symbol.Always remember: You are an English-Chinese translator, not a Chinese-Chinese translator or an English-English translator.Please review and revise your answers carefully before submitting.'
                }
            ];
            window.SetLocalStorage(StorageKeyPromptShortCuts, shortcuts);
        }

        return shortcuts;
    }

    // append prompt shortcuts to html and localstorage
    //
    // @param {Object} shortcut - shortcut object
    function appendPromptShortcut(shortcut, storage = false) {
        let promptShortcutContainer = configContainer.querySelector(".prompt-shortcuts");

        // add to local storage
        if (storage) {
            let shortcuts = loadPromptShortcutsFromStorage();
            shortcuts.push(shortcut);
            window.SetLocalStorage(StorageKeyPromptShortCuts, shortcuts);
        }

        // new element
        let ele = document.createElement("span");
        ele.classList.add("badge", "text-bg-info");
        ele.dataset.prompt = shortcut.description;
        ele.innerHTML = ` ${shortcut.title}  <i class="bi bi-trash"></i>`;

        // add delete click event
        ele.querySelector("i.bi-trash").addEventListener("click", (evt) => {
            evt.stopPropagation();
            evt.target.parentElement.remove();

            // remove localstorage shortcut
            let shortcuts = window.GetLocalStorage(StorageKeyPromptShortCuts);
            shortcuts = shortcuts.filter((item) => item.title !== shortcut.title);
            window.SetLocalStorage(StorageKeyPromptShortCuts, shortcuts);
        });

        // add click event
        // replace system prompt
        ele.addEventListener("click", (evt) => {
            evt.stopPropagation();
            let promptInput = configContainer.querySelector(".input.static-prompt");
            promptInput.value = evt.target.dataset.prompt;
        });

        // add to html
        promptShortcutContainer.appendChild(ele);
    }

    function setupPromptManager() {
        // restore shortcuts from localstorage
        {
            let shortcuts = loadPromptShortcutsFromStorage();
            shortcuts.forEach((shortcut) => {
                appendPromptShortcut(shortcut, false);
            });
        }
    }
});
