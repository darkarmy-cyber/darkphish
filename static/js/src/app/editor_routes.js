(function () {
    "use strict";

    var routeState = {
        active: false,
        dirty: false,
        saving: false,
        allowDismiss: false,
        resource: "",
        mode: "",
        id: 0,
        listPath: ""
    };

    function currentRoute() {
        var match = window.location.pathname.match(/^\/(templates|landing_pages)\/(?:(new)|([0-9]+)\/(edit|copy))$/);
        if (!match) return null;
        return {
            resource: match[1],
            mode: match[2] ? "new" : match[4],
            id: match[3] ? parseInt(match[3], 10) : 0,
            listPath: match[1] === "templates" ? "/templates" : "/landing_pages"
        };
    }

    function collection() {
        return routeState.resource === "templates" ? window.templates : window.pages;
    }

    function markDirty() {
        if (routeState.active && !routeState.saving) routeState.dirty = true;
    }

    function clearDirty() {
        routeState.dirty = false;
    }

    function confirmLeave() {
        return !routeState.dirty || window.confirm("You have unsaved changes. Leave this editor without saving?");
    }

    function redirectToList() {
        window.location.assign(routeState.listPath);
    }

    function bindDirtyTracking() {
        var modal = $("#modal");
        modal.off(".editorRoute");
        modal.on("input.editorRoute change.editorRoute", "input, textarea, select", markDirty);
        modal.on("click.editorRoute", "#attachmentsTable .fa-trash-o", markDirty);
        modal.on("click.editorRoute", "#modalSubmit", function () {
            routeState.saving = true;
        });

        if (window.CKEDITOR && CKEDITOR.instances && CKEDITOR.instances.html_editor) {
            CKEDITOR.instances.html_editor.on("change", markDirty);
        }
        if (window.CKEDITOR) {
            CKEDITOR.on("instanceReady", function (event) {
                if (routeState.active && event.editor && event.editor.name === "html_editor") {
                    event.editor.on("change", markDirty);
                }
            });
        }
    }

    function installDismissGuard() {
        if (!routeState.active || window.__darkphishEditorDismissGuard) return;
        window.__darkphishEditorDismissGuard = true;
        var originalDismiss = window.dismiss;
        if (typeof originalDismiss !== "function") return;

        window.dismiss = function () {
            if (!routeState.active) {
                return originalDismiss.apply(this, arguments);
            }
            if (routeState.saving) {
                clearDirty();
                var saved = originalDismiss.apply(this, arguments);
                setTimeout(redirectToList, 0);
                return saved;
            }
            if (!routeState.allowDismiss && !confirmLeave()) return;
            routeState.allowDismiss = false;
            clearDirty();
            var result = originalDismiss.apply(this, arguments);
            setTimeout(redirectToList, 0);
            return result;
        };
    }

    function openRouteEditor() {
        var route = currentRoute();
        if (!route) return;
        routeState.active = true;
        routeState.resource = route.resource;
        routeState.mode = route.mode;
        routeState.id = route.id;
        routeState.listPath = route.listPath;

        installDismissGuard();

        function open() {
            if (route.mode === "new") {
                window.edit(-1);
            } else {
                var items = collection();
                if (!Array.isArray(items) || !items.length) return false;
                var idx = -1;
                $.each(items, function (i, item) {
                    if (String(item.id) === String(route.id)) idx = i;
                });
                if (idx < 0) return false;
                if (route.mode === "copy") window.copy(idx);
                else window.edit(idx);
            }
            $("#modal").modal({ backdrop: "static", keyboard: false, show: true });
            bindDirtyTracking();
            clearDirty();
            return true;
        }

        if (open()) return;
        var attempts = 0;
        var timer = window.setInterval(function () {
            attempts += 1;
            if (open() || attempts >= 100) {
                window.clearInterval(timer);
                if (attempts >= 100 && !$("#modal").hasClass("in")) {
                    errorFlash("Unable to open the requested editor.");
                    redirectToList();
                }
            }
        }, 50);
    }

    function interceptListActions(event) {
        if (routeState.active) {
            var cancel = $(event.target).closest("#modal [data-dismiss='modal'], #modal .close");
            if (cancel.length) {
                if (!confirmLeave()) {
                    event.preventDefault();
                    event.stopImmediatePropagation();
                    return;
                }
                routeState.allowDismiss = true;
            }
            return;
        }

        var path = window.location.pathname;
        if (path !== "/templates" && path !== "/landing_pages") return;
        var button = $(event.target).closest("button[onclick]");
        if (!button.length) return;
        var action = (button.attr("onclick") || "").match(/^\s*(edit|copy)\((\d+)\)\s*;?\s*$/);
        if (!action) return;

        var items = path === "/templates" ? window.templates : window.pages;
        var idx = parseInt(action[2], 10);
        if (!Array.isArray(items) || !items[idx] || !items[idx].id) return;

        event.preventDefault();
        event.stopImmediatePropagation();
        window.location.assign(path + "/" + items[idx].id + "/" + action[1]);
    }

    window.addEventListener("beforeunload", function (event) {
        if (!routeState.active || !routeState.dirty) return;
        event.preventDefault();
        event.returnValue = "";
    });

    document.addEventListener("click", interceptListActions, true);

    $(document).ajaxSuccess(function (_event, _xhr, settings) {
        if (!routeState.active) return;
        var url = settings && settings.url ? settings.url : "";
        if (/\/api\/import\//.test(url)) markDirty();
    });

    $(document).ajaxError(function (_event, _xhr, settings) {
        if (!routeState.active || !routeState.saving) return;
        var url = settings && settings.url ? settings.url : "";
        if ((routeState.resource === "templates" && /\/api\/templates/.test(url)) ||
            (routeState.resource === "landing_pages" && /\/api\/pages/.test(url))) {
            routeState.saving = false;
        }
    });

    $(document).ready(openRouteEditor);
})();