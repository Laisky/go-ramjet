"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";

(function () {
    let chatContainer = document.getElementById("chatContainer"),
        chatPromptInput = chatContainer.querySelector(".input.prompt"),
        chatPromptInputBtn = chatContainer.querySelector(".btn.send"),

        currentAIRespSSE, currentAIRespEle;

    (function main() {
        setupLocalStorage();
        setupConfig();
        setupSessionManager();
        setupChatInput();
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

        messages = [{
            role: RoleSystem,
            content: "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question and if possible, reference the context as much as you can."
        }].concat(messages);


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
            reqPromp = chatContainer.querySelector(`.chatManager .conservations #${chatID}`).dataset.prompt;
            isReload = true;
        }

        currentAIRespEle = chatContainer.querySelector(`.chatManager .conservations #${chatID} .ai-response`);
        currentAIRespEle = currentAIRespEle;
        lockChatInput();

        let chatmodel = (GetLocalStorage("config_chat_model") || ChatModelTurbo35);
        switch (chatmodel) {
            case ChatModelTurbo35:
            case ChatModelGPT4:
                let messages;
                messages = getLastNChatMessages(6);
                if (isReload) {
                    messages.push({
                        role: RoleHuman,
                        content: reqPromp
                    });
                    messages = messages.slice(-6);
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
                        prompt: reqPromp,
                        stop: ["\n\n"]
                    })
                });

                break;
        }


        let rawHTMLResp = "";
        currentAIRespSSE.addEventListener("message", (evt) => {
            evt.stopPropagation();

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
        currentAIRespSSE.close();
        currentAIRespSSE = null;

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

        switch (role) {
            case RoleSystem:
                chatEle = `
                    <div class="container-fluid row role-human">
                        <div class="col-1">💻</div>
                        <div class="col-11 text-start">${text}</div>
                    </div>`
                break
            case RoleHuman:
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
                        <div class="row d-flex align-items-center justify-content-center">
                            <button class="btn btn-sm btn-outline-secondary reload" type="button"><i class="bi bi-repeat"></i> Reload</button>
                        </div>
                    </div>`
                }

                chatEle = `
                    <div class="container-fluid row role-human">
                        <div class="col-1">🤔️</div>
                        <div class="col-11 text-start">${text}</div>
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
                        <div class="row d-flex align-items-center justify-content-center">
                            <button class="btn btn-sm btn-outline-secondary reload" type="button"><i class="bi bi-repeat"></i> Reload</button>
                        </div>
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
        // TODO 当恢复历史会话时，因为没有保留历史 reponse 对应的 prompt，暂时不支持 reload
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
        let tokenTypeParent = document.
            querySelector("#hiddenChatConfigSideBar .input-group.token-type");

        // set token type
        {
            let selectItems = tokenTypeParent.querySelectorAll("a.dropdown-item");
            switch (window.OpenaiTokenType()) {
                case "proxy":
                    ActiveElementsByData(selectItems, "value", "proxy");
                    break;
                case "direct":
                    ActiveElementsByData(selectItems, "value", "direct");
                    break;
            }

            // bind evt listener for choose different token type
            selectItems.forEach((ele) => {
                ele.addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    window.SetLocalStorage("config_api_token_type", evt.target.dataset.value);
                })
            });
        }

        //  config_api_token_value
        {
            let apitokenInput = document
                .querySelector("#hiddenChatConfigSideBar .input.api-token");
            apitokenInput.value = window.OpenaiToken();
            apitokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_token_value", evt.target.value);
            })
        }

        //  config_api_max_tokens
        {
            let maxtokenInput = document
                .querySelector("#hiddenChatConfigSideBar .input.max-token");
            maxtokenInput.value = window.OpenaiMaxTokens();
            maxtokenInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_max_tokens", evt.target.value);
            })
        }

        //  config_api_static_context
        {
            let staticConfigInput = document
                .querySelector("#hiddenChatConfigSideBar .input.static-config");
            staticConfigInput.value = window.OpenaiChatStaticContext();
            staticConfigInput.addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_static_context", evt.target.value);
            })
        }

        // bind reset button
        {
            document.querySelector("#hiddenChatConfigSideBar .btn.reset")
                .addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    localStorage.clear();
                    location.reload();
                })
        }

        // bind submit button
        {
            document.querySelector("#hiddenChatConfigSideBar .btn.submit")
                .addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    location.reload();
                })

        }
    }
})();
