"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";

(function () {
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
        let activeSession = document.getElementById("chatSessionManager").querySelector(".card-body button.active");
        if (activeSession) {
            return activeSession.dataset.session;
        }

        return null;
    }

    function listenSessionSwitch(evt) {
        // deactive all sessions
        document.querySelectorAll("#chatSessionContainer .list-group-item.active").forEach((item) => {
            item.classList.remove("active");
        });
        evt.target.classList.add("active");

        // restore session hisgoty
        let sessionID = evt.target.dataset.session;
        document.getElementById("chatConversation").innerHTML = "";
        sessionChatHistory(sessionID).forEach((item) => {
            append2Chats(item.role, item.content, true);
        });
    }

    function setupSessionManager() {
        // bind remove all sessions
        {
            document.getElementById("purgeAllSessionsBtn").addEventListener("click", (evt) => {
                let allkeys = Object.keys(localStorage);
                allkeys.forEach((key) => {
                    if (key.startsWith("chat_user_session_")) {
                        localStorage.removeItem(key);
                    }
                });

                document.getElementById("chatConversation").innerHTML = "";
                document.getElementById("chatSessionContainer").innerHTML = `
                    <div class="list-group" style="border-radius: 0%;">
                        <button type="button" class="list-group-item list-group-item-action active" aria-current="true" data-session="1">
                            会话 1
                        </button>
                    </div>`;
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

                document.getElementById("chatSessionContainer").insertAdjacentHTML("beforeend", `
                    <div class="list-group" style="border-radius: 0%;">
                        <button type="button" class="list-group-item list-group-item-action ${active}" aria-current="true" data-session="${sessionID}">
                            会话 ${sessionID}
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
            document.getElementById("newChatSessionBtn").addEventListener("click", (evt) => {
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
                document.querySelectorAll("#chatSessionContainer .list-group-item.active").forEach((item) => {
                    item.classList.remove("active");
                });

                // add new active session
                document.getElementById("chatConversation").innerHTML = "";
                let newSessionID = maxSessionID + 1;
                document.getElementById("chatSessionContainer").insertAdjacentHTML("afterbegin", `
                    <div class="list-group" style="border-radius: 0%;">
                        <button type="button" class="list-group-item list-group-item-action active" aria-current="true" data-session="${newSessionID}">
                            会话 ${newSessionID}
                        </button>
                    </div>`);
                window.SetLocalStorage(storageSessionKey(newSessionID), []);

                // bind session switch listener for new session
                document.getElementById("chatSessionContainer").querySelector(`[data-session="${newSessionID}"]`).addEventListener("click", listenSessionSwitch);
            });
        }

        // switch session
        {
            document.querySelectorAll("#chatSessionContainer .list-group button").forEach((item) => {
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
        window.ScrollDown(document.getElementById("chatConversation"));
    }

    function getLastNChatMessages(N) {
        let messages = activeSessionChatHistory().filter((ele) => {
            return ele.role == RoleHuman;
        });

        messages = messages.slice(-N);
        if (GetLocalStorage("config_api_static_context")) {
            messages = [{
                role: RoleHuman,
                content: GetLocalStorage("config_api_static_context")
            }].concat(messages);
        }

        return messages;
    }

    function sendChat2server() {
        let prompt = document.getElementById("inputPrompt").value || "";
        document.getElementById("inputPrompt").value = "";
        prompt = window.TrimSpace(prompt);
        if (prompt == "") {
            return;
        }


        append2Chats(RoleHuman, prompt);
        appendChants2Storage(RoleHuman, prompt);
        let lastAIInputEle = document.getElementById("chatConversation").querySelector(".row.role-ai:last-child").querySelector(".text-start");

        let inputBtn = document.getElementById("inputPromptBtn");
        inputBtn.classList.add("disabled");  // lock input

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
                inputBtn.classList.remove("disabled");  // unlock input
            }
        });

        source.onerror = (err) => {
            source.close();
            if (lastAIInputEle.dataset.status == "waiting") {
                lastAIInputEle.innerHTML = `<p>⚠️出错了……</p><pre style="background-color: #f8e8e8;">${window.RenderStr2HTML(JSON.parse(err.data))}</pre>`;
            }

            window.ScrollDown(document.getElementById("chatConversation"));
            inputBtn.classList.remove("disabled");  // unlock input
        };
        source.stream();
    }


    function setupChatInput() {
        // bind input press enter
        {
            let isComposition = false;
            document.getElementById("inputPrompt").addEventListener("compositionstart", (evt) => {
                evt.stopPropagation();
                isComposition = true;
            })
            document.getElementById("inputPrompt").addEventListener("compositionend", (evt) => {
                evt.stopPropagation();
                isComposition = false;
            })


            document.getElementById("inputPrompt").addEventListener("keydown", (evt) => {
                evt.stopPropagation();
                if (evt.key != 'Enter' || isComposition) {
                    return;
                }

                sendChat2server();
                document.getElementById("inputPrompt").value = "";
            })
        }

        // bind input button
        document.getElementById("inputPromptBtn").addEventListener("click", (evt) => {
            evt.stopPropagation();
            sendChat2server();
            document.getElementById("inputPrompt").value = "";
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

        document.getElementById("chatConversation").insertAdjacentHTML("beforeend", chatEle);

        // chatEle = DOMParser.parseFromString(chatEle);
        // document.getElementById("chatConversation").insertAdjacentElement("beforeend", chatEle);
        // return chatEle
    }


    function setupConfig() {
        // set token type
        {
            switch (window.OpenaiTokenType()) {
                case "proxy":
                    ActiveElementsByData(document.getElementById("configTokenType").querySelectorAll("a.dropdown-item"), "value", "proxy");
                    break;
                case "direct":
                    ActiveElementsByData(document.getElementById("configTokenType").querySelectorAll("a.dropdown-item"), "value", "direct");
                    break;
            }

            // bind evt listener for configTokenType
            document.getElementById("configTokenType").querySelectorAll("a.dropdown-item").forEach((ele) => {
                ele.addEventListener("click", (evt) => {
                    evt.stopPropagation();
                    window.SetLocalStorage("config_api_token_type", evt.target.dataset.value);
                })
            });
        }

        //  config_api_token_value
        {
            document.getElementById("configAPIToken").value = window.OpenaiToken();
            document.getElementById("configAPIToken").addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_token_value", evt.target.value);
            })
        }

        //  config_api_max_tokens
        {
            document.getElementById("configMaxTokens").value = window.OpenaiMaxTokens();
            document.getElementById("configMaxTokens").addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_max_tokens", evt.target.value);
            })
        }

        //  config_api_static_context
        {
            document.getElementById("configStaticContext").value = window.OpenaiChatStaticContext();
            document.getElementById("configStaticContext").addEventListener("input", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage("config_api_static_context", evt.target.value);
            })
        }

        // bind reset button
        {
            document.getElementById("resetBtn").addEventListener("click", (evt) => {
                evt.stopPropagation();

                localStorage.clear();
                location.reload();
            })
        }
    }
})();
