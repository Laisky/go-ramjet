'use strict';

// const
const RoleHuman = 'user';
const RoleSystem = 'system';
const RoleAI = 'assistant';

const chatContainer = document.getElementById('chatContainer');
const configContainer = document.getElementById('hiddenChatConfigSideBar');
const chatPromptInputEle = chatContainer.querySelector('.user-input .input.prompt');
const chatPromptInputBtn = chatContainer.querySelector('.user-input .btn.send');

// could be controlled(interrupt) anywhere, so it's global
let globalAIRespSSE, globalAIRespEle;

// [
//     {
//         filename: xx,
//         contentB64: xx,
//         cache_key: xx
//     }
// ]
//
// should invoke updateChatVisionSelectedFileStore after update this object
let chatVisionSelectedFileStore = [];

// eslint-disable-next-line no-unused-vars
async function setupChatJs () {
    await setupSessionManager();
    await setupConfig();
    await setupChatInput();
    await setupPromptManager();
    await setupPrivateDataset();
    setInterval(fetchImageDrawingResultBackground, 3000);
}

function newChatID () {
    return `chat-${(new Date()).getTime()}-${RandomString(6)}`;
}

// show alert
//
// type: primary, secondary, success, danger, warning, info, light, dark
function showalert (type, msg) {
    const alertEle = `<div class="alert alert-${type} alert-dismissible" role="alert">
            <div>${sanitizeHTML(msg)}</div>
            <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
        </div>`;

    // append as first child
    chatContainer.querySelector('.chatManager .alerts-container .alerts')
        .insertAdjacentHTML('afterbegin', alertEle);
}

// check sessionID's type, secure convert to int, default is 1
function kvSessionKey (sessionID) {
    sessionID = parseInt(sessionID) || 1
    return `${KvKeyPrefixSessionHistory}${sessionID}`
}

/**
 *
 * @param {*} sessionID
 * @returns {Array} An array of chat messages.
 */
async function sessionChatHistory (sessionID) {
    let data = (await KvGet(kvSessionKey(sessionID)));
    if (!data) {
        return [];
    }

    // fix legacy bug for marshal data twice
    if (typeof data === 'string') {
        data = JSON.parse(data);
        await KvSet(kvSessionKey(sessionID), data);
    }

    return data;
}

/**
 *
 * @returns {Array} An array of chat messages, oldest first.
 */
async function activeSessionChatHistory () {
    const sid = await activeSessionID();
    if (!sid) {
        return [];
    }

    return await sessionChatHistory(sid);
}

async function activeSessionID () {
    let activeSession = document.querySelector('#sessionManager .card-body button.active');
    if (activeSession) {
        return activeSession.dataset.session;
    }

    activeSession = await KvGet(KvKeyPrefixSelectedSession)
    if (activeSession) {
        return activeSession;
    }

    return 1;
}

async function listenSessionSwitch (evt) {
    // deactive all sessions
    evt = evtTarget(evt);
    if (!evt.classList.contains('list-group-item')) {
        evt = evt.closest('.list-group-item');
    }

    const activeSid = evt.dataset.session;
    document
        .querySelectorAll(`
            #sessionManager .sessions .list-group-item,
            #chatContainer .sessions .list-group-item
        `)
        .forEach((item) => {
            if (item.dataset.session === activeSid) {
                item.classList.add('active');
            } else {
                item.classList.remove('active');
            }
        })

    // restore session hisgoty
    chatContainer.querySelector('.conservations .chats').innerHTML = '';
    (await sessionChatHistory(activeSid)).forEach((item) => {
        append2Chats(item.chatID, item.role, item.content, true, item.attachHTML);
        renderAfterAIResponse(item.chatID);
    })

    await KvSet(KvKeyPrefixSelectedSession, activeSid);
    await updateConfigFromSessionConfig();
    await autoToggleUserImageUploadBtn();
    EnableTooltipsEverywhere();
}

/**
 * Fetches the image drawing result background for the AI response and displays it in the chat container.
 * @returns {Promise<void>}
 */
async function fetchImageDrawingResultBackground () {
    const elements = chatContainer
        .querySelectorAll('.role-ai .ai-response[data-task-type="image"][data-status="waiting"]') || [];

    await Promise.all(Array.from(elements).map(async (item) => {
        if (item.dataset.status !== 'waiting') {
            return;
        }

        // const taskId = item.dataset.taskId;
        const imageUrls = JSON.parse(item.dataset.imageUrls) || [];
        const chatId = item.closest('.role-ai').dataset.chatid;

        try {
            await Promise.all(imageUrls.map(async (imageUrl) => {
                // check any err msg
                const errFileUrl = imageUrl.slice(0, imageUrl.lastIndexOf('-')) + '.err.txt';
                const errFileResp = await fetch(`${errFileUrl}?rr=${RandomString(12)}`, {
                    method: 'GET',
                    cache: 'no-cache'
                });
                if (errFileResp.ok || errFileResp.status === 200) {
                    const errText = await errFileResp.text();
                    item.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${errText}</pre>`;
                    checkIsImageAllSubtaskDone(item, imageUrl, false);
                    await appendChats2Storage(RoleAI, chatId, item.innerHTML);
                    return;
                }

                // check is image ready
                const imgResp = await fetch(`${imageUrl}?rr=${RandomString(12)}`, {
                    method: 'GET',
                    cache: 'no-cache'
                });
                if (!imgResp.ok || imgResp.status != 200) {
                    return;
                }

                // check is all tasks finished
                checkIsImageAllSubtaskDone(item, imageUrl, true);
            }))
        } catch (err) {
            console.warn('fetch img result, ' + err);
        };
    }));
}

/** append chat to chat container
 *
 * @param {element} item ai respnse
 * @param {string} imageUrl current subtask's image url
 * @param {boolean} succeed is current subtask succeed
 */
function checkIsImageAllSubtaskDone (item, imageUrl, succeed) {
    let processingImageUrls = JSON.parse(item.dataset.imageUrls) || [];
    if (!processingImageUrls.includes(imageUrl)) {
        return;
    }

    // remove current subtask from imageUrls(tasks)
    processingImageUrls = processingImageUrls.filter((url) => url !== imageUrl)
    item.dataset.imageUrls = JSON.stringify(processingImageUrls);

    const succeedImageUrls = JSON.parse(item.dataset.succeedImageUrls || '[]');
    if (succeed) {
        succeedImageUrls.push(imageUrl);
        item.dataset.succeedImageUrls = JSON.stringify(succeedImageUrls);
    } else { // task failed
        processingImageUrls = [];
        item.dataset.imageUrls = JSON.stringify(processingImageUrls);
    }

    if (processingImageUrls.length == 0 && succeedImageUrls.length > 0) {
        item.dataset.status = 'done';
        let imgHTML = '';
        succeedImageUrls.forEach((url) => {
            imgHTML += `<img src="${url}">`;
        });
        item.innerHTML = imgHTML;

        if (succeedImageUrls.length > 1) {
            item.classList.add('multi-images');
        }
    }
}

/**
 * Clears all user sessions and chats from local storage,
 * and resets the chat UI to its initial state.
 * @param {Event} evt - The event that triggered the function (optional).
 * @param {string} sessionID - The session ID to clear (optional).
 *
 * @returns {void}
 */
async function clearSessionAndChats (evt, sessionID) {
    console.debug('clearSessionAndChats', evt, sessionID)
    if (evt) {
        evt.stopPropagation();
    }

    // remove pinned materials
    await KvDel(KvKeyPinnedMaterials);

    if (!sessionID) { // remove all session
        const sessionConfig = await KvGet(`${KvKeyPrefixSessionConfig}${(await activeSessionID())}`);

        await Promise.all((await KvList()).map(async (key) => {
            if (
                key.startsWith(KvKeyPrefixSessionHistory) || // remove all sessions
                key.startsWith(KvKeyPrefixSessionConfig) // remove all sessions' config
            ) {
                await KvDel(key);
            }
        }));

        // restore session config
        await KvSet(`${KvKeyPrefixSessionConfig}1`, sessionConfig);
        await KvSet(kvSessionKey(1), []);
    } else { // only remove one session's chat, keep config
        await Promise.all((await KvList()).map(async (key) => {
            if (
                key.startsWith(KvKeyPrefixSessionHistory) && // remove all sessions
                key.endsWith(`_${sessionID}`) // remove specified session
            ) {
                await KvSet(key, []);
            }
        }));
    }

    location.reload();
}

function bindSessionEditBtn () {
    document.querySelectorAll('#sessionManager .sessions .session .bi.bi-pencil-square')
        .forEach((item) => {
            if (item.dataset.bindClicked) {
                return;
            } else {
                item.dataset.bindClicked = true;
            }

            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                evt = evtTarget(evt);
                const sid = evt.closest('.session').dataset.session;
                const sconfig = await getChatSessionConfig(sid);
                const oldSessionName = sconfig.session_name || sid;

                SingleInputModal('Edit session', 'Session name', async (newSessionName) => {
                    if (!newSessionName) {
                        return;
                    }

                    // update session config
                    sconfig.session_name = newSessionName;
                    await KvSet(`${KvKeyPrefixSessionConfig}${sid}`, sconfig);

                    // update session name
                    document
                        .querySelector(`#sessionManager .sessions [data-session="${sid}"] .col`).innerHTML = newSessionName;
                    chatContainer
                        .querySelector(`.sessions [data-session="${sid}"] .col`)
                        .innerHTML = newSessionName;
                }, oldSessionName);
            })
        })
}

function bindSessionDeleteBtn () {
    const btns = document.querySelectorAll('#sessionManager .sessions .session .bi-trash') || [];
    btns.forEach((item) => {
        if (item.dataset.bindClicked) {
            return;
        } else {
            item.dataset.bindClicked = true;
        }

        item.addEventListener('click', async (evt) => {
            evt.stopPropagation();

            // if there is only one session, don't delete it
            if (document.querySelectorAll('#sessionManager .sessions .session').length == 1) {
                return;
            }

            const sid = evtTarget(evt).closest('.session').dataset.session;
            ConfirmModal('Are you sure to delete this session?', async () => {
                await KvDel(`${KvKeyPrefixSessionHistory}${sid}`);
                await KvDel(`${KvKeyPrefixSessionConfig}${sid}`);
                document
                    .querySelector(`#sessionManager .sessions [data-session="${sid}"]`).remove();
                chatContainer
                    .querySelector(`.sessions [data-session="${sid}"]`).remove();
            });
        });
    });
}

/** setup session manager and restore current chat history
 *
 */
async function setupSessionManager () {
    const selectedSessionID = await activeSessionID();

    // bind remove all sessions
    {
        document
            .querySelector('#sessionManager .btn.purge')
            .addEventListener('click', clearSessionAndChats);
    }

    // restore all sessions from storage
    {
        const allSessionKeys = [];
        (await KvList()).forEach((key) => {
            if (key.startsWith(KvKeyPrefixSessionHistory)) {
                allSessionKeys.push(key);
            }
        });

        if (allSessionKeys.length == 0) { // there is no session, create one
            // create session history
            const skey = kvSessionKey(1);
            allSessionKeys.push(skey);
            await KvSet(skey, []);

            // create session config
            await KvSet(`${KvKeyPrefixSessionConfig}1`, newSessionConfig());
        }

        await Promise.all(allSessionKeys.map(async (key) => {
            const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));

            let active = '';
            const sconfig = await getChatSessionConfig(sessionID);
            const sessionName = sconfig.session_name || sessionID;
            if (sessionID == selectedSessionID) {
                active = 'active';
            }

            document
                .querySelector('#sessionManager .sessions')
                .insertAdjacentHTML(
                    'beforeend',
                    `<div class="list-group">
                        <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                            <div class="col">${sessionName}</div>
                            <i class="bi bi-pencil-square"></i>
                            <i class="bi bi-trash col-auto"></i>
                        </button>
                    </div>`);
            chatContainer
                .querySelector('.sessions')
                .insertAdjacentHTML(
                    'beforeend',
                    `<div class="list-group">
                        <button type="button" class="list-group-item list-group-item-action session ${active}" aria-current="true" data-session="${sessionID}">
                            <div class="col">${sessionName}</div>
                        </button>
                    </div>`);
        }));

        // restore conservation history
        (await activeSessionChatHistory()).forEach((item) => {
            append2Chats(item.chatID, item.role, item.content, true, item.attachHTML);
            renderAfterAIResponse(item.chatID);
        });
    }

    // add widget to scroll bottom
    {
        document.querySelector('#chatContainer .chatManager .card-footer .scroll-down')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                scrollChatToDown();
            });
    }

    // new session
    {
        document
            .querySelector('#sessionManager .btn.new-session')
            .addEventListener('click', async (evt) => {
                let maxSessionID = 0;
                (await KvList()).forEach((key) => {
                    if (key.startsWith(KvKeyPrefixSessionHistory)) {
                        const sessionID = parseInt(key.replace(KvKeyPrefixSessionHistory, ''));
                        if (sessionID > maxSessionID) {
                            maxSessionID = sessionID;
                        }
                    }
                });

                // deactive all sessions
                document.querySelectorAll(`
                    #sessionManager .sessions .list-group-item.active,
                    #chatContainer .sessions .list-group-item.active
                `).forEach((item) => {
                    item.classList.remove('active');
                });

                // add new active session
                chatContainer
                    .querySelector('.chatManager .conservations .chats').innerHTML = '';
                const newSessionID = maxSessionID + 1;
                document
                    .querySelector('#sessionManager .sessions')
                    .insertAdjacentHTML(
                        'beforeend',
                        `<div class="list-group">
                            <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                <div class="col">${newSessionID}</div>
                                <i class="bi bi-pencil-square"></i>
                                <i class="bi bi-trash col-auto"></i>
                            </button>
                        </div>`);
                chatContainer
                    .querySelector('.sessions')
                    .insertAdjacentHTML(
                        'beforeend',
                        `<div class="list-group">
                            <button type="button" class="list-group-item list-group-item-action active session" aria-current="true" data-session="${newSessionID}">
                                <div class="col">${newSessionID}</div>
                            </button>
                        </div>`);

                // save new session history and config
                await KvSet(kvSessionKey(newSessionID), []);
                const oldSessionConfig = await KvGet(`${KvKeyPrefixSessionConfig}${maxSessionID}`);
                const sconfig = newSessionConfig();
                sconfig.api_token = oldSessionConfig.api_token; // keep api token
                await KvSet(`${KvKeyPrefixSessionConfig}${newSessionID}`, sconfig);
                await KvSet(KvKeyPrefixSelectedSession, newSessionID);

                // bind session switch listener for new session
                document
                    .querySelector(`
                        #sessionManager .sessions [data-session="${newSessionID}"],
                        #chatContainer .sessions [data-session="${newSessionID}"]
                    `)
                    .addEventListener('click', listenSessionSwitch);

                bindSessionEditBtn();
                bindSessionDeleteBtn();
                await updateConfigFromSessionConfig();
            })
    }

    // bind session switch
    {
        document
            .querySelectorAll(`
                #sessionManager .sessions .list-group .session,
                #chatContainer .sessions .list-group .session
            `)
            .forEach((item) => {
                item.addEventListener('click', listenSessionSwitch);
            })
    }

    bindSessionEditBtn();
    bindSessionDeleteBtn();
}

// remove chat in storage by chatid
async function removeChatInStorage (chatid) {
    if (!chatid) {
        throw new Error('chatid is required');
    }

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    let session = await activeSessionChatHistory();

    // remove all chats with the same chatid
    session = session.filter((item) => item.chatID !== chatid);

    await KvSet(storageActiveSessionKey, session);
}

/** append or update chat history by chatid and role
    * @param {string} chatid - chat id
    * @param {string} role - user or assistant
    * @param {string} renderedContent - chat content
    * @param {string} attachHTML - chat content's attach html
*/
async function appendChats2Storage (role, chatid, renderedContent, attachHTML, rawContent) {
    if (!chatid) {
        throw new Error('chatid is required');
    }

    const storageActiveSessionKey = kvSessionKey(await activeSessionID());
    const session = await activeSessionChatHistory();

    // if chat is already in history, find and update it.
    let found = false;
    session.forEach((item, idx) => {
        if (item.chatID === chatid && item.role === role) {
            found = true;
            item.content = renderedContent;
            item.attachHTML = attachHTML;
            item.rawContent = rawContent;
        }
    });

    // if ai response is not in history, add it after user's chat which has same chatid
    if (!found && role == RoleAI) {
        session.forEach((item, idx) => {
            if (item.chatID === chatid) {
                found = true;
                if (item.role !== RoleAI) {
                    session.splice(idx + 1, 0, {
                        role: RoleAI,
                        chatID: chatid,
                        content: renderedContent,
                        attachHTML,
                        rawContent
                    });
                }
            }
        });
    }

    // if chat is not in history, add it
    if (!found) {
        session.push({
            role,
            chatID: chatid,
            content: renderedContent,
            attachHTML,
            rawContent
        });
    }

    await KvSet(storageActiveSessionKey, session);
}

function scrollChatToDown () {
    ScrollDown(document.querySelector('html'));
    ScrollDown(chatContainer.querySelector('.chatManager .conservations'));
}

function scrollToChat (chatEle) {
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
async function getLastNChatMessages (N, ignoredChatID) {
    console.debug('getLastNChatMessages', N, ignoredChatID)

    const systemPrompt = await OpenaiChatStaticContext()
    const selectedModel = await OpenaiSelectedModel()

    if (selectedModel == ChatModelGeminiPro) {
        // one-api's gemoni-pro do not support context
        return [{
            role: RoleSystem,
            content: systemPrompt
        }]
    }

    let messages = (await activeSessionChatHistory()).filter((ele) => {
        if (ele.role != RoleHuman) {
            // Ignore AI's chat, only use human's chat as context.
            return false
        };

        if (ignoredChatID && ignoredChatID == ele.chatID) {
            // This is a reload request with edited chat,
            // ignore chat with same chatid to avoid duplicate context.
            return false
        }

        return true
    })

    if (N == 0) {
        messages = []
    } else {
        messages = messages.slice(-N)
    }

    if (systemPrompt) {
        messages = [{
            role: RoleSystem,
            content: systemPrompt
        }].concat(messages)
    }

    return messages
}

function lockChatInput () {
    chatPromptInputBtn.classList.add('disabled')
}
function unlockChatInput () {
    chatPromptInputBtn.classList.remove('disabled')
}
function isAllowChatPrompInput () {
    return !chatPromptInputBtn.classList.contains('disabled')
}

function parseChatResp (chatmodel, payload) {
    if (!payload.choices || payload.choices.length === 0) {
        payload.choices = [{
            delta: {
                content: '',
                text: ''
            }
        }];
    }

    if (IsChatModel(chatmodel) || IsQaModel(chatmodel)) {
        return payload.choices[0].delta.content || ''
    } else if (IsCompletionModel(chatmodel)) {
        return payload.choices[0].text || ''
    } else {
        showalert('error', `Unknown chat model ${chatmodel}`)
    }
}

const httpsRegexp = /\bhttps:\/\/\S+/

/**
 * extract https urls from reqPrompt and pin them to the chat conservation window
 *
 * @param {string} reqPrompt - request prompt
 * @returns {string} modified request prompt
 */
async function userPromptEnhance (reqPrompt) {
    const pinnedUrls = getPinnedMaterials() || [];
    const sconfig = await getChatSessionConfig();
    const urls = reqPrompt.match(httpsRegexp);

    if (sconfig.chat_switch.disable_https_crawler) {
        console.debug('https create new material is disabled, skip prompt enhance');
        return reqPrompt;
    }

    if (urls) {
        urls.forEach((url) => {
            if (!pinnedUrls.includes(url)) {
                pinnedUrls.push(url);
            }
        });
    }

    if (pinnedUrls.length === 0) {
        return reqPrompt;
    }

    let urlEle = '';
    for (const url of pinnedUrls) {
        urlEle += `<p><i class="bi bi-trash"></i> <a href="${url}" class="link-primary" target="_blank">${url}</a></p>`;
    }

    // save to storage
    // FIXME save to session config
    await KvSet(KvKeyPinnedMaterials, urlEle);
    await restorePinnedMaterials();

    // re generate reqPrompt
    reqPrompt = reqPrompt.replace(httpsRegexp, '');
    reqPrompt += '\nrefs:\n- ' + pinnedUrls.join('\n- ');
    return reqPrompt;
}

async function restorePinnedMaterials () {
    const urlEle = await KvGet(KvKeyPinnedMaterials) || '';
    const container = document.querySelector('#chatContainer .pinned-refs');
    container.innerHTML = urlEle;

    // bind to remove pinned materials
    document.querySelectorAll('#chatContainer .pinned-refs p .bi-trash')
        .forEach((item) => {
            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const container = evtTarget(evt).closest('.pinned-refs');
                const ele = evtTarget(evt).closest('p');
                ele.parentNode.removeChild(ele);

                // update storage
                await KvSet(KvKeyPinnedMaterials, container.innerHTML);
            })
        })
}

function getPinnedMaterials () {
    const urls = []
    document.querySelectorAll('#chatContainer .pinned-refs a')
        .forEach((item) => {
            urls.push(item.innerHTML);
        })

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
async function sendTxt2ImagePrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;

    switch (selectedModel) {
    case ImageModelDalle2:
        url = '/images/generations';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
    }

    const sconfig = await getChatSessionConfig();
    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: JSON.stringify({
            model: selectedModel,
            prompt
        })
    });
    if (!resp.ok || resp.status != 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    let attachHTML = '';
    respData.image_urls.forEach((url) => {
        attachHTML += `<img src="${url}">`;
    })

    // save img to storage no matter it's done or not
    await appendChats2Storage(RoleAI, chatID, attachHTML);
}

async function sendSdxlturboPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
    case ImageModelSdxlTurbo:
        url = '/images/generations/sdxl-turbo';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    let imageBase64 = '';
    if (chatVisionSelectedFileStore.length !== 0) {
        imageBase64 = chatVisionSelectedFileStore[0].contentB64;

        // insert image to user input & hisotry
        await appendImg2UserInput(chatID, imageBase64, `${DateStr()}.png`);

        chatVisionSelectedFileStore = [];
        updateChatVisionSelectedFileStore();
    }

    const sconfig = await getChatSessionConfig();
    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
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

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // save img to storage no matter it's done or not
    let attachHTML = '';
    respData.image_urls.forEach((url) => {
        attachHTML += `<img src="${url}">`
    });

    await appendChats2Storage(RoleAI, chatID, attachHTML);
}

async function sendImg2ImgPrompt2Server (chatID, selectedModel, currentAIRespEle, prompt) {
    let url;
    switch (selectedModel) {
    case ImageModelImg2Img:
        url = '/images/generations/lcm';
        break;
    default:
        throw new Error(`unknown image model: ${selectedModel}`);
    }

    // get first image in store
    if (chatVisionSelectedFileStore.length === 0) {
        throw new Error('no image selected');
    }
    const imageBase64 = chatVisionSelectedFileStore[0].contentB64;

    // insert image to user input & hisotry
    await appendImg2UserInput(chatID, imageBase64, `${DateStr()}.png`);

    chatVisionSelectedFileStore = [];
    updateChatVisionSelectedFileStore();

    const sconfig = await getChatSessionConfig();
    const resp = await fetch(url, {
        method: 'POST',
        headers: {
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        body: JSON.stringify({
            model: selectedModel,
            prompt,
            image_base64: imageBase64
        })
    });
    if (!resp.ok || resp.status != 200) {
        throw new Error(`[${resp.status}]: ${await resp.text()}`);
    }
    const respData = await resp.json();

    currentAIRespEle.dataset.status = 'waiting';
    currentAIRespEle.dataset.taskType = 'image';
    currentAIRespEle.dataset.taskId = respData.task_id;
    currentAIRespEle.dataset.imageUrls = JSON.stringify(respData.image_urls);

    // save img to storage no matter it's done or not
    let attachHTML = '';
    respData.image_urls.forEach((url) => {
        attachHTML += `<img src="${url}">`;
    })

    await appendChats2Storage(RoleAI, chatID, attachHTML);
}

async function appendImg2UserInput (chatID, imgDataBase64, imgName) {
    // insert image to user hisotry
    const text = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start pre`).innerHTML;
    await appendChats2Storage(RoleHuman, chatID, text,
        `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`
    );

    // insert image to user input
    chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start`)
        .insertAdjacentHTML(
            'beforeend',
            `<img src="data:image/png;base64,${imgDataBase64}" data-name="${imgName}">`
        );
}

async function sendChat2Server (chatID) {
    let reqPrompt;
    if (!chatID) { // if chatID is empty, it's a new request
        chatID = newChatID();
        reqPrompt = TrimSpace(chatPromptInputEle.value || '');

        chatPromptInputEle.value = '';
        if (reqPrompt == '') {
            return;
        }

        append2Chats(chatID, RoleHuman, reqPrompt, false);
        await appendChats2Storage(RoleHuman, chatID, reqPrompt)
    } else { // if chatID is not empty, it's a reload request
        reqPrompt = chatContainer
            .querySelector(`.chatManager .conservations .chats #${chatID} .role-human .text-start pre`).innerHTML;
    }

    // extract and pin new material in chat
    reqPrompt = await userPromptEnhance(reqPrompt);

    globalAIRespEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`);
    lockChatInput();

    let selectedModel = await OpenaiSelectedModel();
    // get chatmodel from url parameters
    if (location.search) {
        const params = new URLSearchParams(location.search);
        if (params.has('chatmodel')) {
            selectedModel = params.get('chatmodel')
        }
    }

    // these extras will append to the tail of AI's response
    let responseExtras = '';
    let reqBody;
    const sconfig = await getChatSessionConfig();

    if (IsChatModel(selectedModel)) {
        let messages;
        const nContexts = parseInt(sconfig.n_contexts);

        if (chatID) { // reload current chat by latest context
            messages = await getLastNChatMessages(nContexts - 1, chatID);
            messages.push({
                role: RoleHuman,
                content: reqPrompt
            });
        } else {
            messages = await getLastNChatMessages(nContexts, chatID);
        }

        // if selected model is vision model, but no image selected, abort
        if (selectedModel.includes('vision') && chatVisionSelectedFileStore.length === 0) {
            abortAIResp('you should select at least one image for vision model');
            return;
        }

        // there are pinned files, add them to user's prompt
        if (chatVisionSelectedFileStore.length !== 0) {
            if (!selectedModel.includes('vision')) {
                // if selected model is not vision model, just ignore it
                chatVisionSelectedFileStore = [];
                updateChatVisionSelectedFileStore();
                return;
            }

            messages[messages.length - 1].files = [];
            for (const item of chatVisionSelectedFileStore) {
                messages[messages.length - 1].files.push({
                    type: 'image',
                    name: item.filename,
                    content: item.contentB64
                });

                // insert image to user input & hisotry
                await appendImg2UserInput(chatID, item.contentB64, item.filename);
            }

            chatVisionSelectedFileStore = [];
            updateChatVisionSelectedFileStore();
        }

        reqBody = JSON.stringify({
            model: selectedModel,
            stream: true,
            max_tokens: parseInt(sconfig.max_tokens),
            temperature: parseFloat(sconfig.temperature),
            presence_penalty: parseFloat(sconfig.presence_penalty),
            frequency_penalty: parseFloat(sconfig.frequency_penalty),
            messages,
            stop: ['\n\n'],
            laisky_extra: {
                chat_switch: sconfig.chat_switch
            }
        });
    } else if (IsCompletionModel(selectedModel)) {
        reqBody = JSON.stringify({
            model: selectedModel,
            stream: true,
            max_tokens: parseInt(sconfig.max_tokens),
            temperature: parseFloat(sconfig.temperature),
            presence_penalty: parseFloat(sconfig.presence_penalty),
            frequency_penalty: parseFloat(sconfig.frequency_penalty),
            prompt: reqPrompt,
            stop: ['\n\n']
        });
    } else if (IsQaModel(selectedModel)) {
        // {
        //     "question": "XFS 是干啥的",
        //     "text": " XFS is a simple CLI tool that can be used to create volumes/mounts and perform simple filesystem operations.\n",
        //     "url": "http://xego-dev.basebit.me/doc/xfs/support/xfs2_cli_instructions/"
        // }

        let url, project;
        const params = new URLSearchParams(location.search);
        switch (selectedModel) {
        case QAModelBasebit:
        case QAModelSecurity:
        case QAModelImmigrate:
            data.qa_chat_models.forEach((item) => {
                if (item.name == selectedModel) {
                    url = item.url;
                    project = item.project;
                }
            })

            if (!project) {
                console.error("can't find project name for chat model: " + selectedModel);
                return;
            }

            url = `${url}?p=${project}&q=${encodeURIComponent(reqPrompt)}`;
            break;
        case QAModelCustom:
            url = `/ramjet/gptchat/ctx/search?q=${encodeURIComponent(reqPrompt)}`;
            break;
        case QAModelShared:
            // example url:
            //
            // https://chat2.laisky.com/?chatmodel=qa-shared&uid=public&chatbotName=default

            url = `/ramjet/gptchat/ctx/share?uid=${params.get('uid')}` +
                    `&chatbotName=${params.get('chatbotName')}` +
                    `&q=${encodeURIComponent(reqPrompt)}`;
            break;
        default:
            console.error('unknown qa chat model: ' + selectedModel);
        }

        globalAIRespEle.scrollIntoView({ behavior: 'smooth' });
        try {
            const resp = await fetch(url, {
                method: 'GET',
                cache: 'no-cache',
                headers: {
                    Connection: 'keep-alive',
                    'Content-Type': 'application/json',
                    Authorization: 'Bearer ' + sconfig.api_token,
                    'X-Laisky-User-Id': await getSHA1(sconfig.api_token),
                    'X-Laisky-Api-Base': sconfig.api_base,
                    'X-PDFCHAT-PASSWORD': await KvGet(KvKeyCustomDatasetPassword)
                }
            });

            if (!resp.ok || resp.status != 200) {
                throw new Error(`[${resp.status}]: ${await resp.text()}`);
            }

            const data = await resp.json();
            if (!data || !data.text) {
                abortAIResp('cannot gather sufficient context to answer the question');
                return;
            }

            responseExtras = `
                    <p style="margin-bottom: 0;">
                        <button class="btn btn-info" type="button" data-bs-toggle="collapse" data-bs-target="#chatRef-${chatID}" aria-expanded="false" aria-controls="chatRef-${chatID}" style="font-size: 0.6em">
                            > toggle reference
                        </button>
                    </p>`;

            if (data.url) {
                responseExtras += `
                    <div>
                        <div class="collapse" id="chatRef-${chatID}">
                            <div class="card card-body">${combineRefs(data.url)}</div>
                        </div>
                    </div>`;
            }

            const messages = [{
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
            const model = ChatModelTurbo35V1106; // rewrite chat model

            reqBody = JSON.stringify({
                model,
                stream: true,
                max_tokens: parseInt(sconfig.max_tokens),
                temperature: parseFloat(sconfig.temperature),
                presence_penalty: parseFloat(sconfig.presence_penalty),
                frequency_penalty: parseFloat(sconfig.frequency_penalty),
                messages,
                stop: ['\n\n']
            });
        } catch (err) {
            abortAIResp(err);
            return;
        }
    } else if (IsImageModel(selectedModel)) {
        try {
            switch (selectedModel) {
            case ImageModelDalle2:
                await sendTxt2ImagePrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            case ImageModelImg2Img:
                await sendImg2ImgPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            case ImageModelSdxlTurbo:
                await sendSdxlturboPrompt2Server(chatID, selectedModel, globalAIRespEle, reqPrompt);
                break;
            default:
                throw new Error(`unknown image model: ${selectedModel}`);
            }
        } catch (err) {
            abortAIResp(err);
        } finally {
            unlockChatInput();
        }

        return;
    } else {
        globalAIRespEle.innerHTML = '<p>🔥Someting in trouble...</p>' +
            '<pre style="background-color: #f8e8e8; text-wrap: pretty;">' +
            `unimplemented model: ${sanitizeHTML(selectedModel)}</pre>`;
        appendChats2Storage(RoleAI, chatID, globalAIRespEle.innerHTML);
        unlockChatInput();
        return;
    }

    if (!reqBody) {
        return;
    }

    globalAIRespSSE = new SSE('/api', {
        headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer ' + sconfig.api_token,
            'X-Laisky-User-Id': await getSHA1(sconfig.api_token),
            'X-Laisky-Api-Base': sconfig.api_base
        },
        method: 'POST',
        payload: reqBody
    });

    // origin response from ai
    let aiRawResp = '';
    globalAIRespSSE.addEventListener('message', async (evt) => {
        evt.stopPropagation();

        let isChatRespDone = false;
        if (evt.data == '[DONE]') {
            isChatRespDone = true;
        } else if (evt.data == '[HEARTBEAT]') {
            return;
        }

        // remove prefix [HEARTBEAT]
        evt.data = evt.data.replace(/^\[HEARTBEAT\]+/, '');

        if (!isChatRespDone) {
            const payload = JSON.parse(evt.data);
            const respContent = parseChatResp(selectedModel, payload);

            if (payload.choices[0].finish_reason) {
                isChatRespDone = true;
            }

            switch (globalAIRespEle.dataset.status) {
            case 'waiting':
                globalAIRespEle.dataset.status = 'writing';

                if (respContent) {
                    globalAIRespEle.innerHTML = respContent;
                    aiRawResp += respContent;
                } else {
                    globalAIRespEle.innerHTML = '';
                }

                break;
            case 'writing':
                if (respContent) {
                    aiRawResp += respContent;
                    globalAIRespEle.innerHTML = Markdown2HTML(aiRawResp);
                }

                scrollToChat(globalAIRespEle);
                break;
            }
        }

        if (isChatRespDone) {
            if (!globalAIRespSSE) {
                return;
            }

            globalAIRespSSE.close();
            globalAIRespSSE = null;

            globalAIRespEle.innerHTML = Markdown2HTML(aiRawResp);
            globalAIRespEle.innerHTML += responseExtras;
            globalAIRespEle
                .insertAdjacentHTML('afterbegin', `<i class="bi bi-copy" data-content="${encodeURIComponent(aiRawResp)}" data-bs-toggle="tooltip" data-bs-placement="top" title="copy raw"></i>`);

            // setup prism
            {
                // add line number
                globalAIRespEle.querySelectorAll('pre').forEach((item) => {
                    item.classList.add('line-numbers');
                });
            }

            // should save html before prism formatted,
            // because prism.js do not support formatted html.
            const markdownContent = globalAIRespEle.innerHTML;

            renderAfterAIResponse(chatID);

            scrollToChat(globalAIRespEle);
            await appendChats2Storage(RoleAI, chatID, markdownContent, null, aiRawResp);
            unlockChatInput();
        }
    })

    globalAIRespSSE.onerror = (err) => {
        abortAIResp(err);
    };
    globalAIRespSSE.stream();
}

/** append chat to chat conservation window
 *
 * @param {string} chatID - chat id
 */
function renderAfterAIResponse (chatID) {
    // Prism.highlightAll();
    const chatEle = chatContainer.querySelector(`.chatManager .conservations .chats #${chatID} .ai-response`)

    if (!chatEle) {
        return
    }

    Prism.highlightAllUnder(chatEle)
    EnableTooltipsEverywhere()

    if (chatEle.querySelector('.bi.bi-copy')) { // not every ai response has copy button
        chatEle.querySelector('.bi.bi-copy')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation()
                const content = decodeURIComponent(evtTarget(evt).dataset.content)
                // copy to clipboard
                navigator.clipboard.writeText(content)
            })
    }
}

function combineRefs (arr) {
    let markdown = ''
    for (const val of arr) {
        if (val.startsWith('https') || val.startsWith('http')) {
            // markdown += `- <${val}>\n`;
            markdown += `<li><a href="${val}">${decodeURIComponent(val)}</li>`
        } else { // sometimes refs are just plain text, not url
            // markdown += `- \`${val}\`\n`;
            markdown += `<li><p>${val}</p></li>`
        }
    }

    return `<ul style="margin-bottom: 0;">${markdown}</ul>`
}

// parse langchain qa references to markdown links
// function wrapRefLines (input) {
//     const lines = input.split('\n')
//     let result = ''
//     for (let i = 0; i < lines.length; i++) {
//         // skip empty lines
//         if (lines[i].trim() == '') {
//             continue
//         }

//         result += `* <${lines[i]}>\n`
//     }
//     return result
// }

function abortAIResp (err) {
    console.error(`abort AI resp: ${err}`);
    if (globalAIRespSSE) {
        globalAIRespSSE.close();
        globalAIRespSSE = null;
    }

    let errMsg;
    if (err.data) {
        errMsg = err.data;
    } else {
        errMsg = err.toString();
    }

    if (errMsg == '[object CustomEvent]') {
        if (navigator.userAgent.includes('Firefox')) {
            // firefox will throw this error when SSE is closed, just ignore it.
            return;
        }

        errMsg = 'There may be a network issue, please check the network connection and try again later.';
    }

    if (typeof errMsg !== 'string') {
        errMsg = JSON.stringify(errMsg);
    }

    // if errMsg contains
    if (errMsg.includes('Access denied due to invalid subscription key or wrong API endpoint')) {
        showalert('danger', 'API TOKEN invalid, please ask admin to get new token.\nAPI TOKEN 无效，请联系管理员获取新的 API TOKEN。');
    }

    if (globalAIRespEle.dataset.status == 'waiting') { // || currentAIRespEle.dataset.status == "writing") {
        globalAIRespEle.innerHTML = `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${RenderStr2HTML(errMsg)}</pre>`;
    } else {
        globalAIRespEle.innerHTML += `<p>🔥Someting in trouble...</p><pre style="background-color: #f8e8e8; text-wrap: pretty;">${RenderStr2HTML(errMsg)}</pre>`;
    }

    // ScrollDown(chatContainer.querySelector(".chatManager .conservations .chats"));
    globalAIRespEle.scrollIntoView({ behavior: 'smooth' });
    appendChats2Storage(RoleAI, globalAIRespEle.closest('.role-ai').dataset.chatid, globalAIRespEle.innerHTML);
    unlockChatInput();
}

async function bindUserInputSelectFilesBtn () {
    chatContainer.querySelector('.user-input .btn.upload')
        .addEventListener('click', async (evt) => {
            // click to select images
            evt.stopPropagation();

            const inputEle = document.createElement('input');
            inputEle.type = 'file';
            inputEle.multiple = true;
            inputEle.accept = 'image/*';

            inputEle.addEventListener('change', async (evt) => {
                const files = evtTarget(evt).files;
                for (const file of files) {
                    readFileForVision(file);
                }
            });

            inputEle.click();
        });
}

/** auto display or hide user input select files button according to selected model
 *
 */
async function autoToggleUserImageUploadBtn () {
    const sconfig = await getChatSessionConfig();
    const isVision = sconfig.selected_model.includes('vision') ||
        sconfig.selected_model === ImageModelImg2Img;

    const btnEle = chatContainer.querySelector('.user-input .btn.upload');
    if ((isVision && btnEle) || (!isVision && !btnEle)) {
        // everything is ok
        return;
    }

    const uploadEleHtml = '<button class="btn btn-outline-secondary upload" type="button"><i class="bi bi-images"></i></button>';
    if (isVision) {
        chatPromptInputBtn.insertAdjacentHTML('beforebegin', uploadEleHtml);
        bindUserInputSelectFilesBtn();
    } else {
        btnEle.remove();
    }
}

async function setupChatInput () {
    // bind input press enter
    {
        let isComposition = false
        chatPromptInputEle
            .addEventListener('compositionstart', async (evt) => {
                evt.stopPropagation();
                isComposition = true;
            });
        chatPromptInputEle
            .addEventListener('compositionend', async (evt) => {
                evt.stopPropagation();
                isComposition = false;
            });

        chatPromptInputEle
            .addEventListener('keydown', async (evt) => {
                evt.stopPropagation();
                if (evt.key != 'Enter' ||
                    isComposition ||
                    (evt.key == 'Enter' && !(evt.ctrlKey || evt.metaKey || evt.altKey)) ||
                    !isAllowChatPrompInput()) {
                    return;
                }

                await sendChat2Server();
                chatPromptInputEle.value = '';
            });
    }

    // change hint when models change
    {
        KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
            if (op != KvOp.SET) {
                return;
            }

            const expectedKey = `KvKeyPrefixSessionConfig${(await activeSessionID())}`;
            if (key != expectedKey) {
                return;
            }

            const sconfig = newVal;
            chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;
        });
    }

    // bind input button
    chatPromptInputBtn
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();
            await sendChat2Server();
            chatPromptInputEle.value = '';
        });

    // bindImageUploadButton
    await autoToggleUserImageUploadBtn();
    KvAddListener(KvKeyPrefixSessionConfig, async (key, op, oldVal, newVal) => {
        if (op != KvOp.SET) {
            return;
        }

        await autoToggleUserImageUploadBtn();
    });

    // restore pinned materials
    await restorePinnedMaterials();

    // bind input element's drag-drop
    {
        const dropfileModalEle = document.querySelector('#modal-dropfile.modal');
        const dropfileModal = new bootstrap.Modal(dropfileModalEle);

        const fileDragLeave = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();
        };

        const fileDragDropHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            dropfileModal.hide();

            if (!evt.dataTransfer || !evt.dataTransfer.items) {
                return;
            }

            for (let i = 0; i < evt.dataTransfer.items.length; i++) {
                const item = evt.dataTransfer.items[i];
                if (item.kind != 'file') {
                    continue;
                }

                const file = item.getAsFile();
                if (!file) {
                    continue;
                }

                // get file content as Blob
                readFileForVision(file);
            }
        };

        const fileDragOverHandler = async (evt) => {
            evt.stopPropagation();
            evt.preventDefault();
            evt.dataTransfer.dropEffect = 'copy'; // Explicitly show this is a copy.
            dropfileModal.show();
        };

        chatPromptInputEle.addEventListener('paste', filePasteHandler);

        document.body.addEventListener('dragover', fileDragOverHandler);
        document.body.addEventListener('drop', fileDragDropHandler);
        document.body.addEventListener('paste', filePasteHandler);

        dropfileModalEle.addEventListener('drop', fileDragDropHandler);
        dropfileModalEle.addEventListener('dragleave', fileDragLeave);
    }

    // bind chat switch
    {
        chatContainer
            .querySelector('#switchChatEnableHttpsCrawler')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                const switchEle = evtTarget(evt);
                const sconfig = await getChatSessionConfig();
                sconfig.chat_switch.disable_https_crawler = !switchEle.checked;

                // clear pinned https urls
                if (!switchEle.checked) {
                    await KvSet(KvKeyPinnedMaterials, '');
                    await await restorePinnedMaterials();
                }

                await saveChatSessionConfig(sconfig);
            });

        chatContainer
            .querySelector('#switchChatEnableGoogleSearch')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = evtTarget(evt)
                const sconfig = await getChatSessionConfig()
                sconfig.chat_switch.enable_google_search = switchEle.checked
                await saveChatSessionConfig(sconfig)
            })

        chatContainer
            .querySelector('#switchChatEnableAutoSync')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation()
                const switchEle = evtTarget(evt)
                await KvSet(KvKeyAutoSyncUserConfig, switchEle.checked);
            });
        let userConfigSyncer;
        KvAddListener(KvKeyAutoSyncUserConfig, async (key, op, oldVal, newVal) => {
            if (op !== KvOp.SET) {
                return;
            }

            // update ui
            const switchEle = chatContainer.querySelector('#switchChatEnableAutoSync');
            switchEle.checked = newVal;

            // update background syncer
            if (!newVal) {
                console.debug('stop user config syncer');
                if (userConfigSyncer) {
                    clearTimeout(userConfigSyncer);
                    userConfigSyncer = null;
                }

                return;
            }

            if (userConfigSyncer) {
                return;
            }

            console.debug('start user config syncer');
            // await syncUserConfig();
            userConfigSyncer = setTimeout(async () => {
                await syncUserConfig();
            }, 1800 * 1000);
        });
    }
}

// read paste file
async function filePasteHandler (evt) {
    if (!evt.clipboardData || !evt.clipboardData.items) {
        return;
    }

    for (let i = 0; i < evt.clipboardData.items.length; i++) {
        const item = evt.clipboardData.items[i];
        if (item.kind != 'file') {
            continue;
        }

        const file = item.getAsFile();
        if (!file) {
            continue;
        }

        evt.stopPropagation();
        evt.preventDefault();

        // get file content as Blob
        readFileForVision(file, `paste-${DateStr()}.png`);
    }
};

/** read file content and append to vision store
 *
 * @param {*} file - file object
 * @param {*} rewriteName - rewrite file name
 */
function readFileForVision (file, rewriteName) {
    const filename = rewriteName || file.name;

    // get file content as Blob
    const reader = new FileReader();
    reader.onload = async (e) => {
        const arrayBuffer = e.target.result;
        if (arrayBuffer.byteLength > 1024 * 1024 * 10) {
            showalert('danger', 'file size should less than 10M');
            return;
        }

        const byteArray = new Uint8Array(arrayBuffer);
        const chunkSize = 0xffff; // Use chunks to avoid call stack limit
        const chunks = [];
        for (let i = 0; i < byteArray.length; i += chunkSize) {
            chunks.push(String.fromCharCode.apply(null, byteArray.subarray(i, i + chunkSize)));
        }
        const base64String = btoa(chunks.join(''));

        // only support 5 image for current version
        if (chatVisionSelectedFileStore.length > 5) {
            showalert('warning', 'only support 5 images for current version');
            return;
        }

        // check duplicate
        for (const item of chatVisionSelectedFileStore) {
            if (item.contentB64 === base64String) {
                return;
            }
        }

        chatVisionSelectedFileStore.push({
            filename,
            contentB64: base64String
        });
        updateChatVisionSelectedFileStore();
    };

    reader.readAsArrayBuffer(file);
}

async function updateChatVisionSelectedFileStore () {
    const pinnedFiles = chatContainer.querySelector('.pinned-files');
    pinnedFiles.innerHTML = '';
    for (const item of chatVisionSelectedFileStore) {
        pinnedFiles.insertAdjacentHTML('beforeend', `<p data-key="${item.filename}"><i class="bi bi-trash"></i> ${item.filename}</p>`);
    }

    // click to remove pinned file
    chatContainer.querySelectorAll('.pinned-files .bi.bi-trash')
        .forEach((item) => {
            item.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const ele = evtTarget(evt).closest('p');
                const key = ele.dataset.key;
                chatVisionSelectedFileStore = chatVisionSelectedFileStore.filter((item) => item.filename !== key);
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
 * @param {string} rawAiResp - raw ai response
 */
async function append2Chats (chatID, role, text, isHistory = false, attachHTML, rawAiResp) {
    if (!chatID) {
        throw new Error('chatID is required');
    }

    const robotIcon = '🤖️';
    let chatEleHtml;
    let chatOp = 'append';
    let waitAI = '';
    switch (role) {
    case RoleSystem:
        text = escapeHtml(text);

        chatEleHtml = `
            <div class="container-fluid row role-human">
                <div class="col-auto icon">💻</div>
                <div class="col text-start"><pre>${text}</pre></div>
            </div>`;
        break;
    case RoleHuman:
        text = escapeHtml(text);
        if (!isHistory) {
            waitAI = `
                        <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                            <div class="col-auto icon">${robotIcon}</div>
                            <div class="col text-start ai-response" data-status="waiting">
                                <p dir="auto" class="card-text placeholder-glow">
                                    <span class="placeholder col-7"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-4"></span>
                                    <span class="placeholder col-6"></span>
                                    <span class="placeholder col-8"></span>
                                </p>
                            </div>
                        </div>`;
        }

        if (attachHTML) {
            attachHTML = `${attachHTML}`;
        } else {
            attachHTML = '';
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
                </div>`;
        break;
    case RoleAI:
        // let insertText;
        // if (rawAiResp) {
        //     insertText = `<i class="bi bi-copy" data-content="${rawAiResp}"></i>${text}`
        // }else {
        //     insertText = text
        // }

        chatEleHtml = `
                <div class="container-fluid row role-ai" style="background-color: #f4f4f4;" data-chatid="${chatID}">
                        <div class="col-auto icon">${robotIcon}</div>
                        <div class="col text-start ai-response" data-status="waiting">
                            ${text}
                        </div>
                </div>`;
        if (!isHistory) {
            chatOp = 'replace';
        }

        break;
    }

    if (chatOp === 'append') {
        if (role === RoleAI) {
            // ai response is always after human, so we need to find the last human chat,
            // and append ai response after it
            chatContainer.querySelector(`#${chatID}`)
                .insertAdjacentHTML('beforeend', chatEleHtml);
        } else {
            chatContainer.querySelector('.chatManager .conservations .chats')
                .insertAdjacentHTML('beforeend', chatEleHtml);
        }
    }

    const chatEle = chatContainer
        .querySelector(`.chatManager .conservations .chats #${chatID}`);
    if (chatOp === 'replace') {
        // replace html element of ai
        chatEle.querySelector('.role-ai')
            .outerHTML = chatEleHtml;
    }

    if (!isHistory && role === RoleHuman) {
        scrollToChat(chatEle);
    }

    // avoid duplicate event listener, only bind event listener for new chat
    if (role === RoleHuman) {
        // bind delete button
        const deleteBtnHandler = (evt) => {
            evt.stopPropagation();

            ConfirmModal('Are you sure to delete this chat?', async () => {
                chatEle.parentNode.removeChild(chatEle);
                removeChatInStorage(chatID);
            });
        };

        const editHumanInputHandler = (evt) => {
            evt.stopPropagation();

            const oldText = chatContainer.querySelector(`#${chatID}`).innerHTML;
            let text = chatContainer.querySelector(`#${chatID} .role-human .text-start pre`).innerHTML;

            // attach image to vision-selected-store when edit human input
            const attachEles = chatContainer
                .querySelectorAll(`.chatManager .conservations .chats #${chatID} .role-human .text-start img`) || [];
            let attachHTML = '';
            attachEles.forEach((ele) => {
                const b64fileContent = ele.getAttribute('src').replace('data:image/png;base64,', '');
                const key = ele.dataset.name || `${DateStr()}.png`;
                chatVisionSelectedFileStore.push({
                    filename: key,
                    contentB64: b64fileContent
                });
                attachHTML += `<img src="data:image/png;base64,${b64fileContent}" data-name="${key}">`;
            })
            updateChatVisionSelectedFileStore();

            text = sanitizeHTML(text);
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

            const saveBtn = chatEle.querySelector('.role-human .btn.save');
            const cancelBtn = chatEle.querySelector('.role-human .btn.cancel');
            saveBtn.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const newText = chatEle.querySelector('.role-human textarea').value;
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
                        <div class="col-auto icon">${robotIcon}</div>
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
                chatEle.querySelector('.role-ai').dataset.status = 'waiting';

                // bind delete and edit button
                chatEle.querySelector('.role-human .bi-trash')
                    .addEventListener('click', deleteBtnHandler);
                chatEle.querySelector('.bi.bi-pencil-square')
                    .addEventListener('click', editHumanInputHandler);

                await sendChat2Server(chatID);
                await appendChats2Storage(RoleHuman, chatID, newText, attachHTML);
            });

            cancelBtn.addEventListener('click', async (evt) => {
                evt.stopPropagation();
                chatEle.innerHTML = oldText;

                // bind delete and edit button
                chatEle.querySelector('.role-human .bi-trash')
                    .addEventListener('click', deleteBtnHandler);
                chatEle.querySelector('.bi.bi-pencil-square')
                    .addEventListener('click', editHumanInputHandler);
            });
        };

        // bind delete and edit button
        chatEle.querySelector('.role-human .bi-trash')
            .addEventListener('click', deleteBtnHandler);
        chatEle.querySelector('.bi.bi-pencil-square')
            .addEventListener('click', editHumanInputHandler);
    }
}

const getChatSessionConfig = async (sid) => {
    if (!sid) {
        sid = await activeSessionID();
    }

    const skey = `${KvKeyPrefixSessionConfig}${sid}`;
    let sconfig = await KvGet(skey);

    if (!sconfig) {
        console.info(`create new session config for session ${sid}`);
        sconfig = newSessionConfig();
    }

    return sconfig;
};

const saveChatSessionConfig = async (sconfig) => {
    const sid = await activeSessionID();
    const skey = `${KvKeyPrefixSessionConfig}${sid}`;

    await KvSet(skey, sconfig);
};

function newSessionConfig () {
    return {
        api_token: 'FREETIER-' + RandomString(32),
        api_base: 'https://api.openai.com',
        max_tokens: 500,
        temperature: 1,
        presence_penalty: 0,
        frequency_penalty: 0,
        n_contexts: 6,
        system_prompt: "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can.",
        selected_model: ChatModelTurbo35V1106,
        chat_switch: {
            disable_https_crawler: true,
            enable_google_search: false
        }
    }
}

/**
 * initialize every chat component by active session config
 */
async function updateConfigFromSessionConfig () {
    console.debug(`updateConfigFromSessionConfig for session ${(await activeSessionID())}`);

    const sconfig = await getChatSessionConfig();

    // update config
    configContainer.querySelector('.input.api-token').value = sconfig.api_token;
    configContainer.querySelector('.input.api-base').value = sconfig.api_base;
    configContainer.querySelector('.input.contexts').value = sconfig.n_contexts;
    configContainer.querySelector('.input-group.contexts .contexts-val').innerHTML = sconfig.n_contexts;
    configContainer.querySelector('.input.max-token').value = sconfig.max_tokens;
    configContainer.querySelector('.input-group.max-token .max-token-val').innerHTML = sconfig.max_tokens;
    configContainer.querySelector('.input.temperature').value = sconfig.temperature;
    configContainer.querySelector('.input-group.temperature .temperature-val').innerHTML = sconfig.temperature;
    configContainer.querySelector('.input.presence_penalty').value = sconfig.presence_penalty;
    configContainer.querySelector('.input-group.presence_penalty .presence_penalty-val').innerHTML = sconfig.presence_penalty;
    configContainer.querySelector('.input.frequency_penalty').value = sconfig.frequency_penalty;
    configContainer.querySelector('.input-group.frequency_penalty .frequency_penalty-val').innerHTML = sconfig.frequency_penalty;
    configContainer.querySelector('.system-prompt .input').value = sconfig.system_prompt;

    // update chat input hint
    chatPromptInputEle.attributes.placeholder.value = `[${sconfig.selected_model}] CTRL+Enter to send`;

    // update chat controller
    chatContainer.querySelector('#switchChatEnableHttpsCrawler')
        .checked = !sconfig.chat_switch.disable_https_crawler;
    chatContainer.querySelector('#switchChatEnableGoogleSearch')
        .checked = sconfig.chat_switch.enable_google_search;
    chatContainer.querySelector('#switchChatEnableAutoSync')
        .checked = await KvGet(KvKeyAutoSyncUserConfig);

    // update selected model
    // set active status for models
    const selectedModel = sconfig.selected_model;
    document.querySelectorAll('#headerbar .navbar-nav a.dropdown-toggle')
        .forEach((elem) => {
            elem.classList.remove('active');
        });
    document
        .querySelectorAll('#headerbar .chat-models li a, ' +
            '#headerbar .qa-models li a, ' +
            '#headerbar .image-models li a'
        )
        .forEach((elem) => {
            elem.classList.remove('active');

            if (elem.dataset.model == selectedModel) {
                elem.classList.add('active');
                elem.closest('.dropdown').querySelector('a.dropdown-toggle').classList.add('active');
            }
        });
}

async function setupConfig () {
    await updateConfigFromSessionConfig();

    //  config_api_token_value
    {
        const apitokenInput = configContainer
            .querySelector('.input.api-token');
        apitokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.api_token = evtTarget(evt).value;
            await KvSet(skey, sconfig);
        });
    }

    // bind api_base
    {
        const apibaseInput = configContainer
            .querySelector('.input.api-base');
        apibaseInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.api_base = evtTarget(evt).value;
            await KvSet(skey, sconfig);
        });
    }

    //  config_chat_n_contexts
    {
        const maxtokenInput = configContainer
            .querySelector('.input.contexts');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.n_contexts = evtTarget(evt).value;
            await KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.contexts .contexts-val').innerHTML = evtTarget(evt).value;
        });
    }

    //  config_api_max_tokens
    {
        const maxtokenInput = configContainer
            .querySelector('.input.max-token');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.max_tokens = evtTarget(evt).value;
            await KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.max-token .max-token-val').innerHTML = evtTarget(evt).value;
        });
    }

    //  config_api_temperature
    {
        const maxtokenInput = configContainer
            .querySelector('.input.temperature');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`
            const sconfig = await KvGet(skey);

            sconfig.temperature = evtTarget(evt).value;
            await KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.temperature .temperature-val').innerHTML = evtTarget(evt).value;
        });
    }

    //  config_api_presence_penalty
    {
        const maxtokenInput = configContainer
            .querySelector('.input.presence_penalty');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.presence_penalty = evtTarget(evt).value;
            await KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.presence_penalty .presence_penalty-val').innerHTML = evtTarget(evt).value;
        });
    }

    //  config_api_frequency_penalty
    {
        const maxtokenInput = configContainer
            .querySelector('.input.frequency_penalty');
        maxtokenInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.frequency_penalty = evtTarget(evt).value;
            await KvSet(skey, sconfig);

            configContainer.querySelector('.input-group.frequency_penalty .frequency_penalty-val').innerHTML = evtTarget(evt).value;
        });
    }

    //  config_api_static_context
    {
        const staticConfigInput = configContainer
            .querySelector('.system-prompt .input');
        staticConfigInput.addEventListener('input', async (evt) => {
            evt.stopPropagation();

            const sid = await activeSessionID();
            const skey = `${KvKeyPrefixSessionConfig}${sid}`;
            const sconfig = await KvGet(skey);

            sconfig.system_prompt = evtTarget(evt).value;
            await KvSet(skey, sconfig);
        });
    }

    // bind reset button
    {
        configContainer.querySelector('.btn.reset')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                ConfirmModal('Reset everything?', async () => {
                    localStorage.clear();
                    await KvClear();
                    location.reload();
                });
            });
    }

    // bind clear-chats button
    {
        configContainer.querySelector('.btn.clear-chats')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                ConfirmModal('Clear all chat records in the current session?', async () => {
                    clearSessionAndChats(evt, await activeSessionID());
                });
            });
    }

    // bind submit button
    {
        configContainer.querySelector('.btn.submit')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                location.reload();
            });
    }

    // bind sync key
    {
        const syncKeyEle = configContainer.querySelector('.input.sync-key');
        syncKeyEle
            .addEventListener('input', async (evt) => {
                evt.stopPropagation();
                const syncKey = evtTarget(evt).value;
                await KvSet(KvKeySyncKey, syncKey);
            });
        KvAddListener(KvKeySyncKey, async (key, op, oldVal, newVal) => {
            if (op != KvOp.SET) {
                return;
            }

            syncKeyEle.value = newVal;
        });

        // set default val
        if (!(await KvGet(KvKeySyncKey))) {
            await KvSet(KvKeySyncKey, `sync-${RandomString(64)}`);
        }
        syncKeyEle.value = await KvGet(KvKeySyncKey);
    }

    // bind upload & download configs
    {
        configContainer.querySelector('.btn[data-app-fn="cloud-sync"]')
            .addEventListener('click', async (evt) => {
                try {
                    ShowSpinner();
                    await syncUserConfig(evt);
                    location.reload();
                } catch (err) {
                    console.error(err);
                    showalert('danger', `sync user config failed: ${err}`);
                } finally {
                    HideSpinner();
                }
            });

        // configContainer.querySelector('.btn-upload')
        //     .addEventListener('click', uploadUserConfig);

        // configContainer.querySelector('.btn-download')
        //     .addEventListener('click', downloadUserConfig);
    }

    EnableTooltipsEverywhere();
}

async function syncUserConfig (evt) {
    await downloadUserConfig(evt);
    await uploadUserConfig(evt);
}

async function uploadUserConfig (evt) {
    console.debug('uploadUserConfig');
    evt && evt.stopPropagation();

    const data = {};
    await Promise.all((await KvList()).map(async (key) => {
        data[key] = await KvGet(key);
    }));

    const syncKey = await KvGet(KvKeySyncKey);
    const resp = await fetch('/user/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-LAISKY-SYNC-KEY': syncKey
        },
        body: JSON.stringify(data)
    });

    if (resp.status !== 200) {
        throw new Error(`upload config failed: ${resp.status}`);
    }
}

async function downloadUserConfig (evt) {
    console.debug('downloadUserConfig');
    evt && evt.stopPropagation();

    const syncKey = await KvGet(KvKeySyncKey);
    const resp = await fetch('/user/config', {
        method: 'GET',
        headers: {
            'X-LAISKY-SYNC-KEY': syncKey,
            'Cache-Control': 'no-cache'
        }
    });

    if (resp.status !== 200) {
        if (resp.status === 400) {
            return;
        }

        throw new Error(`download config failed: ${resp.status}`);
    }

    const data = await resp.json();
    for (const key in data) {
        // download non-exists key
        if (!(await KvExists(key))) {
            await KvSet(key, data[key]);
            continue;
        }

        // only update session config with different session name
        if (key.startsWith(KvKeyPrefixSessionConfig)) {
            const localConfig = await KvGet(key);
            if (localConfig.session_name !== data[key].session_name) {
                await KvSet(key, data[key]);
                continue;
            }
        }

        // incremental update local sessions chat history
        if (key.startsWith(KvKeyPrefixSessionHistory)) {
            let localHistory = await KvGet(key);
            let iLocal = 0;
            let iRemote = 0;
            let localChatId = 0;
            let remoteChatId = 0;
            let localChatNum = 0;
            let remoteChatNum = 0;
            while (iLocal < localHistory.length || iRemote < data[key].length) {
                if (iLocal >= localHistory.length) {
                    localHistory = localHistory.concat(data[key].slice(iRemote));
                    break;
                }
                if (iRemote >= data[key].length) {
                    break;
                }

                localChatId = localHistory[iLocal].chatID;
                // latest version's chatid like: chat-1705899120122-NwN9sB
                if (!localChatId || !localChatId.match(/chat-\d+-\w+/)) {
                    iLocal++;
                    continue;
                }

                localChatNum = parseInt(localChatId.split('-')[1]);

                while (iRemote < data[key].length) {
                    remoteChatId = data[key][iRemote].chatID;
                    if (!remoteChatId || !remoteChatId.match(/chat-\d+-\w+/)) {
                        localHistory.splice(iLocal - 1, 0, data[key][iRemote]);
                        iLocal++;
                        iRemote++;
                        continue;
                    }

                    remoteChatNum = parseInt(remoteChatId.split('-')[1]);
                    break;
                }

                if (iRemote >= data[key].length) {
                    break;
                }

                // skip same chat
                if (localChatNum === remoteChatNum) {
                    while (iLocal < localHistory.length && localHistory[iLocal].chatID === localChatId) {
                        iLocal++;
                    }

                    while (iRemote < data[key].length && data[key][iRemote].chatID === remoteChatId) {
                        iRemote++;
                    }

                    continue;
                }

                // insert remote chat into local by chat num
                if (localChatNum > remoteChatNum) {
                    // insert before
                    localHistory.splice(iLocal - 1, 0, data[key][iRemote]);
                    iRemote++;
                }

                iLocal++;
            }

            await KvSet(key, localHistory);
        }
    }
}

async function loadPromptShortcutsFromStorage () {
    let shortcuts = await KvGet(KvKeyPromptShortCuts);
    if (!shortcuts) {
        // default prompts
        shortcuts = [
            {
                title: '中英互译',
                description: 'As an English-Chinese translator, your task is to accurately translate text between the two languages. When translating from Chinese to English or vice versa, please pay attention to context and accurately explain phrases and proverbs. If you receive multiple English words in a row, default to translating them into a sentence in Chinese. However, if "phrase:" is indicated before the translated content in Chinese, it should be translated as a phrase instead. Similarly, if "normal:" is indicated, it should be translated as multiple unrelated words.Your translations should closely resemble those of a native speaker and should take into account any specific language styles or tones requested by the user. Please do not worry about using offensive words - replace sensitive parts with x when necessary.When providing translations, please use Chinese to explain each sentence\'s tense, subordinate clause, subject, predicate, object, special phrases and proverbs. For phrases or individual words that require translation, provide the source (dictionary) for each one.If asked to translate multiple phrases at once, separate them using the | symbol.Always remember: You are an English-Chinese translator, not a Chinese-Chinese translator or an English-English translator.Please review and revise your answers carefully before submitting.'
            }
        ];
        await KvSet(KvKeyPromptShortCuts, shortcuts);
    }

    return shortcuts;
}

// append prompt shortcuts to html and kv
//
// @param {Object} shortcut - shortcut object
// @param {bool} storage - whether to save to kv
async function appendPromptShortcut (shortcut, storage = false) {
    const promptShortcutContainer = configContainer.querySelector('.prompt-shortcuts');

    // add to local storage
    if (storage) {
        const shortcuts = await loadPromptShortcutsFromStorage();
        shortcuts.push(shortcut);
        await KvSet(KvKeyPromptShortCuts, shortcuts);
    }

    // new element
    const ele = document.createElement('span');
    ele.classList.add('badge', 'text-bg-info');
    ele.dataset.prompt = shortcut.description;
    ele.innerHTML = ` ${shortcut.title}  <i class="bi bi-trash"></i>`;

    // add delete click event
    ele.querySelector('i.bi-trash')
        .addEventListener('click', async (evt) => {
            evt.stopPropagation();

            ConfirmModal('delete saved prompt', async () => {
                evtTarget(evt).parentElement.remove();

                let shortcuts = await KvGet(KvKeyPromptShortCuts);
                shortcuts = shortcuts.filter((item) => item.title !== shortcut.title);
                await KvSet(KvKeyPromptShortCuts, shortcuts);
            })
        });

    // add click event
    // replace system prompt
    ele.addEventListener('click', async (evt) => {
        evt.stopPropagation();
        const promptInput = configContainer.querySelector('.system-prompt .input');

        await OpenaiChatStaticContext(evtTarget(evt).dataset.prompt);
        promptInput.value = evtTarget(evt).dataset.prompt;
    });

    // add to html
    promptShortcutContainer.appendChild(ele);
}

async function setupPromptManager () {
    // restore shortcuts from kv
    {
        // bind default prompt shortcuts
        configContainer
            .querySelector('.prompt-shortcuts .badge')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptInput = configContainer.querySelector('.system-prompt .input');
                promptInput.value = evtTarget(evt).dataset.prompt;
                await OpenaiChatStaticContext(evtTarget(evt).dataset.prompt);
            })

        const shortcuts = await loadPromptShortcutsFromStorage();
        await Promise.all(shortcuts.map(async (shortcut) => {
            await appendPromptShortcut(shortcut, false);
        }));
    }

    // bind star prompt
    const saveSystemPromptModelEle = document.querySelector('#save-system-prompt.modal');
    const saveSystemPromptModal = new bootstrap.Modal(saveSystemPromptModelEle);
    {
        configContainer
            .querySelector('.system-prompt .bi.save-prompt')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptInput = configContainer
                    .querySelector('.system-prompt .input');

                saveSystemPromptModelEle
                    .querySelector('.modal-body textarea.user-input')
                    .innerHTML = promptInput.value;

                saveSystemPromptModal.show();
            });
    }

    // bind prompt market modal
    {
        configContainer
            .querySelector('.system-prompt .bi.open-prompt-market')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const promptMarketModalEle = document.querySelector('#prompt-market.modal');
                const promptMarketModal = new bootstrap.Modal(promptMarketModalEle);
                promptMarketModal.show();
            });
    }

    // bind save button in system-prompt modal
    {
        saveSystemPromptModelEle
            .querySelector('.btn.save')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                const titleInput = saveSystemPromptModelEle
                    .querySelector('.modal-body input.title');
                const descriptionInput = saveSystemPromptModelEle
                    .querySelector('.modal-body textarea.user-input');

                // trim space
                titleInput.value = titleInput.value.trim();
                descriptionInput.value = descriptionInput.value.trim();

                // if title is empty, set input border to red
                if (titleInput.value === '') {
                    titleInput.classList.add('border-danger');
                    return;
                }

                const shortcut = {
                    title: titleInput.value,
                    description: descriptionInput.value
                };

                appendPromptShortcut(shortcut, true);

                // clear input
                titleInput.value = '';
                descriptionInput.value = '';
                titleInput.classList.remove('border-danger');
                saveSystemPromptModal.hide();
            })
    }

    // fill chat prompts market
    const promptMarketModal = document.querySelector('#prompt-market');
    const promptInput = promptMarketModal.querySelector('textarea.prompt-content');
    const promptTitle = promptMarketModal.querySelector('input.prompt-title');
    {
        chatPrompts.forEach((prompt) => {
            const ele = document.createElement('span');
            ele.classList.add('badge', 'text-bg-info');
            ele.dataset.description = prompt.description;
            ele.dataset.title = prompt.title;
            ele.innerHTML = ` ${prompt.title}  <i class="bi bi-plus-circle"></i>`;

            // add click event
            // replace system prompt
            ele.addEventListener('click', async (evt) => {
                evt.stopPropagation();

                promptInput.value = evtTarget(evt).dataset.description;
                promptTitle.value = evtTarget(evt).dataset.title;
            });

            promptMarketModal.querySelector('.prompt-labels').appendChild(ele);
        });
    }

    // bind chat prompts market add button
    {
        promptMarketModal.querySelector('.modal-body .save')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                // trim and check empty
                promptTitle.value = promptTitle.value.trim();
                promptInput.value = promptInput.value.trim();
                if (promptTitle.value === '') {
                    promptTitle.classList.add('border-danger');
                    return;
                }
                if (promptInput.value === '') {
                    promptInput.classList.add('border-danger');
                    return;
                }

                const shortcut = {
                    title: promptTitle.value,
                    description: promptInput.value
                };

                appendPromptShortcut(shortcut, true);

                promptTitle.value = '';
                promptInput.value = '';
                promptTitle.classList.remove('border-danger');
                promptInput.classList.remove('border-danger');
            });
    }
}

// setup private dataset modal
async function setupPrivateDataset () {
    const pdfchatModalEle = document.querySelector('#modal-pdfchat');

    // bind header's custom qa button
    {
        // bind pdf-file modal
        const pdfFileModalEle = document.querySelector('#modal-pdfchat');
        const pdfFileModal = new bootstrap.Modal(pdfFileModalEle);

        document
            .querySelector('#headerbar .qa-models a[data-model="qa-custom"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                pdfFileModal.show();
            });
    }

    // bind datakey to kv
    {
        const datakeyEle = pdfchatModalEle
            .querySelector('div[data-field="data-key"] input');

        datakeyEle.value = await KvGet(KvKeyCustomDatasetPassword);

        // set default datakey
        if (!datakeyEle.value) {
            datakeyEle.value = RandomString(16);
            await KvSet(KvKeyCustomDatasetPassword, datakeyEle.value);
        }

        datakeyEle
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();
                await KvSet(KvKeyCustomDatasetPassword, evtTarget(evt).value);
            });
    }

    // bind file upload
    {
        // when user choosen file, get file name of
        // pdfchatModalEle.querySelector('div[data-field="pdffile"] input').files[0]
        // and set to dataset-name input
        pdfchatModalEle
            .querySelector('div[data-field="pdffile"] input')
            .addEventListener('change', async (evt) => {
                evt.stopPropagation();

                if (evtTarget(evt).files.length === 0) {
                    return;
                }

                let filename = evtTarget(evt).files[0].name;
                const fileext = filename.substring(filename.lastIndexOf('.')).toLowerCase();

                if (['.pdf', '.md', '.ppt', '.pptx', '.doc', '.docx'].indexOf(fileext) === -1) {
                    // remove choosen
                    pdfchatModalEle
                        .querySelector('div[data-field="pdffile"] input').value = '';

                    showalert('warning', 'currently only support pdf file');
                    return;
                }

                // remove extension and non-ascii charactors
                filename = filename.substring(0, filename.lastIndexOf('.'));
                filename = filename.replace(/[^a-zA-Z0-9]/g, '_');

                pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input')
                    .value = filename;
            });

        // bind upload button
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="upload"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                if (pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files.length === 0) {
                    showalert('warning', 'please choose a pdf file before upload');
                    return;
                }

                const sconfig = await getChatSessionConfig();

                // build post form
                const form = new FormData();
                form.append('file', pdfchatModalEle
                    .querySelector('div[data-field="pdffile"] input').files[0]);
                form.append('file_key', pdfchatModalEle
                    .querySelector('div[data-field="dataset-name"] input').value);
                form.append('data_key', pdfchatModalEle
                    .querySelector('div[data-field="data-key"] input').value);
                // and auth token to header
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                headers.append('X-Laisky-Api-Base', sconfig.api_base);

                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'POST',
                        headers,
                        body: form
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    showalert('success', 'upload dataset success, please wait few minutes to process');
                } catch (err) {
                    showalert('danger', `upload dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }
            });
    }

    // bind delete datasets buttion
    const bindDatasetDeleteBtn = () => {
        const datasets = pdfchatModalEle
            .querySelectorAll('div[data-field="dataset"] .dataset-item .bi-trash');

        if (datasets == null || datasets.length === 0) {
            return;
        }

        datasets.forEach((ele) => {
            ele.addEventListener('click', async (evt) => {
                evt.stopPropagation();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                // headers.append("X-PDFCHAT-PASSWORD", await KvGet(KvKeyCustomDatasetPassword));

                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'DELETE',
                        headers,
                        body: JSON.stringify({
                            datasets: [evtTarget(evt).closest('.dataset-item').getAttribute('data-filename')]
                        })
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }
                    await resp.json();
                } catch (err) {
                    showalert('danger', `delete dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                // remove dataset item
                evtTarget(evt).closest('.dataset-item').remove();
            });
        });
    }

    // bind list datasets
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] button[data-fn="refresh"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-PDFCHAT-PASSWORD', await KvGet(KvKeyCustomDatasetPassword));

                let body;
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/files', {
                        method: 'GET',
                        cache: 'no-cache',
                        headers
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch dataset failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                const datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let datasetsHTML = '';

                // add processing files
                // show processing files in grey and progress bar
                body.datasets.forEach((dataset) => {
                    switch (dataset.status) {
                    case 'done':
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
                                </div>`;
                        break;
                    case 'processing':
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
                                </div>`;
                        break;
                    }
                });

                datasetListEle.innerHTML = datasetsHTML;

                // selected binded datasets
                body.selected.forEach((dataset) => {
                    datasetListEle
                        .querySelector(`div[data-filename="${dataset}"] input[type="checkbox"]`)
                        .checked = true;
                })

                bindDatasetDeleteBtn();
            });
    }

    // bind list chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="list-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evtTarget(evt).closest('.dropdown')).hide();

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-PDFCHAT-PASSWORD', await KvGet(KvKeyCustomDatasetPassword));

                let body;
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/ctx/list', {
                        method: 'GET',
                        cache: 'no-cache',
                        headers
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    body = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                const datasetListEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"]');
                let chatbotsHTML = '';

                body.chatbots.forEach((chatbot) => {
                    let selectedHTML = '';
                    if (chatbot == body.current) {
                        selectedHTML = 'checked';
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
                        ele.addEventListener('change', async (evt) => {
                            evt.stopPropagation();

                            if (!evtTarget(evt).checked) {
                                // at least one chatbot should be selected
                                evtTarget(evt).checked = true;
                                return;
                            } else {
                                // uncheck other chatbot
                                datasetListEle
                                    .querySelectorAll('div[data-field="dataset"] .chatbot-item input[type="checkbox"]')
                                    .forEach((ele) => {
                                        if (ele != evtTarget(evt)) {
                                            ele.checked = false;
                                        }
                                    });
                            }

                            const headers = new Headers();
                            headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                            headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                            headers.append('X-Laisky-Api-Base', sconfig.api_base);

                            try {
                                ShowSpinner();
                                const chatbotName = evtTarget(evt).closest('.chatbot-item').getAttribute('data-name');
                                const resp = await fetch('/ramjet/gptchat/ctx/active', {
                                    method: 'POST',
                                    headers,
                                    body: JSON.stringify({
                                        data_key: await KvGet(KvKeyCustomDatasetPassword),
                                        chatbotName
                                    })
                                });

                                if (!resp.ok || resp.status !== 200) {
                                    throw new Error(`${resp.status} ${await resp.text()}`);
                                }

                                // const body = await resp.json();
                                showalert('success', `active chatbot success, you can chat with ${chatbotName} now`);
                            } catch (err) {
                                showalert('danger', `active chatbot failed, ${err.message}`);
                                throw err;
                            } finally {
                                HideSpinner();
                            }
                        });
                    });
            });
    }

    // bind share chatbots
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="share-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evtTarget(evt).closest('.dropdown')).hide();

                const checkedChatbotEle = pdfchatModalEle
                    .querySelector('div[data-field="dataset"] .chatbot-item input[type="checkbox"]:checked');
                if (!checkedChatbotEle) {
                    showalert('danger', 'please click [Chatbot List] first');
                    return;
                }

                const chatbotName = checkedChatbotEle.closest('.chatbot-item').getAttribute('data-name');

                const sconfig = await getChatSessionConfig();
                const headers = new Headers();
                headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                headers.append('Cache-Control', 'no-cache');
                headers.append('X-Laisky-Api-Base', sconfig.api_base);

                let respBody
                try {
                    ShowSpinner();
                    const resp = await fetch('/ramjet/gptchat/ctx/share', {
                        method: 'POST',
                        headers,
                        body: JSON.stringify({
                            chatbotName,
                            data_key: await KvGet(KvKeyCustomDatasetPassword)
                        })
                    });

                    if (!resp.ok || resp.status !== 200) {
                        throw new Error(`${resp.status} ${await resp.text()}`);
                    }

                    respBody = await resp.json();
                } catch (err) {
                    showalert('danger', `fetch chatbot list failed, ${err.message}`);
                    throw err;
                } finally {
                    HideSpinner();
                }

                // open new tab page
                const sharedChatbotUrl = `${location.origin}/?chatmodel=qa-shared&uid=${respBody.uid}&chatbotName=${respBody.chatbotName}`;
                showalert('info', `open ${sharedChatbotUrl}`);
                open(sharedChatbotUrl, '_blank');
            });
    }

    // build custom chatbot
    {
        pdfchatModalEle
            .querySelector('div[data-field="buttons"] a[data-fn="build-bot"]')
            .addEventListener('click', async (evt) => {
                evt.stopPropagation();
                new bootstrap.Dropdown(evtTarget(evt).closest('.dropdown')).hide();

                const selectedDatasets = [];
                pdfchatModalEle
                    .querySelectorAll('div[data-field="dataset"] .dataset-item input[type="checkbox"]')
                    .forEach((ele) => {
                        if (ele.checked) {
                            selectedDatasets.push(
                                ele.closest('.dataset-item').getAttribute('data-filename'));
                        }
                    });

                if (selectedDatasets.length === 0) {
                    showalert('warning', 'please select at least one dataset, click [List Dataset] button to fetch dataset list');
                    return;
                }

                // ask chatbot's name
                SingleInputModal('build bot', 'chatbot name', async (botname) => {
                    // botname should be 1-32 ascii characters
                    if (!botname.match(/^[a-zA-Z0-9_-]{1,32}$/)) {
                        showalert('warning', 'chatbot name should be 1-32 ascii characters');
                        return;
                    }

                    const sconfig = await getChatSessionConfig();
                    const headers = new Headers();
                    headers.append('Content-Type', 'application/json');
                    headers.append('Authorization', `Bearer ${sconfig.api_token}`);
                    headers.append('X-Laisky-User-Id', await getSHA1(sconfig.api_token));
                    headers.append('X-Laisky-Api-Base', sconfig.api_base);

                    try { // build chatbot
                        ShowSpinner()
                        const resp = await fetch('/ramjet/gptchat/ctx/build', {
                            method: 'POST',
                            headers,
                            body: JSON.stringify({
                                chatbotName: botname,
                                datasets: selectedDatasets,
                                data_key: pdfchatModalEle
                                    .querySelector('div[data-field="data-key"] input').value
                            })
                        });

                        if (!resp.ok || resp.status !== 200) {
                            throw new Error(`${resp.status} ${await resp.text()}`);
                        }

                        showalert('success', 'build dataset success, you can chat now');
                    } catch (err) {
                        showalert('danger', `build dataset failed, ${err.message}`);
                        throw err;
                    } finally {
                        HideSpinner();
                    }
                });
            });
    }
}
