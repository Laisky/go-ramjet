"use strict";

(function () {
    let kv;  // indexeddb
    (() => {
        // setup pouch db
        kv = new PouchDB("mydatabase");
    })();

    window.ActiveElementsByID = (elements, id) => {
        for (let i = 0; i < elements.length; i++) {
            let item = elements[i];
            if (item.id == id) {
                item.classList.add("active");
            } else {
                item.classList.remove("active");
            }
        }
    };

    window.ActiveElementsByData = (elements, dataKey, dataVal) => {
        for (let i = 0; i < elements.length; i++) {
            let item = elements[i];
            if (item.dataset[dataKey] == dataVal) {
                item.classList.add("active");
            } else {
                item.classList.remove("active");
            }
        }
    };

    window.DateStr = () => {
        let now = new Date();

        let year = now.getUTCFullYear();
        let month = now.getUTCMonth() + 1; // Months are 0-based, so we add 1
        let day = now.getUTCDate();
        let hours = now.getUTCHours();
        let minutes = now.getUTCMinutes();
        let seconds = now.getUTCSeconds();

        // Pad the month, day, hours, minutes and seconds with leading zeros, if required
        month = (month < 10 ? "0" : "") + month;
        day = (day < 10 ? "0" : "") + day;
        hours = (hours < 10 ? "0" : "") + hours;
        minutes = (minutes < 10 ? "0" : "") + minutes;
        seconds = (seconds < 10 ? "0" : "") + seconds;

        // Compose the date string
        return `${year}${month}${day}${hours}${minutes}${seconds}`;
    };

    // set data into indexeddb
    window.KvSet = async (key, val) => {
        try {
            await kv.put({
                _id: key,
                val: JSON.stringify(val),
            });
        } catch (error) {
            if (error.status === 409) {
                // Fetch the current document
                let doc = await kv.get(key);

                // Save the new document with the _rev of the current document
                await kv.put({
                    _id: key,
                    _rev: doc._rev,
                    val: JSON.stringify(val),
                });
            } else {
                throw error;
            }
        }
    };
    // get data from indexeddb
    window.KvGet = async (key) => {
        try {
            let doc = await kv.get(key);
            return JSON.parse(doc.val);
        } catch (error) {
            if (error.status === 404) {
                // Ignore not found error
                return null;
            }
            throw error;
        }
    };
    // delete data from indexeddb
    window.KvDel = async (key) => {
        try {
            const doc = await kv.get(key);
            await kv.remove(doc);
        } catch (error) {
            if (error.status === 404) {
                // Ignore not found error
                return;
            }
            throw error;
        }
    };
    // list all keys from indexeddb
    window.KvList = async () => {
        let docs = await kv.allDocs({ include_docs: true });
        let keys = [];
        for (let i = 0; i < docs.rows.length; i++) {
            keys.push(docs.rows[i].doc._id);
        }
        return keys;
    };
    // clear all data from indexeddb
    window.KvClear = async () => {
        await kv.destroy();
        kv = new PouchDB("mydatabase");
    };


    window.SetLocalStorage = (key, val) => {
        localStorage.setItem(key, JSON.stringify(val));
    };
    window.GetLocalStorage = (key) => {
        let v = localStorage.getItem(key);
        if (v) {
            return JSON.parse(v);
        } else {
            return v;
        }
    };

    window.Markdown2HTML = (markdown) => {
        let markdownConverter = new window.showdown.Converter();
        return markdownConverter.makeHtml(markdown);
    }

    window.ScrollDown = (element) => {
        element.scrollTo({
            top: element.scrollHeight,
            left: 0,
            behavior: 'smooth' // 可加入平滑过渡效果
        });
    };

    window.TrimSpace = (str) => {
        return str.replace(/^[\s\n]+|[\s\n]+$/g, "");
    };

    window.RenderStr2HTML = (str) => {
        return str.replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/\n/g, '<br/>')
            .replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
    }

    window.getSHA1 = async (str) => {
        // http do not support crypto
        if (!crypto || !crypto.subtle) {  // http do not support crypto
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

        return str.replace(/[&<>"']/g, function (m) { return map[m]; });
    };

    window.EnableTooltipsEverywhere = () => {
        var tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'))
        var tooltipList = tooltipTriggerList.map(function (tooltipTriggerEl) {
            return new bootstrap.Tooltip(tooltipTriggerEl)
        })
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
        let blob = new Blob([stringVal], { type: "text/plain" });
        let s = new CompressionStream("gzip");
        let ps = blob.stream().pipeThrough(s);
        let compressedBlob = await new Response(ps).blob();
        return await blob2Hex(compressedBlob);
    };

    // convert compressed hex string to decompressed string
    window.ungzip = async (hexStringVal) => {
        let blob = hex2Blob(hexStringVal);
        let s = new DecompressionStream("gzip");
        let ps = blob.stream().pipeThrough(s);
        let decompressedBlob = await new Response(ps).blob();
        return await decompressedBlob.text();
    };

    // sanitize html
    window.sanitizeHTML = (str) => {
        return str
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;")
            .replace(/'/g, "&#039;");
    };
})();
