'use strict';

/**
 * load js modules by urls
 *
 * @param {*} moduleUrls array of module urls
 * @param {*} moduleType script type, default is 'text/javascript'
 */
export const LoadJsModules = async (moduleUrls, moduleType) => {
    moduleType = moduleType || 'text/javascript';
    const promises = moduleUrls.map((moduleUrl) => {
        return new Promise((resolve, reject) => {
            const script = document.createElement('script');
            script.src = moduleUrl;
            script.type = moduleType;
            script.async = false;
            script.onload = resolve;
            script.onerror = reject;
            document.head.appendChild(script);
        });
    });

    await Promise.all(promises);
};

/**
 * async wait for seconds
 *
 * @param {*} seconds
 * @returns
 */
export const Sleep = async (seconds) => {
    return new Promise(resolve => setTimeout(resolve, seconds * 1000));
};

export const ActiveElementsByID = (elements, id) => {
    for (let i = 0; i < elements.length; i++) {
        const item = elements[i];
        if (item.id === id) {
            item.classList.add('active');
        } else {
            item.classList.remove('active');
        }
    }
};

export const ActiveElementsByData = (elements, dataKey, dataVal) => {
    for (let i = 0; i < elements.length; i++) {
        const item = elements[i];
        if (item.dataset[dataKey] === dataVal) {
            item.classList.add('active');
        } else {
            item.classList.remove('active');
        }
    }
};

export const DateStr = () => {
    const now = new Date();

    const year = now.getUTCFullYear();
    let month = now.getUTCMonth() + 1; // Months are 0-based, so we add 1
    let day = now.getUTCDate();
    let hours = now.getUTCHours();
    let minutes = now.getUTCMinutes();
    let seconds = now.getUTCSeconds();

    // Pad the month, day, hours, minutes and seconds with leading zeros, if required
    month = (month < 10 ? '0' : '') + month;
    day = (day < 10 ? '0' : '') + day;
    hours = (hours < 10 ? '0' : '') + hours;
    minutes = (minutes < 10 ? '0' : '') + minutes;
    seconds = (seconds < 10 ? '0' : '') + seconds;

    // Compose the date string
    return `${year}${month}${day}${hours}${minutes}${seconds}`;
};

// {key: [callback1, callback2]}
const kvListeners = {};
let kv;

function initKv() {
    if (!kv) {
        kv = new window.PouchDB('mydatabase');
    }
}

export const KvOp = Object.freeze({
    SET: 1,
    DEL: 2
});

// callback: function(keyPrefix, op, oldVal, newVal)
export const KvAddListener = (keyPrefix, callback) => {
    initKv();
    if (!kvListeners[keyPrefix]) {
        kvListeners[keyPrefix] = [];
    }

    if (kvListeners[keyPrefix].indexOf(callback) === -1) {
        kvListeners[keyPrefix].push(callback);
    }
};
// set data into indexeddb
export const KvSet = async (key, val) => {
    initKv();
    console.debug(`KvSet: ${key}`);
    const marshaledVal = JSON.stringify(val);
    let oldVal = null;
    try {
        await kv.put({
            _id: key,
            val: marshaledVal
        });
    } catch (error) {
        if (error.status === 409) {
            // Fetch the current document
            const doc = await kv.get(key);
            if (doc && doc.val) {
                oldVal = JSON.parse(doc.val);
            }

            // Save the new document with the _rev of the current document
            await kv.put({
                _id: key,
                _rev: doc._rev,
                val: marshaledVal
            });
        } else {
            console.error(`KvSet for key=${key}, val=${val} got error ${error.status}`);
            throw error;
        }
    }

    // notify listeners
    Object.keys(kvListeners).forEach((keyPrefix) => {
        if (key.startsWith(keyPrefix)) {
            for (let i = 0; i < kvListeners[keyPrefix].length; i++) {
                const callback = kvListeners[keyPrefix][i];
                callback(key, KvOp.SET, oldVal, val);
            }
        }
    });
};
/** get data from indexeddb
 *
 * @param {*} key
 * @returns null if not found
 */
export const KvGet = async (key) => {
    initKv();
    console.debug(`KvGet: ${key}`);
    try {
        const doc = await kv.get(key);
        if (!doc || !doc.val) {
            return null;
        }

        return JSON.parse(doc.val);
    } catch (error) {
        if (error.status === 404) {
            // Ignore not found error
            return null;
        }

        throw error;
    }
};
/** check if key exists in indexeddb
 *
 * @param {*} key
 * @returns true if exists, false otherwise
 */
export const KvExists = async (key) => {
    initKv();
    console.debug(`KvExists: ${key}`);
    try {
        await kv.get(key);
        return true;
    } catch (error) {
        if (error.status === 404) {
            // Ignore not found error
            return false;
        }

        throw error;
    }
};
/** rename key in indexeddb
 *
 * @param {*} oldKey
 * @param {*} newKey
 */
export const KvRename = async (oldKey, newKey) => {
    initKv();
    console.debug(`KvRename: ${oldKey} -> ${newKey}`);
    const oldVal = await KvGet(oldKey);
    if (!oldVal) {
        return
    }

    await KvSet(newKey, oldVal);
    await KvDel(oldKey);
};
// delete data from indexeddb
export const KvDel = async (key) => {
    initKv();
    console.debug(`KvDel: ${key}`);
    let oldVal = null;
    try {
        const doc = await kv.get(key);
        oldVal = JSON.parse(doc.val);
        await kv.remove(doc);
    } catch (error) {
        // ignore not found error
        if (error.status !== 404) {
            throw error;
        }
    }

    // notify listeners
    Object.keys(kvListeners).forEach((keyPrefix) => {
        if (key.startsWith(keyPrefix)) {
            for (let i = 0; i < kvListeners[keyPrefix].length; i++) {
                const callback = kvListeners[keyPrefix][i];
                callback(key, KvOp.DEL, oldVal, null);
            }
        }
    });
};
// list all keys from indexeddb
export const KvList = async () => {
    initKv();
    console.debug('KvList');
    const docs = await kv.allDocs({ include_docs: true });
    const keys = [];
    for (let i = 0; i < docs.rows.length; i++) {
        keys.push(docs.rows[i].doc._id);
    }
    return keys;
};
// clear all data from indexeddb
export const KvClear = async () => {
    initKv();
    console.debug('KvClear');

    // notify listeners
    (await KvList()).forEach((key) => {
        Object.keys(kvListeners).forEach((keyPrefix) => {
            if (key.startsWith(keyPrefix)) {
                for (let i = 0; i < kvListeners[keyPrefix].length; i++) {
                    const callback = kvListeners[keyPrefix][i];
                    callback(key, KvOp.DEL, null, null);
                }
            }
        });
    });

    await kv.destroy();
    kv = new window.PouchDB('mydatabase');
};

export const SetLocalStorage = (key, val) => {
    localStorage.setItem(key, JSON.stringify(val));
};
export const GetLocalStorage = (key) => {
    const v = localStorage.getItem(key);
    if (v) {
        return JSON.parse(v);
    } else {
        return v;
    }
};

export const Markdown2HTML = (markdown) => {
    const markdownConverter = new window.showdown.Converter();
    return markdownConverter.makeHtml(markdown);
};

export const ScrollDown = (element) => {
    element.scrollTo({
        top: element.scrollHeight,
        left: 0,
        behavior: 'smooth' // 可加入平滑过渡效果
    });
};

export const TrimSpace = (str) => {
    return str.replace(/^[\s\n]+|[\s\n]+$/g, '');
};

export const RenderStr2HTML = (str) => {
    return str.replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/\n/g, '<br/>')
        .replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
};

export const getSHA1 = async (str) => {
    // http do not support crypto
    if (!crypto || !crypto.subtle) { // http do not support crypto
        return window.sha1(str);
    }

    const encoder = new TextEncoder();
    const data = encoder.encode(str);
    const hash = await crypto.subtle.digest('SHA-1', data);
    return Array.from(new Uint8Array(hash))
        .map(b => b.toString(16).padStart(2, '0'))
        .join('');
};

export const ready = (fn) => {
    if (document.readyState === 'complete') {
        fn();
    } else {
        document.addEventListener('DOMContentLoaded', fn);
    }
};

export const escapeHtml = (str) => {
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    };

    return str.replace(/[&<>"']/g, function (m) { return map[m] });
};

/**
 * enable bootstrap tooltips for all elements with data-bs-toggle="tooltip"
 */
export const EnableTooltipsEverywhere = () => {
    const eles = document.querySelectorAll('[data-bs-toggle="tooltip"]') || [];
    eles.forEach((ele) => {
        if (ele.dataset.bsToggle === 'true') {
            return;
        }

        ele.dataset.bsToggle = 'true';
        return new window.bootstrap.Tooltip(ele);
    });
};

/**
 * disable bootstrap tooltips for all elements with class="tooltip.bs-tooltip-auto.fade.show"
 */
export const DisableTooltipsEverywhere = () => {
    const eles = document.querySelectorAll('.tooltip.bs-tooltip-auto.fade.show') || [];
    eles.forEach((ele) => {
        ele.remove();
    });
}

// convert blob to hex string
export const blob2Hex = async (blob) => {
    const arrayBuffer = await blob.arrayBuffer();
    const uint8Array = new Uint8Array(arrayBuffer);
    const hexString = Array.from(uint8Array)
        .map(byte => byte.toString(16).padStart(2, '0'))
        .join('');

    return hexString;
};

export const hex2Bytes = (hexString) => {
    const bytePairs = hexString.match(/.{1,2}/g);
    const bytes = bytePairs.map(bytePair => parseInt(bytePair, 16));
    const uint8Array = new Uint8Array(bytes);

    return uint8Array;
};

// convert hex string to blob
export const hex2Blob = (hexString) => {
    const arrayBuffer = hexString.match(/.{1,2}/g)
        .map(byte => parseInt(byte, 16));
    const uint8Array = new Uint8Array(arrayBuffer);
    const blob = new Blob([uint8Array]);

    return blob;
};

// convert string to compressed hex string
export const gzip = async (stringVal) => {
    const blob = new Blob([stringVal], { type: 'text/plain' });
    const s = new CompressionStream('gzip');
    const ps = blob.stream().pipeThrough(s);
    const compressedBlob = await new Response(ps).blob();
    return await blob2Hex(compressedBlob);
};

// convert compressed hex string to decompressed string
export const ungzip = async (hexStringVal) => {
    const blob = hex2Blob(hexStringVal);
    const s = new DecompressionStream('gzip');
    const ps = blob.stream().pipeThrough(s);
    const decompressedBlob = await new Response(ps).blob();
    return await decompressedBlob.text();
};

// sanitize html
export const sanitizeHTML = (str) => {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
};

export const evtTarget = (evt) => {
    return evt.currentTarget || evt.target;
};

/**
 * Generates a random string of the specified length.
 * @param {number} length - The length of the string to generate.
 * @returns {string} - The generated random string.
 */
export const RandomString = (length) => {
    const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length));
    }

    return result;
};
