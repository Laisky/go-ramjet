"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";


let chatContainer = document.getElementById("chatContainer"),
    configContainer = document.getElementById("hiddenChatConfigSideBar"),
    chatPromptInput = chatContainer.querySelector(".input.prompt"),
    chatPromptInputBtn = chatContainer.querySelector(".btn.send"),

    currentAIRespSSE, currentAIRespEle;

window.ready(() => {
    (function main() {
        // -------------------------------------
        // for compatibility
        updateChatHistory();
        // -------------------------------------

        setupLocalStorage();
        setupConfig();
        setupSessionManager();
        setupChatInput();
        setupPromptManager();
        setupPrivateDataset();
    })();
});


function newChatID() {
    return "chat-" + window.RandomString();
}

// show alert
//
// type: primary, secondary, success, danger, warning, info, light, dark
function showalert(type, msg) {
    let alertEle = `<div class="alert alert-${type} alert-dismissible" role="alert">
            <div>${msg}</div>
            <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
        </div>`;

    // append as first child
    chatContainer.querySelector(".chatManager")
        .insertAdjacentHTML("afterbegin", alertEle);
}

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
        append2Chats(item.role, item.content, true, item.chatID);
    });
}

function clearSessionAndChats(evt) {
    if (evt) {
        evt.stopPropagation();
    }

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
}

// update legacy chat history, add chatID to each chat
function updateChatHistory() {
    Object.keys(localStorage).forEach((key) => {
        if (!key.startsWith("chat_user_session_")) {
            return;
        }

        let sessionID = parseInt(key.replace("chat_user_session_", ""));

        let latestChatID,
            history = sessionChatHistory(sessionID);
        history.forEach((item) => {
            if (!item.chatID && item.role != RoleAI) {
                // compatability with old version,
                // that old version's history doesn't have chatID.
                item.chatID = newChatID();
            }

            if (item.role == RoleAI) {
                // compatability with old version,
                // some old version's AI history has different chatID with user's,
                // so we overwrite it with latestChatID.
                item.chatID = latestChatID;
            }

            latestChatID = item.chatID;
        });

        window.SetLocalStorage(key, history);
    });
}

function setupSessionManager() {
    // bind remove all sessions
    {
        chatContainer
            .querySelector(".sessionManager .btn.purge")
            .addEventListener("click", clearSessionAndChats);
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
            append2Chats(item.role, item.content, true, item.chatID);
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


// remove chat in storage by chatid
function removeChatInStorage(chatid) {
    if (!chatid) {
        throw "chatid is required";
    }

    let storageActiveSessionKey = storageSessionKey(activeSessionID()),
        history = activeSessionChatHistory();

    // remove all chats with the same chatid
    history = history.filter((item) => item.chatID !== chatid);

    window.localStorage.setItem(storageActiveSessionKey, JSON.stringify(history));
}


// append or update chat history by chatid and role
function appendChats2Storage(role, content, chatid) {
    if (!chatid) {
        throw "chatid is required";
    }

    let storageActiveSessionKey = storageSessionKey(activeSessionID()),
        history = activeSessionChatHistory();

    // if chat is already in history, find and update it.
    let found = false;
    history.forEach((item, idx) => {
        if (item.chatID == chatid && item.role == role) {
            found = true;
            item.content = content;
        }
    });

    // if ai response is not in history, add it after user's chat which has same chatid
    if (!found && role == RoleAI) {
        history.forEach((item, idx) => {
            if (item.chatID == chatid) {
                found = true;
                if (item.role != RoleAI) {
                    history.splice(idx + 1, 0, {
                        role: RoleAI,
                        content: content,
                        chatID: chatid,
                    });
                }
            }
        });
    }

    // if chat is not in history, add it
    if (!found) {
        history.push({
            role: role,
            content: content,
            chatID: chatid,
        });
    }

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
    if (GetLocalStorage(StorageKeySystemPrompt)) {
        messages = [{
            role: RoleSystem,
            content: GetLocalStorage(StorageKeySystemPrompt)
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


async function sendChat2Server(chatID) {
    let isReload = false,
        reqPromp;
    if (!chatID) { // if chatID is empty, it's a new request
        chatID = newChatID();
        reqPromp = window.TrimSpace(chatPromptInput.value || "");

        chatPromptInput.value = "";
        if (reqPromp == "") {
            return;
        }

        append2Chats(RoleHuman, reqPromp, false, chatID);
        appendChats2Storage(RoleHuman, reqPromp, chatID);
    } else { // if chatID is not empty, it's a reload request
        reqPromp = chatContainer
            .querySelector(`.chatManager .conservations #${chatID} .role-human .text-start pre`).innerHTML;
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
        case QAModelCustom:
        case QAModelBasebit:
        case QAModelSecurity:
            // {
            //     "question": "XFS 是干啥的",
            //     "text": " XFS is a simple CLI tool that can be used to create volumes/mounts and perform simple filesystem operations.\n",
            //     "url": "http://xego-dev.basebit.me/doc/xfs/support/xfs2_cli_instructions/"
            // }

            let url, project;
            switch (chatmodel) {
                case QAModelBasebit:
                case QAModelSecurity:
                    window.data['qa_chat_models'].forEach((item) => {
                        if (item['name'] == chatmodel) {
                            url = item['url'];
                            project = item['project'];
                        }
                    });

                    if (!project) {
                        console.error("can't find project name for chat model: " + chatmodel);
                        return;
                    }
                    break;
                case QAModelCustom:
                    url = "/ramjet/gptchat/ctx";
                    break
            }

            currentAIRespEle.scrollIntoView({ behavior: "smooth" });
            try {
                const resp = await fetch(`${url}?p=${project}&q=${encodeURIComponent(reqPromp)}`, {
                    method: "GET",
                    headers: {
                        "Content-Type": "application/json",
                        "Authorization": "Bearer " + window.OpenaiToken(),
                        "X-Authorization-Type": window.OpenaiTokenType(),
                        "X-PDFCHAT-PASSWORD": window.GetLocalStorage(StorageKeyCustomDatasetPassword)
                    },
                    cache: "no-cache"
                });

                if (resp.status != 200) {
                    throw new Error(`[${resp.status}]: ${await resp.text()}`);
                }

                let data = await resp.json();
                if (data && data.text) {
                    let rawHTMLResp = `${data.text}\n\n📖: \n\n${combineRefs(data.url)}`;
                    currentAIRespEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
                    appendChats2Storage(RoleAI, currentAIRespEle.innerHTML, chatID);
                }
            } catch (err) {
                abortAIResp(err);
                return;
            } finally {
                unlockChatInput();
            }

            return;
    }

    if (!currentAIRespSSE) {
        return;
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

            appendChats2Storage(RoleAI, currentAIRespEle.innerHTML, chatID);
            unlockChatInput();
        }
    });

    currentAIRespSSE.onerror = (err) => {
        abortAIResp(err);
    };
    currentAIRespSSE.stream();
}

function combineRefs(arr) {
    let markdown = "";
    for (const val of arr) {
        if (val.startsWith("https") || val.startsWith("http")) {
            markdown += `- <${val}>\n`;
        } else {  // sometimes refs are just plain text, not url
            markdown += `- \`${val}\`\n`;
        }
    }

    return markdown;
}

// parse langchain qa references to markdown links
function wrapRefLines(input) {
    const lines = input.split('\n');
    let result = '';
    for (let i = 0; i < lines.length; i++) {
        // skip empty lines
        if (lines[i].trim() == '') {
            continue;
        }

        result += `* <${lines[i]}>\n`;
    }
    return result;
}



// function replaceChatInStorage(role, chatID, content) {
//     let storageKey = storageSessionKey(activeSessionID()),
//         chats = window.GetLocalStorage(storageKey) || [];

//     chats.forEach((item) => {
//         if (item.chatID == chatID && item.role == role) {
//             item.content = content;
//         }
//     });

//     window.SetLocalStorage(storageKey, chats);
// }

function abortAIResp(err) {
    if (currentAIRespSSE) {
        currentAIRespSSE.close();
        currentAIRespSSE = null;
    }

    let errMsg;
    try {
        errMsg = JSON.parse(err.data);
    } catch (e) {
        errMsg = err.toString();
    }

    // if errMsg contains
    if (errMsg.includes("Access denied due to invalid subscription key or wrong API endpoint")) {
        showalert("danger", "API TOKEN invalid, please ask admin to get new token.\nAPI TOKEN 无效，请联系管理员获取新的 API TOKEN。");
    }

    if (currentAIRespEle.dataset.status == "waiting" || currentAIRespEle.dataset.status == "writing") {
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
            addEventListener("keydown", async (evt) => {
                evt.stopPropagation();
                if (evt.key != 'Enter'
                    || isComposition
                    || (evt.key == 'Enter' && !(evt.ctrlKey || evt.metaKey || evt.altKey || evt.shiftKey))
                    || !isAllowChatPrompInput()) {
                    return;
                }

                await sendChat2Server();
                chatPromptInput.value = "";
            })
    }

    // bind input button
    chatPromptInputBtn.
        addEventListener("click", async (evt) => {
            evt.stopPropagation();
            await sendChat2Server();
            chatPromptInput.value = "";
        })
}

// append chat to conservation container
//
// @param {string} role - RoleHuman/RoleSystem/RoleAI
// @param {string} text - chat text
// @param {boolean} isHistory - is history chat, default false. if true, will not append to storage
function append2Chats(role, text, isHistory = false, chatID) {
    let chatEle;

    if (chatID == undefined) {
        throw "chatID is required";
    }

    let reloadBtnHTML = `
            <div class="row d-flex align-items-center justify-content-center">
                <button class="btn btn-sm btn-outline-secondary reload" type="button">
                    <i class="bi bi-repeat"></i>
                    Reload</button>
            </div>`;

    let chatOp = "append";
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
                        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                            <div class="row">
                                <div class="col-1">🤖️</div>
                                <div class="col-10 text-start ai-response" data-status="waiting">
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
                    <div id="${chatID}">
                        <div class="container-fluid row role-human" data-chatid="${chatID}">
                            <div class="col-1">🤔️</div>
                            <div class="col-10 text-start"><pre>${text}</pre></div>
                            <div class="col-1">

                            <div class="col-1 d-flex justify-content-between">
                                <i class="bi bi-pencil-square"></i>
                                <i class="bi bi-trash"></i>
                            </div>
                        </div>
                        ${waitAI}
                    </div>`
            break
        case RoleAI:
            if (!isHistory) {
                chatOp = "replace";
                chatEle = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="row">
                            <div class="col-1">🤖️</div>
                            <div class="col-11 text-start ai-response" data-status="writing">${text}</div>
                        </div>
                        ${reloadBtnHTML}
                    </div>`
            } else {
                chatEle = `
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="col-1">🤖️</div>
                        <div class="col-11 text-start ai-response" data-status="writing">${text}</div>
                        ${reloadBtnHTML}
                    </div>`
            }

            break
    }

    console.log("append chat", role, chatOp, chatID);
    if (chatOp == "append") {
        if (role == RoleAI) {
            // ai response is always after human, so we need to find the last human chat,
            // and append ai response after it
            chatContainer.querySelector(`.chatManager .conservations #${chatID}`).
                insertAdjacentHTML("beforeend", chatEle);
        } else {
            chatContainer.querySelector(`.chatManager .conservations`).
                insertAdjacentHTML("beforeend", chatEle);
        }
    } else if (chatOp == "replace") {
        // replace html element of ai
        chatContainer.querySelector(`.chatManager .conservations #${chatID} .role-ai`).
            outerHTML = chatEle;
    }


    // avoid duplicate event listener, only bind event listener for new chat
    if (role == RoleHuman) {
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

        // bind delete button
        let deleteBtnHandler = (evt) => {
            evt.stopPropagation();
            let chatEle = chatContainer.querySelector(`#${chatID}`);
            chatEle.parentNode.removeChild(chatEle);
            removeChatInStorage(chatID);
        };

        let editHumanInputHandler = (evt) => {
            evt.stopPropagation();

            let oldText = chatContainer.querySelector(`#${chatID}`).innerHTML,
                text = chatContainer.querySelector(`#${chatID} .role-human .text-start pre`).innerHTML;

            chatContainer.querySelector(`#${chatID} .role-human`).innerHTML = `
                <textarea class="form-control" rows="3">${text}</textarea>
                <div class="btn-group" role="group">
                    <button class="btn btn-sm btn-outline-secondary save" type="button">
                        <i class="bi bi-check"></i>
                        Save</button>
                    <button class="btn btn-sm btn-outline-secondary cancel" type="button">
                        <i class="bi bi-x"></i>
                        Cancel</button>
                </div>`;

            let saveBtn = chatContainer.querySelector(`#${chatID} .role-human .btn.save`);
            let cancelBtn = chatContainer.querySelector(`#${chatID} .role-human .btn.cancel`);
            saveBtn.addEventListener("click", async (evt) => {
                evt.stopPropagation();
                let newText = chatContainer.querySelector(`#${chatID} .role-human textarea`).value;
                chatContainer.querySelector(`#${chatID}`).innerHTML = `
                        <div class="container-fluid row role-human" data-chatid="chat-93upb32o06e">
                            <div class="col-1">🤔️</div>
                            <div class="col-10 text-start"><pre>${newText}</pre></div>
                            <div class="col-1 d-flex justify-content-between">
                                <i class="bi bi-pencil-square"></i>
                                <i class="bi bi-trash"></i>
                            </div>
                        </div>
                        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="chat-93upb32o06e">
                            <div class="col-1">🤖️</div>
                            <div class="col-11 text-start ai-response" data-status="writing">
                                <p class="card-text placeholder-glow">
                                    <span class="placeholder col-7"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-6"></span>
                                    <span class="placeholder col-8"></span>
                                </p>
                            </div>
                        </div>
                    `;

                // bind delete and edit button
                chatContainer.querySelector(`#${chatID} .role-human .bi-trash`)
                    .addEventListener("click", deleteBtnHandler);
                chatContainer.querySelector(`#${chatID} .bi.bi-pencil-square`)
                    .addEventListener("click", editHumanInputHandler);

                await sendChat2Server(chatID);
                appendChats2Storage(RoleHuman, newText, chatID);
            });

            cancelBtn.addEventListener("click", (evt) => {
                evt.stopPropagation();
                chatContainer.querySelector(`#${chatID}`).innerHTML = oldText;

                // bind delete and edit button
                chatContainer.querySelector(`#${chatID} .role-human .bi-trash`)
                    .addEventListener("click", deleteBtnHandler);
                chatContainer.querySelector(`#${chatID} .bi.bi-pencil-square`)
                    .addEventListener("click", editHumanInputHandler);
            });
        };



        // bind delete and edit button
        chatContainer.querySelector(`#${chatID} .role-human .bi-trash`)
            .addEventListener("click", deleteBtnHandler);
        chatContainer.querySelector(`#${chatID} .bi.bi-pencil-square`)
            .addEventListener("click", editHumanInputHandler);
    }
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
            .querySelector(".system-prompt .input");
        staticConfigInput.value = window.OpenaiChatStaticContext();
        staticConfigInput.addEventListener("input", (evt) => {
            evt.stopPropagation();
            window.SetLocalStorage(StorageKeySystemPrompt, evt.target.value);
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

    // bind clear-chats button
    {
        configContainer.querySelector(".btn.clear-chats")
            .addEventListener("click", (evt) => {
                clearSessionAndChats(evt);
                location.reload();
            });
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
        // default prompts
        shortcuts = [
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
// @param {bool} storage - whether to save to localstorage
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

        window.deleteCheckCallback = () => {
            evt.target.parentElement.remove();

            // remove localstorage shortcut
            let shortcuts = window.GetLocalStorage(StorageKeyPromptShortCuts);
            shortcuts = shortcuts.filter((item) => item.title !== shortcut.title);
            window.SetLocalStorage(StorageKeyPromptShortCuts, shortcuts);
        }
        window.deleteCheckModal.show();
    });

    // add click event
    // replace system prompt
    ele.addEventListener("click", (evt) => {
        evt.stopPropagation();
        let promptInput = configContainer.querySelector(".system-prompt .input");
        window.SetLocalStorage(StorageKeySystemPrompt, evt.currentTarget.dataset.prompt);
        promptInput.value = evt.currentTarget.dataset.prompt;
    });

    // add to html
    promptShortcutContainer.appendChild(ele);
}

function setupPromptManager() {
    // restore shortcuts from localstorage
    {
        // bind default prompt shortcuts
        configContainer
            .querySelector(".prompt-shortcuts .badge")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                let promptInput = configContainer.querySelector(".system-prompt .input");
                promptInput.value = evt.target.dataset.prompt;
                window.SetLocalStorage(StorageKeySystemPrompt, evt.target.dataset.prompt);
            });

        let shortcuts = loadPromptShortcutsFromStorage();
        shortcuts.forEach((shortcut) => {
            appendPromptShortcut(shortcut, false);
        });
    }

    // bind star prompt
    let saveSystemPromptModelEle = document.querySelector("#save-system-prompt.modal"),
        saveSystemPromptModal = new bootstrap.Modal(saveSystemPromptModelEle);
    {
        configContainer
            .querySelector(".system-prompt .bi.save-prompt")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                let promptInput = configContainer
                    .querySelector(".system-prompt .input");

                saveSystemPromptModelEle
                    .querySelector(".modal-body textarea.user-input")
                    .innerHTML = promptInput.value;

                saveSystemPromptModal.show();
            });
    }

    // bind prompt market modal
    {
        configContainer
            .querySelector(".system-prompt .bi.open-prompt-market")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                let promptMarketModalEle = document.querySelector("#prompt-market.modal");
                let promptMarketModal = new bootstrap.Modal(promptMarketModalEle);
                promptMarketModal.show();
            });
    }

    // bind save button in system-prompt modal
    {
        saveSystemPromptModelEle
            .querySelector(".btn.save")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                let titleInput = saveSystemPromptModelEle
                    .querySelector(".modal-body input.title");
                let descriptionInput = saveSystemPromptModelEle
                    .querySelector(".modal-body textarea.user-input");

                // trim space
                titleInput.value = titleInput.value.trim();
                descriptionInput.value = descriptionInput.value.trim();

                // if title is empty, set input border to red
                if (titleInput.value === "") {
                    titleInput.classList.add("border-danger");
                    return;
                }

                let shortcut = {
                    title: titleInput.value,
                    description: descriptionInput.value
                };

                appendPromptShortcut(shortcut, true);


                // clear input
                titleInput.value = "";
                descriptionInput.value = "";
                titleInput.classList.remove("border-danger");
                saveSystemPromptModal.hide();
            });
    }

    // fill chat prompts market
    let promptMarketModal = document.querySelector("#prompt-market"),
        promptInput = promptMarketModal.querySelector("textarea.prompt-content"),
        promptTitle = promptMarketModal.querySelector("input.prompt-title");
    {
        window.chatPrompts.forEach((prompt) => {
            let ele = document.createElement("span");
            ele.classList.add("badge", "text-bg-info");
            ele.dataset.description = prompt.description;
            ele.dataset.title = prompt.title;
            ele.innerHTML = ` ${prompt.title}  <i class="bi bi-plus-circle"></i>`;

            // add click event
            // replace system prompt
            ele.addEventListener("click", (evt) => {
                evt.stopPropagation();

                promptInput.value = evt.currentTarget.dataset.description;
                promptTitle.value = evt.currentTarget.dataset.title;
            });

            promptMarketModal.querySelector(".prompt-labels").appendChild(ele);
        });
    }

    // bind chat prompts market add button
    {
        promptMarketModal.querySelector(".modal-body .save")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();

                // trim and check empty
                promptTitle.value = promptTitle.value.trim();
                promptInput.value = promptInput.value.trim();
                if (promptTitle.value === "") {
                    promptTitle.classList.add("border-danger");
                    return;
                }
                if (promptInput.value === "") {
                    promptInput.classList.add("border-danger");
                    return;
                }

                let shortcut = {
                    title: promptTitle.value,
                    description: promptInput.value
                };

                appendPromptShortcut(shortcut, true);

                promptTitle.value = "";
                promptInput.value = "";
                promptTitle.classList.remove("border-danger");
                promptInput.classList.remove("border-danger");
            });
    }
}

// setup private dataset modal
function setupPrivateDataset() {
    let pdfchatModalEle = document.querySelector("#modal-pdfchat");

    // bind header's custom qa button
    {
        // bind pdf-file modal
        let pdfFileModalEle = document.querySelector("#modal-pdfchat"),
            pdfFileModal = new bootstrap.Modal(pdfFileModalEle);


        document
            .querySelector('#headerbar .qa-models a[data-model="qa-custom"]')
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                pdfFileModal.show();
            });
    }

    // bind datakey to localstorage
    {
        let datakeyEle = pdfchatModalEle
            .querySelector('div[data-field="data-key"] input');

        datakeyEle.value = window.GetLocalStorage(StorageKeyCustomDatasetPassword);

        datakeyEle
            .addEventListener("change", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage(StorageKeyCustomDatasetPassword, evt.target.value);
            }
            );
    }

    // bind file upload
    {
        // when user choosen file, get file name of
        // pdfchatModalEle.querySelector('div[data-field="pdffile"] input').files[0]
        // and set to dataset-name input
        pdfchatModalEle
            .querySelector('div[data-field="pdffile"] input')
            .addEventListener("change", (evt) => {
                evt.stopPropagation();

                let filename = evt.target.files[0].name;

                // only accept .pdf
                if (!filename.endsWith(".pdf")) {
                    // remove choosen
                    pdfchatModalEle
                        .querySelector('div[data-field="pdffile"] input').value = "";

                    showalert("warning", "currently only support pdf file");
                    return;
                }

                // remove extension and non-ascii charactors
                filename = filename.substring(0, filename.lastIndexOf("."));
                filename = filename.replace(/[^a-zA-Z0-9]/g, "_");

                pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input')
                    .value = filename;
            });

        // bind upload button
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="upload"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();

                // build post form
                let form = new FormData();
                form.append("file", pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files[0]);
                form.append("file_key", pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input').value);
                form.append("data_key", pdfchatModalEle
                    .querySelector('div[data-field="data-key"] input').value);
                // and auth token to header
                let headers = new Headers();
                // headers.append("Content-Type", "multipart/form-data");
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);

                try {
                    window.ShowSpinner();
                    await fetch("/ramjet/gptchat/files", {
                        method: "POST",
                        headers: headers,
                        body: form
                    })

                    showalert("success", "upload dataset success, please wait few minutes to process");
                } catch (err) {
                    showalert("danger", "upload dataset failed");
                    throw err;
                } finally {
                    window.HideSpinner();
                }
            });
    }

    // bind list datasets
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="refresh"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();

                let headers = new Headers();
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("Cache-Control", "no-cache");
                headers.append("X-PDFCHAT-PASSWORD", window.GetLocalStorage(StorageKeyCustomDatasetPassword));

                let body;
                try {
                    window.ShowSpinner();
                    const resp = await fetch("/ramjet/gptchat/files", {
                        method: "GET",
                        headers: headers
                    })
                    body = await resp.json();
                } catch (err) {
                    showalert("danger", "fetch dataset failed");
                    throw err;
                } finally {
                    window.HideSpinner();
                }

                let datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let datasetsHTML = "";


                // add processing files
                // show processing files in grey and progress bar
                body.datasets.forEach((dataset) => {
                    switch (dataset.status) {
                        case "done":
                            datasetsHTML += `
                                <div class="d-flex justify-content-between align-items-center">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch" data-filename="${dataset.name}">
                                                <input class="form-check-input" type="checkbox">
                                                <label class="form-check-label" for="flexSwitchCheckChecked">${dataset.name}</label>
                                            </div>
                                        </div>
                                        <div class="col-5">
                                        </div>
                                        <div class="col-2 text-end">
                                            <i class="bi bi-trash"></i>
                                        </div>
                                    </div>
                                </div>`
                            break;
                        case "processing":
                            datasetsHTML += `
                                <div class="d-flex justify-content-between align-items-center">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch" data-filename="${dataset.name}">
                                                <input class="form-check-input" type="checkbox" disabled>
                                                <label class="form-check-label" for="flexSwitchCheckChecked">${dataset.name}</label>
                                            </div>
                                        </div>
                                        <div class="col-5">
                                            <div class="progress" role="progressbar" aria-label="Example with label" aria-valuenow="${dataset.progress}" aria-valuemin="0" aria-valuemax="100">
                                                <div class="progress-bar" style="width: ${dataset.progress}%">wait few minutes</div>
                                            </div>
                                        </div>
                                        <div class="col-2 text-end">
                                            <i class="bi bi-trash"></i>
                                        </div>
                                    </div>
                                </div>`
                            break;
                    }
                });

                datasetListEle.innerHTML = datasetsHTML;

                // selected binded datasets
                body.selected.forEach((dataset) => {
                    datasetListEle
                        .querySelector(`div[data-filename="${dataset}"] input`)
                        .checked = true;
                });
            });
    }

    // build context
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="build"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();

                let selectedDatasets = [];
                pdfchatModalEle.
                    querySelectorAll('div[data-field="dataset"] input[type="checkbox"]')
                    .forEach((ele) => {
                        if (ele.checked) {
                            selectedDatasets.push(ele.parentElement.getAttribute("data-filename"));
                        }
                    });

                if (selectedDatasets.length === 0) {
                    showalert("warning", "please select at least one dataset, click [List Dataset] button to fetch dataset list");
                    return;
                }

                let headers = new Headers();
                headers.append("Content-Type", "application/json");
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);

                try {
                    window.ShowSpinner();
                    await fetch("/ramjet/gptchat/ctx", {
                        method: "POST",
                        headers: headers,
                        body: JSON.stringify({
                            datasets: selectedDatasets,
                            data_key: pdfchatModalEle
                                .querySelector('div[data-field="data-key"] input').value
                        })
                    })

                    showalert("success", "build dataset success, you can chat now");
                } catch (err) {
                    showalert("danger", "build dataset failed");
                    throw err;
                } finally {
                    window.HideSpinner();
                }
            }
            );
    }
}
