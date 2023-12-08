"use strict";

const RoleHuman = "user",
    RoleSystem = "system",
    RoleAI = "assistant";


let chatContainer = document.getElementById("chatContainer"),
    configContainer = document.getElementById("hiddenChatConfigSideBar"),
    chatPromptInputEle = chatContainer.querySelector(".input.prompt"),
    chatPromptInputBtn = chatContainer.querySelector(".btn.send"),

    currentAIRespSSE, currentAIRespEle;

async function setupChatJs () {
    // -------------------------------------
    // for compatibility
    await updateChatHistory();
    // -------------------------------------

    setupConfig();
    await setupSessionManager();
    setupChatInput();
    setupPromptManager();
    setupPrivateDataset();
    window.setInterval(fetchImageDrawingResultBackground, 3000);
}


function newChatID() {
    return "chat-" + window.RandomString(16);
}

// show alert
//
// type: primary, secondary, success, danger, warning, info, light, dark
function showalert(type, msg) {
    let alertEle = `<div class="alert alert-${type} alert-dismissible" role="alert">
            <div>${sanitizeHTML(msg)}</div>
            <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
        </div>`;

    // append as first child
    chatContainer.querySelector(".chatManager")
        .insertAdjacentHTML("afterbegin", alertEle);
}


// check sessionID's type, secure convert to int, default is 1
function storageSessionKey(sessionID) {
    sessionID = parseInt(sessionID) || 1;
    return `${KvKeyPrefixSessionHistory}${sessionID}`;
}

async function sessionChatHistory(sessionID) {
    let data = (await window.KvGet(storageSessionKey(sessionID)));
    if (!data) {
        return [];
    }

    // fix legacy bug for marshal data twice
    if (typeof data == "string") {
        data = JSON.parse(data);
        await window.KvSet(storageSessionKey(sessionID), data);
    }

    return data;
}

async function activeSessionChatHistory() {
    let sid = activeSessionID();
    if (!sid) {
        return new Array;
    }

    return await sessionChatHistory(sid);
}

function activeSessionID() {
    let activeSession = chatContainer.querySelector(".sessionManager .card-body button.active");
    if (activeSession) {
        return activeSession.dataset.session;
    }

    return 1;
}

async function listenSessionSwitch(evt) {
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
    (await sessionChatHistory(sessionID)).forEach((item) => {
        append2Chats(item.chatID, item.role, item.content, true, item.attachHTML);
    });
}

/**
 * Fetches the image drawing result background for the AI response and displays it in the chat container.
 * @returns {Promise<void>}
 */
async function fetchImageDrawingResultBackground() {
    let elements = chatContainer
        .querySelectorAll('.role-ai .ai-response[data-task-type="image"][data-status="waiting"]') || [];


    await Promise.all(Array.from(elements).map(async (item) => {
        if (item.dataset.status != "waiting") {
            return;
        }

        const taskId = item.dataset.taskId,
            imageUrls = JSON.parse(item.dataset.imageUrls) || [],
            chatId = item.closest(".role-ai").dataset.chatid;

        try {
            await Promise.all(imageUrls.map(async (imageUrl) => {
                // check any err msg
                const errFileUrl = imageUrl.slice(0, imageUrl.lastIndexOf("-")) + ".err.txt";
                const errFileResp = await fetch(`${errFileUrl}?rr=${window.RandomString(12)}`, {
                    method: "GET",
                    cache: "no-cache",
                });
                if (errFileResp.ok || errFileResp.status == 200) {
                    const errText = await errFileResp.text();
                    item.insertAdjacentHTML("beforeend", `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${errText}</pre>`);
                    checkIsImageAllSubtaskDone(item, imageUrl, false);
                    return;
                }

                // check is image ready
                const imgResp = await fetch(`${imageUrl}?rr=${window.RandomString(12)}`, {
                    method: "GET",
                    cache: "no-cache",
                });
                if (!imgResp.ok || imgResp.status != 200) {
                    return;
                }

                // check is all tasks finished
                checkIsImageAllSubtaskDone(item, imageUrl, true);
            }));
        } catch (err) {
            console.warn("fetch img result, " + err);
        };
    }));
}

/** append chat to chat container
 *
 * @param {element} item ai respnse
 * @param {string} imageUrl current subtask's image url
 * @param {boolean} succeed is current subtask succeed
 */
function checkIsImageAllSubtaskDone(item, imageUrl, succeed) {
    let processingImageUrls = JSON.parse(item.dataset.imageUrls) || [];
    if (!processingImageUrls.includes(imageUrl)) {
        return;
    }

    // remove current subtask from imageUrls(tasks)
    processingImageUrls = processingImageUrls.filter((url) => url !== imageUrl);
    item.dataset.imageUrls = JSON.stringify(processingImageUrls);

    if (succeed) {
        let succeedImageUrls = JSON.parse(item.dataset.succeedImageUrls || "[]");
        succeedImageUrls.push(imageUrl);
        item.dataset.succeedImageUrls = JSON.stringify(succeedImageUrls);
    }

    if (processingImageUrls.length == 0) {
        item.dataset.status = "done";
        let imgHTML = "";
        let succeedImageUrls = JSON.parse(item.dataset.succeedImageUrls || "[]");
        succeedImageUrls.forEach((url) => {
            imgHTML += `<img src="${url}">`;
        });
        item.innerHTML = imgHTML;

        if (succeedImageUrls.length > 1) {
            item.classList.add("multi-images");
        }
    }
}

/**
 * Clears all user sessions and chats from local storage,
 * and resets the chat UI to its initial state.
 * @param {Event} evt - The event that triggered the function (optional).
 *
 * @returns {void}
 */
async function clearSessionAndChats(evt) {
    if (evt) {
        evt.stopPropagation();
    }

    let allkeys = await KvList();
    await Promise.all(allkeys.map(async (key) => {
        if (
            key.startsWith(KvKeyPrefixSessionHistory)  // remove all sessions
            || key == StorageKeyPinnedMaterials  // remove pinned materials
        ) {
            await KvDel(key);
        }
    }));

    chatContainer.querySelector(".chatManager .conservations").innerHTML = "";
    chatContainer.querySelector(".sessionManager .sessions").innerHTML = `
        <div class="list-group">
            <button type="button" class="list-group-item list-group-item-action session active" aria-current="true" data-session="1">
                <div>1</div>
                <div><i class="bi bi-trash"></i></div>
            </button>
        </div>`;
    chatContainer
        .querySelector(".sessionManager .sessions .session")
        .addEventListener("click", listenSessionSwitch);

    await window.KvSet(storageSessionKey(1), []);
    bindSessionDeleteBtn();
}

// update legacy chat history, add chatID to each chat
async function updateChatHistory() {
    await Promise.all(Object.keys(localStorage).map(async (key) => {
        if (!key.startsWith(KvKeyPrefixSessionHistory)) {
            return;
        }

        // move from localstorage to kv
        // console.log("move from localstorage to kv: ", key);
        await window.KvSet(key, JSON.parse(localStorage[key]));
        localStorage.removeItem(key);
    }));
}

function bindSessionDeleteBtn() {
    let btns = chatContainer.querySelectorAll(".sessionManager .sessions .session .bi-trash") || [];
    btns.forEach((item) => {
        if (item.dataset["bindClicked"]) {
            return;
        } else {
            item.dataset["bindClicked"] = true;
        }

        item.addEventListener("click", async (evt) => {
            evt.stopPropagation();

            // if there is only one session, don't delete it
            if (chatContainer.querySelectorAll(".sessionManager .sessions .session").length == 1) {
                return;
            }

            let sessionID = evt.target.closest(".session").dataset.session;
            if (confirm("Are you sure to delete this session?")) {
                window.KvDel(storageSessionKey(sessionID));
                evt.target.closest(".list-group").remove();
            }
        });
    });
}

/** setup session manager and restore current chat history
 *
 */
async function setupSessionManager() {
    // bind remove all sessions
    {
        chatContainer
            .querySelector(".sessionManager .btn.purge")
            .addEventListener("click", clearSessionAndChats);
    }


    // restore all sessions from localStorage
    {
        let anyHistorySession = false;
        let allKeys = await KvList();
        allKeys.forEach((key) => {
            if (!key.startsWith(KvKeyPrefixSessionHistory)) {
                return;
            }

            anyHistorySession = true;
        })

        if (!anyHistorySession) {
            await window.KvSet(storageSessionKey(1), []);
        }

        let firstSession = true;
        allKeys.forEach((key) => {
            if (!key.startsWith(KvKeyPrefixSessionHistory)) {
                return;
            }

            anyHistorySession = true;
            let sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ""));

            let active = "";
            if (firstSession) {
                firstSession = false;
                active = "active";
            }

            chatContainer
                .querySelector(".sessionManager .sessions")
                .insertAdjacentHTML(
                    "beforeend",
                    `<div class="list-group">
                        <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                            <div>${sessionID}</div>
                            <div><i class="bi bi-trash"></i></div>
                        </button>
                    </div>`);
        });

        // restore conservation history
        let data = await activeSessionChatHistory();
        (await activeSessionChatHistory()).forEach((item) => {
            append2Chats(item.chatID, item.role, item.content, true, item.attachHTML);
        });

        window.EnableTooltipsEverywhere();
    }

    // add widget to scroll bottom
    {
        document.querySelector("#chatContainer .chatManager .card-footer .scroll-down")
            .addEventListener("click", (evt) => {
                evt.stopPropagation();
                scrollChatToDown();
            });
    }

    // new session
    {
        chatContainer
            .querySelector(".sessionManager .btn.new-session")
            .addEventListener("click", async (evt) => {
                let maxSessionID = 0;
                (await KvList()).forEach((key) => {
                    if (key.startsWith(KvKeyPrefixSessionHistory)) {
                        let sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ""));
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
                        `<div class="list-group">
                            <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                <div>${newSessionID}</div>
                                <div><i class="bi bi-trash"></i></div>
                            </button>
                        </div>`);
                await window.KvSet(storageSessionKey(newSessionID), []);

                // bind session switch listener for new session
                chatContainer
                    .querySelector(`.sessionManager .sessions [data-session="${newSessionID}"]`)
                    .addEventListener("click", listenSessionSwitch);

                bindSessionDeleteBtn();
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

    bindSessionDeleteBtn();
}


// remove chat in storage by chatid
async function removeChatInStorage(chatid) {
    if (!chatid) {
        throw "chatid is required";
    }

    let storageActiveSessionKey = storageSessionKey(activeSessionID()),
        history = await activeSessionChatHistory();

    // remove all chats with the same chatid
    history = history.filter((item) => item.chatID !== chatid);

    await window.KvSet(storageActiveSessionKey, JSON.stringify(history));
}


/** append or update chat history by chatid and role
    * @param {string} chatid - chat id
    * @param {string} role - user or assistant
    * @param {string} content - chat content
    * @param {string} attachHTML - chat content's attach html
*/
async function appendChats2Storage(role, chatid, content, attachHTML) {
    if (!chatid) {
        throw "chatid is required";
    }

    let storageActiveSessionKey = storageSessionKey(activeSessionID()),
        history = await activeSessionChatHistory();

    // if chat is already in history, find and update it.
    let found = false;
    history.forEach((item, idx) => {
        if (item.chatID == chatid && item.role == role) {
            found = true;
            item.content = content;
            item.attachHTML = attachHTML;
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
                        chatID: chatid,
                        content: content,
                        attachHTML: attachHTML,
                    });
                }
            }
        });
    }

    // if chat is not in history, add it
    if (!found) {
        history.push({
            role: role,
            chatID: chatid,
            content: content,
            attachHTML: attachHTML,
        });
    }

    await window.KvSet(storageActiveSessionKey, history);
}


function scrollChatToDown() {
    window.ScrollDown(chatContainer.querySelector(".chatManager .conservations"));
}

function scrollToChat(chatEle) {
    chatEle.scrollIntoView({ behavior: 'smooth', block: 'end' });
}

/**
*
* Get the last N chat messages, which will be sent to the AI as context.
*
* @param {number} N - The number of messages to retrieve.
* @param {string} ignoredChatID - If ignoredChatID is not null, the chat with this chatid will be ignored.
* @returns {Array} An array of chat messages.
*/
async function getLastNChatMessages(N, ignoredChatID) {
    let messages = (await activeSessionChatHistory()).filter((ele) => {
        if (ele.role != RoleHuman) {
            // Ignore AI's chat, only use human's chat as context.
            return false;
        };

        if (ignoredChatID && ignoredChatID == ele.chatID) {
            // This is a reload request with edited chat,
            // ignore chat with same chatid to avoid duplicate context.
            return false;
        }

        return true;
    });

    if (N == 0) {
        messages = [];
    } else {
        messages = messages.slice(-N);
    }

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
}
function unlockChatInput() {
    chatPromptInputBtn.classList.remove("disabled");
}
function isAllowChatPrompInput() {
    return !chatPromptInputBtn.classList.contains("disabled");
}

function parseChatResp(chatmodel, payload) {
    if (window.IsChatModel(chatmodel) || window.IsQaModel(chatmodel)) {
        return payload.choices[0].delta.content || "";
    } else if (window.IsCompletionModel(chatmodel)) {
        return payload.choices[0].text || "";
    } else {
        showalert("error", `Unknown chat model ${chatmodel}`);
    }
}

const httpsRegexp = /\bhttps:\/\/\S+/;

/**
 * extract https urls from reqPrompt and pin them to the chat conservation window
 *
 * @param {string} reqPrompt - request prompt
 * @returns {string} modified request prompt
 */
function pinNewMaterial(reqPrompt) {
    let pinnedUrls = getPinnedMaterials();
    let urls = reqPrompt.match(httpsRegexp)
    if (urls) {
        urls.forEach((url) => {
            if (!pinnedUrls.includes(url)) {
                pinnedUrls.push(url);
            }
        });
    }

    let urlEle = "";
    for (let url of pinnedUrls) {
        urlEle += `<p><i class="bi bi-trash"></i> <a href="${url}" class="link-primary" target="_blank">${url}</a></p>`;
    }

    // save to storage
    window.SetLocalStorage(StorageKeyPinnedMaterials, urlEle)
    restorePinnedMaterials();

    // re generate reqPrompt
    reqPrompt = reqPrompt.replace(httpsRegexp, "");
    reqPrompt += "\n" + pinnedUrls.join("\n");
    return reqPrompt;
}

function restorePinnedMaterials() {
    let urlEle = window.GetLocalStorage(StorageKeyPinnedMaterials) || "";
    let container = document.querySelector("#chatContainer .pinned-refs");
    container.innerHTML = urlEle;

    // bind to remove pinned materials
    document.querySelectorAll("#chatContainer .pinned-refs p .bi-trash")
        .forEach((item) => {
            item.addEventListener("click", (evt) => {
                evt.stopPropagation();
                let container = evt.target.closest(".pinned-refs")
                let ele = evt.target.closest("p");
                ele.parentNode.removeChild(ele);

                // update storage
                window.SetLocalStorage(StorageKeyPinnedMaterials, container.innerHTML);
            });
        });
}

function getPinnedMaterials() {
    let urls = [];
    document.querySelectorAll("#chatContainer .pinned-refs a")
        .forEach((item) => {
            urls.push(item.innerHTML);
        });

    return urls;
}

/**
 * Sends an txt2image prompt to the server for the selected model and updates the current AI response element with the task information.
 * @param {string} chatID - The chat ID.
 * @param {string} selectedModel - The selected image model.
 * @param {HTMLElement} currentAIRespEle - The current AI response element to update with the task information.
 * @param {string} prompt - The image prompt to send to the server.
 * @throws {Error} Throws an error if the selected model is unknown or if the response from the server is not ok.
 */
async function sendTxt2ImagePrompt2Server(chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
        case ImageModelDalle2:
            url = `/images/generations`;
            break;
        default:
            throw new Error(`unknown image model: ${selectedModel}`);
    }


    const resp = await fetch(url, {
        method: "POST",
        headers: {
            "Authorization": "Bearer " + window.OpenaiToken(),
            "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
        },
        body: JSON.stringify({
            model: selectedModel,
            prompt: prompt
        })
    });
    if (!resp.ok || resp.status != 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = "waiting";
    currentAIRespEle.dataset.taskType = "image";
    currentAIRespEle.dataset.taskId = respData["task_id"];
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData["image_urls"]);

    let attachHTML = "";
    respData["image_urls"].forEach((url) => {
        attachHTML += `<img src="${url}">`;
    });

    // save img to storage no matter it's done or not
    await appendChats2Storage(RoleAI, chatID, attachHTML);
}

async function sendSdxlturboPrompt2Server(chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
        case ImageModelSdxlTurbo:
            url = `/images/generations/sdxl-turbo`;
            break;
        default:
            throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    let imageBase64 = "";
    if (Object.keys(chatVisionSelectedFileStore).length != 0) {
        imageBase64 = Object.values(chatVisionSelectedFileStore)[0];

        // insert image to user input & hisotry
        await appendImg2UserInput(chatID, imageBase64, `${DateStr()}.png`);

        chatVisionSelectedFileStore = {};
        updateChatVisionSelectedFileStore();
    }


    const resp = await fetch(url, {
        method: "POST",
        headers: {
            "Authorization": "Bearer " + window.OpenaiToken(),
            "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
        },
        body: JSON.stringify({
            model: selectedModel,
            text: prompt,
            image: imageBase64
        })
    });
    if (!resp.ok || resp.status != 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = "waiting";
    currentAIRespEle.dataset.taskType = "image";
    currentAIRespEle.dataset.taskId = respData["task_id"];
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData["image_urls"]);

    // save img to storage no matter it's done or not
    let attachHTML = "";
    respData["image_urls"].forEach((url) => {
        attachHTML += `<img src="${url}">`;
    });

    await appendChats2Storage(RoleAI, chatID, attachHTML);
}


async function sendImg2ImgPrompt2Server(chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
        case ImageModelImg2Img:
            url = `/images/generations/lcm`;
            break;
        default:
            throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    if (Object.keys(chatVisionSelectedFileStore).length == 0) {
        throw new Error("no image selected");
    }
    const imageBase64 = Object.values(chatVisionSelectedFileStore)[0];

    // insert image to user input & hisotry
    await appendImg2UserInput(chatID, imageBase64, `${DateStr()}.png`);

    chatVisionSelectedFileStore = {};
    updateChatVisionSelectedFileStore();

    const resp = await fetch(url, {
        method: "POST",
        headers: {
            "Authorization": "Bearer " + window.OpenaiToken(),
            "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
        },
        body: JSON.stringify({
            model: selectedModel,
            prompt: prompt,
            image_base64: imageBase64
        })
    });
    if (!resp.ok || resp.status != 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = "waiting";
    currentAIRespEle.dataset.taskType = "image";
    currentAIRespEle.dataset.taskId = respData["task_id"];
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData["image_urls"]);

    // save img to storage no matter it's done or not
    let attachHTML = "";
    respData["image_urls"].forEach((url) => {
        attachHTML += `<img src="${url}">`;
    });

    await appendChats2Storage(RoleAI, chatID, attachHTML);
}

async function appendImg2UserInput(chatID, imgDataBase64, imgName) {
    // insert image to user hisotry
    let text = chatContainer
        .querySelector(`.chatManager .conservations #${chatID} .role-human .text-start pre`).innerHTML;
    await appendChats2Storage(RoleHuman, chatID, text,
        `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`,
    );

    // insert image to user input
    chatContainer
        .querySelector(`.chatManager .conservations #${chatID} .role-human .text-start`)
        .insertAdjacentHTML(
            "beforeend",
            `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`,
        );
}

async function sendChat2Server(chatID) {
    let reqPrompt;
    if (!chatID) { // if chatID is empty, it's a new request
        chatID = newChatID();
        reqPrompt = window.TrimSpace(chatPromptInputEle.value || "");

        chatPromptInputEle.value = "";
        if (reqPrompt == "") {
            return;
        }

        append2Chats(chatID, RoleHuman, reqPrompt, false);
        await appendChats2Storage(RoleHuman, chatID, reqPrompt);
    } else { // if chatID is not empty, it's a reload request
        reqPrompt = chatContainer
            .querySelector(`.chatManager .conservations #${chatID} .role-human .text-start pre`).innerHTML;
    }

    // extract and pin new material in chat
    reqPrompt = pinNewMaterial(reqPrompt);

    currentAIRespEle = chatContainer
        .querySelector(`.chatManager .conservations #${chatID} .ai-response`);
    currentAIRespEle = currentAIRespEle;
    lockChatInput();

    let selectedModel = (GetLocalStorage("config_chat_model") || ChatModelTurbo35);
    // get chatmodel from url parameters
    if (window.location.search) {
        let params = new URLSearchParams(window.location.search);
        if (params.has("chatmodel")) {
            selectedModel = params.get("chatmodel");
        }
    }

    // these extras will append to the tail of AI's response
    let responseExtras = "";

    if (window.IsChatModel(selectedModel)) {
        let messages,
            nContexts = parseInt(window.ChatNContexts());

        if (chatID) {  // reload current chat by latest context
            messages = await getLastNChatMessages(nContexts - 1, chatID);
            messages.push({
                role: RoleHuman,
                content: reqPrompt
            });
        } else {
            messages = await getLastNChatMessages(nContexts, chatID);
        }

        // there are pinned files, add them to user's prompt
        if (Object.keys(chatVisionSelectedFileStore).length != 0) {
            if (!selectedModel.includes("vision")) {
                // if selected model is not vision model, just ignore it
                chatVisionSelectedFileStore = {};
                updateChatVisionSelectedFileStore();
                return
            }

            messages[messages.length - 1].files = [];
            for (let key in chatVisionSelectedFileStore) {
                messages[messages.length - 1].files.push({
                    type: "image",
                    name: key,
                    content: chatVisionSelectedFileStore[key]
                });

                // insert image to user input & hisotry
                await appendImg2UserInput(chatID, chatVisionSelectedFileStore[key], key);
            }

            chatVisionSelectedFileStore = {};
            updateChatVisionSelectedFileStore();
        }

        currentAIRespSSE = new SSE(window.OpenaiAPI(), {
            headers: {
                "Content-Type": "application/json",
                "Authorization": "Bearer " + window.OpenaiToken(),
                "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
                "X-Laisky-Authorization-Type": window.OpenaiTokenType(),
            },
            method: "POST",
            payload: JSON.stringify({
                model: selectedModel,
                stream: true,
                max_tokens: parseInt(window.OpenaiMaxTokens()),
                temperature: parseFloat(window.OpenaiTemperature()),
                presence_penalty: parseFloat(window.OpenaiPresencePenalty()),
                frequency_penalty: parseFloat(window.OpenaiFrequencyPenalty()),
                messages: messages,
                stop: ["\n\n"]
            })
        });
    } else if (window.IsCompletionModel(selectedModel)) {
        currentAIRespSSE = new SSE(window.OpenaiAPI(), {
            headers: {
                "Content-Type": "application/json",
                "Authorization": "Bearer " + window.OpenaiToken(),
                "X-Laisky-Authorization-Type": window.OpenaiTokenType(),
                "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
            },
            method: "POST",
            payload: JSON.stringify({
                model: selectedModel,
                stream: true,
                max_tokens: parseInt(window.OpenaiMaxTokens()),
                temperature: parseFloat(window.OpenaiTemperature()),
                presence_penalty: parseFloat(window.OpenaiPresencePenalty()),
                frequency_penalty: parseFloat(window.OpenaiFrequencyPenalty()),
                prompt: reqPrompt,
                stop: ["\n\n"]
            })
        });
    } else if (window.IsQaModel(selectedModel)) {
        // {
        //     "question": "XFS 是干啥的",
        //     "text": " XFS is a simple CLI tool that can be used to create volumes/mounts and perform simple filesystem operations.\n",
        //     "url": "http://xego-dev.basebit.me/doc/xfs/support/xfs2_cli_instructions/"
        // }

        let url, project;
        switch (selectedModel) {
            case QAModelBasebit:
            case QAModelSecurity:
            case QAModelImmigrate:
                window.data['qa_chat_models'].forEach((item) => {
                    if (item['name'] == selectedModel) {
                        url = item['url'];
                        project = item['project'];
                    }
                });

                if (!project) {
                    console.error("can't find project name for chat model: " + selectedModel);
                    return;
                }

                url = `${url}?p=${project}&q=${encodeURIComponent(reqPrompt)}`;
                break;
            case QAModelCustom:
                url = `/ramjet/gptchat/ctx/search?q=${encodeURIComponent(reqPrompt)}`;
                break
            case QAModelShared:
                // example url:
                //
                // https://chat2.laisky.com/?chatmodel=qa-shared&uid=public&chatbot_name=default

                let params = new URLSearchParams(window.location.search);
                url = `/ramjet/gptchat/ctx/share?uid=${params.get("uid")}`
                    + `&chatbot_name=${params.get("chatbot_name")}`
                    + `&q=${encodeURIComponent(reqPrompt)}`;
                break;
            default:
                console.error("unknown qa chat model: " + selectedModel);
        }

        currentAIRespEle.scrollIntoView({ behavior: "smooth" });
        try {
            const resp = await fetch(url, {
                method: "GET",
                cache: "no-cache",
                headers: {
                    "Connection": "keep-alive",
                    "Content-Type": "application/json",
                    "Authorization": "Bearer " + window.OpenaiToken(),
                    "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
                    "X-Laisky-Authorization-Type": window.OpenaiTokenType(),
                    "X-PDFCHAT-PASSWORD": window.GetLocalStorage(StorageKeyCustomDatasetPassword)
                },
            });

            if (!resp.ok || resp.status != 200) {
                throw new Error(`[${resp.status}]: ${await resp.text()}`);
            }

            let data = await resp.json();
            if (data && data.text) {
                responseExtras = `
                    <p style="margin-bottom: 0;">
                        <button class="btn btn-info" type="button" data-bs-toggle="collapse" data-bs-target="#chatRef-${chatID}" aria-expanded="false" aria-controls="chatRef-${chatID}" style="font-size: 0.6em">
                            > toggle reference
                        </button>
                    </p>
                    <div>
                        <div class="collapse" id="chatRef-${chatID}">
                            <div class="card card-body">${combineRefs(data.url)}</div>
                        </div>
                    </div>`;
                let messages = [{
                    role: RoleHuman,
                    content: `Use the following pieces of context to answer the users question.
                    the context that help you answer the question is between ">>>>>>>" and "<<<<<<<",
                    the user' question that you should answer in after "<<<<<<<".
                    you should directly answer the user's question, and you can use the context to help you answer the question.

                    >>>>>>>
                    context: ${data.text}
                    <<<<<<<

                    question: ${reqPrompt}
                    `
                }];
                let model = ChatModelTurbo35;  // rewrite chat model
                if (window.IsChatModelAllowed(ChatModelTurbo35_16K) && !window.OpenaiToken().startsWith("FREETIER-")) {
                    model = ChatModelTurbo35_16K;
                }

                currentAIRespSSE = new SSE(window.OpenaiAPI(), {
                    headers: {
                        "Content-Type": "application/json",
                        "Authorization": "Bearer " + window.OpenaiToken(),
                        "X-Laisky-User-Id": await window.getSHA1(window.OpenaiToken()),
                        "X-Laisky-Authorization-Type": window.OpenaiTokenType(),
                    },
                    method: "POST",
                    payload: JSON.stringify({
                        model: model,
                        stream: true,
                        max_tokens: parseInt(window.OpenaiMaxTokens()),
                        temperature: parseFloat(window.OpenaiTemperature()),
                        presence_penalty: parseFloat(window.OpenaiPresencePenalty()),
                        frequency_penalty: parseFloat(window.OpenaiFrequencyPenalty()),
                        messages: messages,
                        stop: ["\n\n"]
                    })
                });
            }
        } catch (err) {
            abortAIResp(err);
            return;
        }
    } else if (window.IsImageModel(selectedModel)) {
        try {
            switch (selectedModel) {
                case ImageModelDalle2:
                    await sendTxt2ImagePrompt2Server(chatID, selectedModel, currentAIRespEle, reqPrompt);
                    break
                case ImageModelImg2Img:
                    await sendImg2ImgPrompt2Server(chatID, selectedModel, currentAIRespEle, reqPrompt);
                    break
                case ImageModelSdxlTurbo:
                    await sendSdxlturboPrompt2Server(chatID, selectedModel, currentAIRespEle, reqPrompt);
                    break
                default:
                    throw new Error(`unknown image model: ${selectedModel}`);
            }
        } catch (err) {
            abortAIResp(err);
            return;
        } finally {
            unlockChatInput();
        }
    } else {
        currentAIRespEle.innerHTML = `<p>🔥Someting in trouble...</p>`
            + `<pre style="background-color: #f8e8e8; text-wrap: pretty;">`
            + `unimplemented model: ${sanitizeHTML(selectedModel)}</pre>`;
        unlockChatInput();
        return;
    }

    if (!currentAIRespSSE) {
        return;
    }

    let rawHTMLResp = "";
    currentAIRespSSE.addEventListener("message", async (evt) => {
        evt.stopPropagation();

        let isChatRespDone = false;
        if (evt.data == "[DONE]") {
            isChatRespDone = true
        }

        if (!isChatRespDone) {
            let payload = JSON.parse(evt.data),
                respContent = parseChatResp(selectedModel, payload);

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

                    scrollToChat(currentAIRespEle);
                    break
            }
        }

        if (isChatRespDone) {
            currentAIRespSSE.close();
            currentAIRespSSE = null;

            let markdownConverter = new window.showdown.Converter();
            currentAIRespEle.innerHTML = window.Markdown2HTML(rawHTMLResp);
            currentAIRespEle.innerHTML += responseExtras;

            Prism.highlightAll();
            window.EnableTooltipsEverywhere();

            scrollToChat(currentAIRespEle);
            await appendChats2Storage(RoleAI, chatID, currentAIRespEle.innerHTML);
            unlockChatInput();
        }
    });

    currentAIRespSSE.onerror = (err) => {
        // abortAIResp(new Error("SSE error: " + err));
        abortAIResp(err);
    };
    currentAIRespSSE.stream();
}

function combineRefs(arr) {
    let markdown = "";
    for (const val of arr) {
        if (val.startsWith("https") || val.startsWith("http")) {
            // markdown += `- <${val}>\n`;
            markdown += `<li><a href="${val}">${decodeURIComponent(val)}</li>`
        } else {  // sometimes refs are just plain text, not url
            // markdown += `- \`${val}\`\n`;
            markdown += `<li><p>${val}</p></li>`
        }
    }

    return `<ul style="margin-bottom: 0;">${markdown}</ul>`;
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
    console.error(`abort AI resp: ${err}`);
    if (currentAIRespSSE) {
        currentAIRespSSE.close();
        currentAIRespSSE = null;
    }

    let errMsg;
    if (err.data) {
        try {
            errMsg = JSON.parse(err.data);
        } catch (e) {
            errMsg = err.data;
        }
    } else {
        errMsg = err.toString();
    }

    if (errMsg == "[object CustomEvent]" && navigator.userAgent.includes("Firefox")) {
        // firefox will throw this error when SSE is closed, just ignore it.
        return;
    }

    // if errMsg contains
    if (errMsg.includes("Access denied due to invalid subscription key or wrong API endpoint")) {
        showalert("danger", "API TOKEN invalid, please ask admin to get new token.\nAPI TOKEN 无效，请联系管理员获取新的 API TOKEN。");
    }

    if (currentAIRespEle.dataset.status == "waiting") {// || currentAIRespEle.dataset.status == "writing") {
        currentAIRespEle.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${window.RenderStr2HTML(errMsg)}</pre>`;
    } else {
        currentAIRespEle.innerHTML += `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${window.RenderStr2HTML(errMsg)}</pre>`;
    }

    // window.ScrollDown(chatContainer.querySelector(".chatManager .conservations"));
    currentAIRespEle.scrollIntoView({ behavior: "smooth" });
    unlockChatInput();
}


function setupChatInput() {
    // bind input press enter
    {
        let isComposition = false;
        chatPromptInputEle.
            addEventListener("compositionstart", (evt) => {
                evt.stopPropagation();
                isComposition = true;
            })
        chatPromptInputEle.
            addEventListener("compositionend", (evt) => {
                evt.stopPropagation();
                isComposition = false;
            })


        chatPromptInputEle.
            addEventListener("keydown", async (evt) => {
                evt.stopPropagation();
                if (evt.key != 'Enter'
                    || isComposition
                    || (evt.key == 'Enter' && !(evt.ctrlKey || evt.metaKey || evt.altKey || evt.shiftKey))
                    || !isAllowChatPrompInput()) {
                    return;
                }

                await sendChat2Server();
                chatPromptInputEle.value = "";
            })
    }

    // bind input button
    chatPromptInputBtn.
        addEventListener("click", async (evt) => {
            evt.stopPropagation();
            await sendChat2Server();
            chatPromptInputEle.value = "";
        })

    // restore pinned materials
    restorePinnedMaterials();

    // bind input element's drag-drop
    {


        let dropfileModalEle = document.querySelector("#modal-dropfile.modal"),
            dropfileModal = new bootstrap.Modal(dropfileModalEle);

        let fileDragLeave = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();
        }

        let fileDragDropHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();

            if (!evt.dataTransfer || !evt.dataTransfer.items) {
                return;
            }

            for (let i = 0; i < evt.dataTransfer.items.length; i++) {
                let item = evt.dataTransfer.items[i];
                if (item.kind != "file") {
                    continue;
                }

                let file = item.getAsFile();
                if (!file) {
                    continue;
                }

                // get file content as Blob
                let reader = new FileReader();
                reader.onload = async (e) => {
                    let arrayBuffer = e.target.result;
                    if (arrayBuffer.byteLength > 1024 * 1024 * 10) {
                        showalert("danger", "file size should less than 10M");
                        return;
                    }

                    let byteArray = new Uint8Array(arrayBuffer);
                    let chunkSize = 0xffff; // Use chunks to avoid call stack limit
                    let chunks = [];
                    for (let i = 0; i < byteArray.length; i += chunkSize) {
                        chunks.push(String.fromCharCode.apply(null, byteArray.subarray(i, i + chunkSize)));
                    }
                    let base64String = btoa(chunks.join(''));

                    // only support 1 image for current version
                    chatVisionSelectedFileStore = {};
                    chatVisionSelectedFileStore[file.name] = base64String;
                    updateChatVisionSelectedFileStore();
                };
                reader.readAsArrayBuffer(file);
            }
        }

        let fileDragOverHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            evt.dataTransfer.dropEffect = 'copy'; // Explicitly show this is a copy.
            dropfileModal.show();
        }

        // read paste file
        let filePasteHandler = async (evt) => {
            if (!evt.clipboardData || !evt.clipboardData.items) {
                return;
            }

            for (let i = 0; i < evt.clipboardData.items.length; i++) {
                let item = evt.clipboardData.items[i];
                if (item.kind != "file") {
                    continue;
                }

                let file = item.getAsFile();
                if (!file) {
                    continue;
                }

                evt.stopPropagation();
                evt.preventDefault();

                // get file content as Blob
                let reader = new FileReader();
                reader.onload = async (e) => {
                    let arrayBuffer = e.target.result;
                    if (arrayBuffer.byteLength > 1024 * 1024 * 10) {
                        showalert("danger", "file size should less than 10M");
                        return;
                    }

                    let byteArray = new Uint8Array(arrayBuffer);
                    let chunkSize = 0xffff; // Use chunks to avoid call stack limit
                    let chunks = [];
                    for (let i = 0; i < byteArray.length; i += chunkSize) {
                        chunks.push(String.fromCharCode.apply(null, byteArray.subarray(i, i + chunkSize)));
                    }
                    let base64String = btoa(chunks.join(''));

                    // only support 1 image for current version
                    chatVisionSelectedFileStore = {};
                    chatVisionSelectedFileStore[file.name] = base64String;
                    updateChatVisionSelectedFileStore();
                };
                reader.readAsArrayBuffer(file);
            }
        }

        chatPromptInputEle.addEventListener("paste", filePasteHandler);

        document.body.addEventListener("dragover", fileDragOverHandler);
        document.body.addEventListener("drop", fileDragDropHandler);
        document.body.addEventListener("paste", filePasteHandler);

        dropfileModalEle.addEventListener("drop", fileDragDropHandler);
        dropfileModalEle.addEventListener("dragleave", fileDragLeave);
    }
}

// map[filename]fileContent_in_base64
//
// should invoke updateChatVisionSelectedFileStore after update this object
var chatVisionSelectedFileStore = {};

async function updateChatVisionSelectedFileStore() {
    let pinnedFiles = chatContainer.querySelector(".pinned-files");
    pinnedFiles.innerHTML = "";
    for (let key in chatVisionSelectedFileStore) {
        pinnedFiles.insertAdjacentHTML("beforeend", `<p data-key="${key}"><i class="bi bi-trash"></i> ${key}</p>`);
    }

    // click to remove pinned file
    chatContainer.querySelectorAll(".pinned-files .bi.bi-trash")
        .forEach((item) => {
            item.addEventListener("click", (evt) => {
                evt.stopPropagation();
                let ele = evt.target.closest("p");
                let key = ele.dataset.key;
                delete chatVisionSelectedFileStore[key];
                ele.parentNode.removeChild(ele);
            });
        });
}


/**
 * Append chat to conservation container
 *
 * @param {string} chatID - chat id
 * @param {string} role - RoleHuman/RoleSystem/RoleAI
 * @param {string} text - chat text
 * @param {boolean} isHistory - is history chat, default false. if true, will not append to storage
 * @param {string} attachHTML - html to attach to chat
 */
async function append2Chats(chatID, role, text, isHistory = false, attachHTML) {
    if (!chatID) {
        throw "chatID is required";
    }

    const robot_icon = "🤖️";
    let chatEleHtml,
        chatOp = "append";
    switch (role) {
        case RoleSystem:
            text = window.escapeHtml(text);

            chatEleHtml = `
            <div class="container-fluid row role-human">
                <div class="col-auto icon">💻</div>
                <div class="col text-start"><pre>${text}</pre></div>
            </div>`
            break
        case RoleHuman:
            text = window.escapeHtml(text);

            let waitAI = "";
            if (!isHistory) {
                waitAI = `
                        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                            <div class="col-auto icon">${robot_icon}</div>
                            <div class="col text-start ai-response" data-status="waiting">
                                <p dir="auto" class="card-text placeholder-glow">
                                    <span class="placeholder col-7"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-6"></span>
                                    <span class="placeholder col-8"></span>
                                </p>
                            </div>
                        </div>`
            }

            if (attachHTML) {
                attachHTML = `${attachHTML}`;
            } else {
                attachHTML = "";
            }

            chatEleHtml = `
                <div id="${chatID}">
                    <div class="container-fluid row role-human" data-chatid="${chatID}">
                        <div class="col-auto icon">🤔️</div>
                        <div class="col text-start">
                            <pre>${text}</pre>
                            ${attachHTML}
                        </div>
                        <div class="col-auto d-flex control">
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-trash"></i>
                        </div>
                    </div>
                    ${waitAI}
                </div>`
            break
        case RoleAI:
            chatEleHtml = `
                <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="col-auto icon">${robot_icon}</div>
                        <div class="col text-start ai-response" data-status="waiting">${text}</div>
                </div>`
            if (!isHistory) {
                chatOp = "replace";
            }

            break
    }

    if (chatOp == "append") {
        if (role == RoleAI) {
            // ai response is always after human, so we need to find the last human chat,
            // and append ai response after it
            chatContainer.querySelector(`#${chatID}`).insertAdjacentHTML("beforeend", chatEleHtml);
        } else {
            chatContainer.querySelector(`.chatManager .conservations`).
                insertAdjacentHTML("beforeend", chatEleHtml);
        }
    } else if (chatOp == "replace") {
        // replace html element of ai
        chatEle.querySelector(`.role-ai`).
            outerHTML = chatEleHtml;
    }

    let chatEle = chatContainer.querySelector(`.chatManager .conservations #${chatID}`)
    if (!isHistory && role == RoleHuman) {
        scrollToChat(chatEle);
    }

    // avoid duplicate event listener, only bind event listener for new chat
    if (role == RoleHuman) {
        // bind delete button
        let deleteBtnHandler = (evt) => {
            evt.stopPropagation();

            if (!confirm("Are you sure to delete this chat?")) {
                return;
            }

            chatEle.parentNode.removeChild(chatEle);
            removeChatInStorage(chatID);
        };

        let editHumanInputHandler = (evt) => {
            evt.stopPropagation();

            let oldText = chatContainer.querySelector(`#${chatID}`).innerHTML,
                text = chatContainer.querySelector(`#${chatID} .role-human .text-start pre`).innerHTML;

            // attach image to vision-selected-store when edit human input
            let attachEles = chatContainer
                .querySelectorAll(`.chatManager .conservations #${chatID} .role-human .text-start img`) || [],
                attachHTML = "";
            attachEles.forEach((ele) => {
                let b64fileContent = ele.getAttribute("src").replace("data:image/png;base64,", "");
                let key = ele.dataset.name || `${DateStr()}.png`;
                chatVisionSelectedFileStore[key] = b64fileContent;
                attachHTML += `<img src="data:image/png;base64,${b64fileContent}" data-name="${key}">`;
            });
            updateChatVisionSelectedFileStore();

            text = window.sanitizeHTML(text);
            chatContainer.querySelector(`#${chatID} .role-human`).innerHTML = `
                <textarea dir="auto" class="form-control" rows="3">${text}</textarea>
                <div class="btn-group" role="group">
                    <button class="btn btn-sm btn-outline-secondary save" type="button">
                        <i class="bi bi-check"></i>
                        Save</button>
                    <button class="btn btn-sm btn-outline-secondary cancel" type="button">
                        <i class="bi bi-x"></i>
                        Cancel</button>
                </div>`;

            let saveBtn = chatEle.querySelector(`.role-human .btn.save`);
            let cancelBtn = chatEle.querySelector(`.role-human .btn.cancel`);
            saveBtn.addEventListener("click", async (evt) => {
                evt.stopPropagation();
                let newText = chatEle.querySelector(`.role-human textarea`).value;
                chatEle.innerHTML = `
                    <div class="container-fluid row role-human" data-chatid="${chatID}">
                        <div class="col-auto icon">🤔️</div>
                        <div class="col text-start"><pre>${newText}</pre></div>
                        <div class="col-auto d-flex control">
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-trash"></i>
                        </div>
                    </div>
                    <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="col-auto icon">${robot_icon}</div>
                        <div class="col text-start ai-response" data-status="waiting">
                            <p class="card-text placeholder-glow">
                                <span class="placeholder col-7"></span>
                                <span class="placeholder col-4"></span>
                                <span class="placeholder col-4"></span>
                                <span class="placeholder col-6"></span>
                                <span class="placeholder col-8"></span>
                            </p>
                        </div>
                    </div>`;
                chatEle.querySelector(`.role-ai`).dataset.status = "waiting";

                // bind delete and edit button
                chatEle.querySelector(`.role-human .bi-trash`)
                    .addEventListener("click", deleteBtnHandler);
                chatEle.querySelector(`.bi.bi-pencil-square`)
                    .addEventListener("click", editHumanInputHandler);

                await sendChat2Server(chatID);
                await appendChats2Storage(RoleHuman, chatID, newText, attachHTML);
            });

            cancelBtn.addEventListener("click", (evt) => {
                evt.stopPropagation();
                chatEle.innerHTML = oldText;

                // bind delete and edit button
                chatEle.querySelector(`.role-human .bi-trash`)
                    .addEventListener("click", deleteBtnHandler);
                chatEle.querySelector(`.bi.bi-pencil-square`)
                    .addEventListener("click", editHumanInputHandler);
            });
        };

        // bind delete and edit button
        chatEle.querySelector(`.role-human .bi-trash`)
            .addEventListener("click", deleteBtnHandler);
        chatEle.querySelector(`.bi.bi-pencil-square`)
            .addEventListener("click", editHumanInputHandler);
    }
}


function setupConfig() {
    let tokenTypeParent = configContainer.
        querySelector(".input-group.token-type");

    // set token type
    // {
    //     let selectItems = tokenTypeParent
    //         .querySelectorAll("a.dropdown-item");
    //     switch (window.OpenaiTokenType()) {
    //         case "proxy":
    //             configContainer
    //                 .querySelector(".token-type .show-val").innerHTML = "proxy";
    //             ActiveElementsByData(selectItems, "value", "proxy");
    //             break;
    //         case "direct":
    //             configContainer
    //                 .querySelector(".token-type .show-val").innerHTML = "direct";
    //             ActiveElementsByData(selectItems, "value", "direct");
    //             break;
    //     }

    //     // bind evt listener for choose different token type
    //     selectItems.forEach((ele) => {
    //         ele.addEventListener("click", (evt) => {
    //             // evt.stopPropagation();
    //             configContainer
    //                 .querySelector(".token-type .show-val")
    //                 .innerHTML = evt.target.dataset.value;
    //             window.SetLocalStorage("config_api_token_type", evt.target.dataset.value);
    //         })
    //     });
    // }

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
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();
                localStorage.clear();
                await KvClear();
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

        window.ConfirmModal("delete saved prompt", async () => {
            evt.target.parentElement.remove();

            // remove localstorage shortcut
            let shortcuts = window.GetLocalStorage(StorageKeyPromptShortCuts);
            shortcuts = shortcuts.filter((item) => item.title !== shortcut.title);
            window.SetLocalStorage(StorageKeyPromptShortCuts, shortcuts);
        });
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

        // set default datakey
        if (!datakeyEle.value) {
            datakeyEle.value = window.RandomString(16);
            window.SetLocalStorage(StorageKeyCustomDatasetPassword, datakeyEle.value);
        }

        datakeyEle
            .addEventListener("change", (evt) => {
                evt.stopPropagation();
                window.SetLocalStorage(StorageKeyCustomDatasetPassword, evt.target.value);
            });
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

                if (evt.target.files.length === 0) {
                    return;
                }

                let filename = evt.target.files[0].name,
                    fileext = filename.substring(filename.lastIndexOf(".")).toLowerCase();


                if ([".pdf", ".md", ".ppt", ".pptx", ".doc", ".docx"].indexOf(fileext) === -1) {
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

                if (pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files.length === 0) {
                    showalert("warning", "please choose a pdf file before upload");
                    return;
                }

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
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));

                try {
                    window.ShowSpinner();
                    const resp = await fetch("/ramjet/gptchat/files", {
                        method: "POST",
                        headers: headers,
                        body: form
                    })

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    showalert("success", "upload dataset success, please wait few minutes to process");
                } catch (err) {
                    showalert("danger", `upload dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    window.HideSpinner();
                }
            });
    }

    // bind delete datasets buttion
    const bindDatasetDeleteBtn = () => {
        let datasets = pdfchatModalEle
            .querySelectorAll('div[data-field="dataset"] .dataset-item .bi-trash');

        if (datasets == null || datasets.length === 0) {
            return;
        }

        datasets.forEach((ele) => {
            ele.addEventListener("click", async (evt) => {
                evt.stopPropagation();

                let headers = new Headers();
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));
                headers.append("Cache-Control", "no-cache");
                // headers.append("X-PDFCHAT-PASSWORD", window.GetLocalStorage(StorageKeyCustomDatasetPassword));

                try {
                    window.ShowSpinner();
                    const resp = await fetch(`/ramjet/gptchat/files`, {
                        method: "DELETE",
                        headers: headers,
                        body: JSON.stringify({
                            datasets: [evt.target.closest(".dataset-item").getAttribute("data-filename")]
                        })
                    })

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }
                    await resp.json();
                } catch (err) {
                    showalert("danger", `delete dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    window.HideSpinner();
                }

                // remove dataset item
                evt.target.closest(".dataset-item").remove();
            });
        });
    };

    // bind list datasets
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="refresh"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();

                let headers = new Headers();
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));
                headers.append("Cache-Control", "no-cache");
                headers.append("X-PDFCHAT-PASSWORD", window.GetLocalStorage(StorageKeyCustomDatasetPassword));

                let body;
                try {
                    window.ShowSpinner();
                    const resp = await fetch("/ramjet/gptchat/files", {
                        method: "GET",
                        cache: "no-cache",
                        headers: headers,
                    })

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert("danger", `fetch dataset failed, ${err.message}`);
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
                                <div class="d-flex justify-content-between align-items-center dataset-item" data-filename="${dataset.name}">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch">
                                                <input dir="auto" class="form-check-input" type="checkbox">
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
                                <div class="d-flex justify-content-between align-items-center dataset-item" data-filename="${dataset.name}">
                                    <div class="container-fluid row">
                                        <div class="col-5">
                                            <div class="form-check form-switch">
                                                <input dir="auto" class="form-check-input" type="checkbox" disabled>
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
                        .querySelector(`div[data-filename="${dataset}"] input[type="checkbox"]`)
                        .checked = true;
                });

                bindDatasetDeleteBtn();
            });
    }

    // bind list chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="list-bot"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evt.target.closest(".dropdown")).hide();

                let headers = new Headers();
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));
                headers.append("Cache-Control", "no-cache");
                headers.append("X-PDFCHAT-PASSWORD", window.GetLocalStorage(StorageKeyCustomDatasetPassword));

                let body;
                try {
                    window.ShowSpinner();
                    const resp = await fetch("/ramjet/gptchat/ctx/list", {
                        method: "GET",
                        cache: "no-cache",
                        headers: headers,
                    })

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert("danger", `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    window.HideSpinner();
                }

                let datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let chatbotsHTML = "";

                body.chatbots.forEach((chatbot) => {
                    let selectedHTML = "";
                    if (chatbot == body.current) {
                        selectedHTML = `checked`;
                    }


                    chatbotsHTML += `
                        <div class="d-flex justify-content-between align-items-center chatbot-item" data-name="${chatbot}">
                            <div class="container-fluid row">
                                <div class="col-5">
                                    <div class="form-check form-switch">
                                        <input dir="auto" class="form-check-input" type="checkbox" ${selectedHTML}>
                                        <label class="form-check-label" for="flexSwitchCheckChecked">${chatbot}</label>
                                    </div>
                                </div>
                                <div class="col-5">
                                </div>
                                <div class="col-2 text-end">
                                    <i class="bi bi-trash"></i>
                                </div>
                            </div>
                        </div>`

                });

                datasetListEle.innerHTML = chatbotsHTML;

                // bind active new selected chatbot
                datasetListEle
                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                    .forEach((ele) => {
                        ele.addEventListener("change", async (evt) => {
                            evt.stopPropagation();

                            if (!evt.target.checked) {
                                // at least one chatbot should be selected
                                evt.target.checked = true;
                                return;
                            } else {
                                // uncheck other chatbot
                                datasetListEle
                                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                                    .forEach((ele) => {
                                        if (ele != evt.target) {
                                            ele.checked = false;
                                        }
                                    });
                            }

                            let headers = new Headers();
                            headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                            headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));

                            try {
                                window.ShowSpinner();
                                const chatbotName = evt.target.closest(".chatbot-item").getAttribute("data-name");
                                const resp = await fetch("/ramjet/gptchat/ctx/active", {
                                    method: "POST",
                                    headers: headers,
                                    body: JSON.stringify({
                                        data_key: window.GetLocalStorage(StorageKeyCustomDatasetPassword),
                                        chatbot_name: chatbotName
                                    })
                                })

                                if (!resp.ok || resp.status !== 200) {
                                    throw new Error(`${resp.status} ${await resp.text()}`);
                                }

                                const body = await resp.json();
                                showalert("success", `active chatbot success, you can chat with ${chatbotName} now`);
                            } catch (err) {
                                showalert("danger", `active chatbot failed, ${err.message}`);
                                throw err;
                            } finally {
                                window.HideSpinner();
                            }

                        });
                    });

            });
    }

    // bind share chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="share-bot"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evt.target.closest(".dropdown")).hide();

                let checkedChatbotEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"] .chatbot-item input[type="checkbox"]:checked');
                if (!checkedChatbotEle) {
                    showalert("danger", `please click [Chatbot List] first`);
                    return;
                }

                let chatbot_name = checkedChatbotEle.closest(".chatbot-item").getAttribute("data-name");

                let headers = new Headers();
                headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));
                headers.append("Cache-Control", "no-cache");

                let respBody;
                try {
                    window.ShowSpinner();
                    const resp = await fetch("/ramjet/gptchat/ctx/share", {
                        method: "POST",
                        headers: headers,
                        body: JSON.stringify({
                            chatbot_name: chatbot_name,
                            data_key: window.GetLocalStorage(StorageKeyCustomDatasetPassword),
                        })
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    respBody = await resp.json();
                } catch (err) {
                    showalert("danger", `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    window.HideSpinner();
                }

                // open new tab page
                const sharedChatbotUrl = `${window.location.origin}/?chatmodel=qa-shared&uid=${respBody.uid}&chatbot_name=${respBody.chatbot_name}`;
                showalert("info", `open ${sharedChatbotUrl}`);
                window.open(sharedChatbotUrl, "_blank");
            });
    }

    // build custom chatbot
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="build-bot"]')
            .addEventListener("click", async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evt.target.closest(".dropdown")).hide();

                let selectedDatasets = [];
                pdfchatModalEle.
                    querySelectorAll('div[data-field="dataset"] .dataset-item input[type="checkbox"]')
                    .forEach((ele) => {
                        if (ele.checked) {
                            selectedDatasets.push(
                                ele.closest(".dataset-item").getAttribute("data-filename"));
                        }
                    });

                if (selectedDatasets.length === 0) {
                    showalert("warning", "please select at least one dataset, click [List Dataset] button to fetch dataset list");
                    return;
                }

                // ask chatbot's name
                window.SingleInputModal("build bot", "chatbot name", async (botname) => {
                    // botname should be 1-32 ascii characters
                    if (!botname.match(/^[a-zA-Z0-9_\-]{1,32}$/)) {
                        showalert("warning", "chatbot name should be 1-32 ascii characters");
                        return;
                    }

                    let headers = new Headers();
                    headers.append("Content-Type", "application/json");
                    headers.append("Authorization", `Bearer ${window.OpenaiToken()}`);
                    headers.append("X-Laisky-User-Id", await window.getSHA1(window.OpenaiToken()));

                    try { // build chatbot
                        window.ShowSpinner();
                        const resp = await fetch("/ramjet/gptchat/ctx/build", {
                            method: "POST",
                            headers: headers,
                            body: JSON.stringify({
                                chatbot_name: botname,
                                datasets: selectedDatasets,
                                data_key: pdfchatModalEle
                                    .querySelector('div[data-field="data-key"] input').value
                            })
                        })

                        if (!resp.ok || resp.status !== 200) {
                            throw new Error(`${resp.status} ${await resp.text()}`);
                        }

                        showalert("success", "build dataset success, you can chat now");
                    } catch (err) {
                        showalert("danger", `build dataset failed, ${err.message}`);
                        throw err;
                    } finally {
                        window.HideSpinner();
                    }
                });
            });
    }
}
