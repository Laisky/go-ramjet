"use strict";

(function () {
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

    window.sha1 = async (str) => {
        // http do not support crypto
        if (!crypto.subtle) {  // FIXME
            return str;
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
})();
