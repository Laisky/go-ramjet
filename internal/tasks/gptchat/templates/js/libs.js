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
})();
