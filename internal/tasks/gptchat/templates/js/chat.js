"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";

(function () {
    let chatContainer = document.getElementById("chatContainer"),
        chatPromptInput = chatContainer.querySelector(".input.prompt"),
        chatPromptInputBtn = chatContainer.querySelector(".btn.send");

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
            append2Chats(item.role, item.content, true);
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
                append2Chats(item.role, item.content, true);
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



    function appendChants2Storage(role, content) {
        let storageActiveSessionKey = "chat_user_session_" + activeSessionID(),
            history = activeSessionChatHistory();

        history.push({
            "role": role,
            "content": content,
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

    function lockChatPromptInput() {
        chatPromptInput.classList.add("disabled");
    }
    function unlockChatPromptInput() {
        chatPromptInput.classList.remove("disabled");
    }
    function isAllowChatPrompInput() {
        return !chatPromptInput.classList.contains("disabled");
    }

    function sendChat2server() {
        let prompt = chatPromptInput.value || "";
        chatPromptInput.value = "";
        prompt = window.TrimSpace(prompt);
        if (prompt == "") {
            return;
        }


        append2Chats(RoleHuman, prompt);
        appendChants2Storage(RoleHuman, prompt);
        let lastAIInputEle = chatContainer.querySelector(".chatManager .conservations").querySelector(".row.role-ai:last-child").querySelector(".text-start");

        lockChatPromptInput();

        let source = new SSE(window.OpenaiAPI(), {
            headers: {
                "Content-Type": "application/json",
                "Authorization": "Bearer " + window.OpenaiToken(),
                "X-Authorization-Type": window.OpenaiTokenType(),
            },
            method: "POST",
            payload: JSON.stringify({
                model: "gpt-3.5-turbo",
                stream: true,
                max_tokens: parseInt(window.OpenaiMaxTokens()),
                messages: getLastNChatMessages(6),
                stop: ["\n\n"]
            })
        });


        let rawHTMLResp = "";
        source.addEventListener("message", (evt) => {
            evt.stopPropagation();

            let payload = JSON.parse(evt.data);
            switch (lastAIInputEle.dataset.status) {
                case "waiting":
                    lastAIInputEle.dataset.status = "writing";
                    if (payload.choices[0].delta.content) {
                        lastAIInputEle.innerHTML = payload.choices[0].delta.content;
                        rawHTMLResp += payload.choices[0].delta.content;
                    } else {
                        lastAIInputEle.innerHTML = "";
                    }

                    break
                case "writing":
                    if (payload.choices[0].delta.content) {
                        rawHTMLResp += payload.choices[0].delta.content;
                        lastAIInputEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
                    }

                    scrollChatToDown();
                    break
            }

            if (payload.choices[0].finish_reason) {
                source.close();

                let markdownConverter = new window.showdown.Converter();
                lastAIInputEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
                scrollChatToDown();

                appendChants2Storage(RoleAI, lastAIInputEle.innerHTML);
                unlockChatPromptInput();
            }
        });

        source.onerror = (err) => {
            source.close();
            if (lastAIInputEle.dataset.status == "waiting") {
                lastAIInputEle.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8;">${window.RenderStr2HTML(JSON.parse(err.data))}</pre>`;
            }

            window.ScrollDown(chatContainer.querySelector(".chatManager .conservations"));
            unlockChatPromptInput();
        };
        source.stream();
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

                    sendChat2server();
                    chatPromptInput.value = "";
                })
        }

        // bind input button
        chatPromptInputBtn.
            addEventListener("click", (evt) => {
                evt.stopPropagation();
                sendChat2server();
                chatPromptInput.value = "";
            })
    }

    function append2Chats(role, text, isHistory = false) {
        let chatEle;
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
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;">
                        <div class="col-1">🤖️</div>
                        <div class="col-11 text-start" data-status="waiting">
                            <p class="card-text placeholder-glow">
                                <span class="placeholder col-7"></span>
                                <span class="placeholder col-4"></span>
                                <span class="placeholder col-4"></span>
                                <span class="placeholder col-6"></span>
                                <span class="placeholder col-8"></span>
                            </p>
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
                chatEle = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;">
                        <div class="col-1">🤖️</div>
                        <div class="col-11 text-start">${text}</div>
                    </div>`
                break
        }

        chatContainer.querySelector(".chatManager .conservations").
            insertAdjacentHTML("beforeend", chatEle);

        // chatEle = DOMParser.parseFromString(chatEle);
        // chatContainer.querySelector(".chatManager .conservations").insertAdjacentElement("beforeend", chatEle);
        // return chatEle
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
