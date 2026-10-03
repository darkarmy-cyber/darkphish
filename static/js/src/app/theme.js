(function () {
    "use strict";

    var storageKey = "darkphish-theme";

    function isDark() {
        return document.documentElement.getAttribute("data-theme") === "dark";
    }

    function setTheme(dark) {
        if (dark) document.documentElement.setAttribute("data-theme", "dark");
        else document.documentElement.removeAttribute("data-theme");
        try {
            localStorage.setItem(storageKey, dark ? "dark" : "light");
        } catch (e) {}
        var button = document.getElementById("themeToggle");
        if (!button) return;
        button.setAttribute("aria-pressed", dark ? "true" : "false");
        button.setAttribute("title", dark ? "Use light theme" : "Use dark theme");
        button.setAttribute("aria-label", dark ? "Use light theme" : "Use dark theme");
        var sr = button.querySelector(".sr-only");
        if (sr) sr.textContent = dark ? "Use light theme" : "Use dark theme";
    }

    document.addEventListener("DOMContentLoaded", function () {
        setTheme(isDark());
        var button = document.getElementById("themeToggle");
        if (!button) return;
        button.addEventListener("click", function () {
            setTheme(!isDark());
        });
    });
})();