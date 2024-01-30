'use strict';

(function () {
    document.addEventListener('DOMContentLoaded', async () => {
        let kv; // indexeddb
        (() => {
            // setup pouch db
            kv = new PouchDB('mydatabase');
        })();

        window.ActiveElementsByID = (elements, id) => {
            for (let i = 0; i < elements.length; i++) {
                const item = elements[i];
                if (item.id === id) {
                    item.classList.add('active');
                } else {
                    item.classList.remove('active');
                }
            }
        };

        window.ActiveElementsByData = (elements, dataKey, dataVal) => {
            for (let i = 0; i < elements.length; i++) {
                const item = elements[i];
                if (item.dataset[dataKey] == dataVal) {
                    item.classList.add('active');
                } else {
                    item.classList.remove('active');
                }
            }
        };

        window.DateStr = () => {
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

        window.KvOp = Object.freeze({
            SET: 1,
            DEL: 2
        });

        // callback: function(keyPrefix, op, oldVal, newVal)
        window.KvAddListener = (keyPrefix, callback) => {
            if (!kvListeners[keyPrefix]) {
                kvListeners[keyPrefix] = [];
            }

            if (kvListeners[keyPrefix].indexOf(callback) === -1) {
                kvListeners[keyPrefix].push(callback);
            }
        };
        // set data into indexeddb
        window.KvSet = async (key, val) => {
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
                    oldVal = JSON.parse(doc.val);

                    // Save the new document with the _rev of the current document
                    await kv.put({
                        _id: key,
                        _rev: doc._rev,
                        val: marshaledVal
                    });
                } else {
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
        window.KvGet = async (key) => {
            console.debug(`KvGet: ${key}`);
            try {
                const doc = await kv.get(key);
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
        window.KvExists = async (key) => {
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
        window.KvRename = async (oldKey, newKey) => {
            console.debug(`KvRename: ${oldKey} -> ${newKey}`);
            const oldVal = await KvGet(oldKey);
            if (!oldVal) {
                return
            }

            await KvSet(newKey, oldVal);
            await KvDel(oldKey);
        };
        // delete data from indexeddb
        window.KvDel = async (key) => {
            console.debug(`KvDel: ${key}`);
            let oldVal = null;
            try {
                const doc = await kv.get(key);
                oldVal = JSON.parse(doc.val);
                await kv.remove(doc);
            } catch (error) {
                // ignore not found error
                if (error.status != 404) {
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
        window.KvList = async () => {
            console.debug('KvList');
            const docs = await kv.allDocs({ include_docs: true });
            const keys = [];
            for (let i = 0; i < docs.rows.length; i++) {
                keys.push(docs.rows[i].doc._id);
            }
            return keys;
        };
        // clear all data from indexeddb
        window.KvClear = async () => {
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
            kv = new PouchDB('mydatabase');
        };

        window.SetLocalStorage = (key, val) => {
            localStorage.setItem(key, JSON.stringify(val));
        };
        window.GetLocalStorage = (key) => {
            const v = localStorage.getItem(key);
            if (v) {
                return JSON.parse(v);
            } else {
                return v;
            }
        };

        window.Markdown2HTML = (markdown) => {
            const markdownConverter = new window.showdown.Converter();
            return markdownConverter.makeHtml(markdown);
        };

        window.ScrollDown = (element) => {
            element.scrollTo({
                top: element.scrollHeight,
                left: 0,
                behavior: 'smooth' // 可加入平滑过渡效果
            });
        };

        window.TrimSpace = (str) => {
            return str.replace(/^[\s\n]+|[\s\n]+$/g, '');
        };

        window.RenderStr2HTML = (str) => {
            return str.replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/\n/g, '<br/>')
                .replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
        };

        window.getSHA1 = async (str) => {
            // http do not support crypto
            if (!crypto || !crypto.subtle) { // http do not support crypto
                return sha1(str);
            }

            const encoder = new TextEncoder();
            const data = encoder.encode(str);
            const hash = await crypto.subtle.digest('SHA-1', data);
            return Array.from(new Uint8Array(hash))
                .map(b => b.toString(16).padStart(2, '0'))
                .join('');
        };

        window.ready = (fn) => {
            if (document.readyState == 'complete') {
                fn();
            } else {
                document.addEventListener('DOMContentLoaded', fn);
            }
        };

        window.escapeHtml = (str) => {
            const map = {
                '&': '&amp;',
                '<': '&lt;',
                '>': '&gt;',
                '"': '&quot;',
                "'": '&#039;'
            };

            return str.replace(/[&<>"']/g, function (m) { return map[m] });
        };

        window.EnableTooltipsEverywhere = () => {
            const tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'));
            tooltipTriggerList.map(function (tooltipTriggerEl) {
                return new bootstrap.Tooltip(tooltipTriggerEl);
            });
        };

        // convert blob to hex string
        window.blob2Hex = async (blob) => {
            const arrayBuffer = await blob.arrayBuffer();
            const uint8Array = new Uint8Array(arrayBuffer);
            const hexString = Array.from(uint8Array)
                .map(byte => byte.toString(16).padStart(2, '0'))
                .join('');

            return hexString;
        };

        window.hex2Bytes = (hexString) => {
            const bytePairs = hexString.match(/.{1,2}/g);
            const bytes = bytePairs.map(bytePair => parseInt(bytePair, 16));
            const uint8Array = new Uint8Array(bytes);

            return uint8Array;
        };

        // convert hex string to blob
        window.hex2Blob = (hexString) => {
            const arrayBuffer = hexString.match(/.{1,2}/g)
                .map(byte => parseInt(byte, 16));
            const uint8Array = new Uint8Array(arrayBuffer);
            const blob = new Blob([uint8Array]);

            return blob;
        };

        // convert string to compressed hex string
        window.gzip = async (stringVal) => {
            const blob = new Blob([stringVal], { type: 'text/plain' });
            const s = new CompressionStream('gzip');
            const ps = blob.stream().pipeThrough(s);
            const compressedBlob = await new Response(ps).blob();
            return await blob2Hex(compressedBlob);
        };

        // convert compressed hex string to decompressed string
        window.ungzip = async (hexStringVal) => {
            const blob = hex2Blob(hexStringVal);
            const s = new DecompressionStream('gzip');
            const ps = blob.stream().pipeThrough(s);
            const decompressedBlob = await new Response(ps).blob();
            return await decompressedBlob.text();
        };

        // sanitize html
        window.sanitizeHTML = (str) => {
            return str
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#039;');
        };

        window.evtTarget = (evt) => {
            return evt.currentTarget || evt.target;
        };

        /**
         * run app entrypoint
         */
        if (window.AppEntrypoint) {
            await window.AppEntrypoint();
        }
    });
})();
