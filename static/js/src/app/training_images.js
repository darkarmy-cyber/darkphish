// Only inert training imports use this helper. Safe public raster images are fetched automatically after import.
var trainingImagePreview = (function () {
    var generation = 0, pending = false, attempted = Object.create(null)
    function reset() {
        generation++
        pending = false
        $("#modalSubmit").prop("disabled", false)
        attempted = Object.create(null)
        $("#trainingImageStatus").text("Safe public images will be loaded automatically after import.")
    }
    function load(automatic) {
        if (!trainingStatic || pending) return
        var editor = CKEDITOR.instances.html_editor, current = generation
        if (!editor) return
        pending = true
        $("#modalSubmit").prop("disabled", true)
        function request() {
            if (!trainingStatic || current !== generation) return
            if (!editor.document) { finish(); return }
            var images = Array.prototype.slice.call(editor.document.$.querySelectorAll("img[data-training-image-url]"))
            var urls = []
            images.forEach(function (img) {
                var url = img.getAttribute("data-training-image-url")
                if (!/^https:\/\/[^\s{}\\]+$/i.test(url || "") || /^data:image\/png;base64,/.test(img.getAttribute("src") || "")) return
                if (urls.indexOf(url) < 0) urls.push(url)
            })
            var all = urls
            urls = all.filter(function (url) { return !attempted[url] })
            if (!automatic && !urls.length && all.length) { attempted = Object.create(null); urls = all }
            var remaining = Math.max(0, urls.length - 12)
            urls = urls.slice(0, 12)
            if (!urls.length) { $("#trainingImageStatus").text("No additional supported images to load. Inline SVG and dynamic images are not supported."); finish(); return }
            var continueBatch = false
            pending = true
            $("#trainingImageStatus").text("Verifying images…")
            api.preview_email_images({urls: urls}).done(function (results) {
                if (current !== generation || !trainingStatic) return
                var loaded = 0, overBudget = 0, embedded = Object.create(null)
                if (!Array.isArray(results)) { $("#trainingImageStatus").text("Unexpected image response. No images were changed."); return }
                if (editor.mode !== "wysiwyg" || !editor.document) { $("#trainingImageStatus").text("Editor mode changed. Try loading images again."); return }
                images = Array.prototype.slice.call(editor.document.$.querySelectorAll("img[data-training-image-url]"))
                // Leave 1 MiB for server normalization. Count UTF-8 HTML and both
                // src/saved-src copies at every occurrence before changing DOM.
                var bytes = new TextEncoder().encode(editor.getData()).length
                var maxBytes = 7 * 1024 * 1024
                urls.forEach(function (url) { attempted[url] = true })
                results.forEach(function (result) {
                    if (!result || embedded[result.url] || urls.indexOf(result.url) < 0 || !/^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(result.data || "")) return
                    var matching = images.filter(function (img) { return img.getAttribute("data-training-image-url") === result.url })
                    if (!matching.length) return
                    embedded[result.url] = true
                    var extra = matching.length * (result.data.length * 2 + 128)
                    if (bytes + extra > maxBytes) { overBudget++; return }
                    bytes += extra
                    loaded++
                    matching.forEach(function (img) {
                        img.setAttribute("src", result.data)
                        img.setAttribute("data-cke-saved-src", result.data)
                    })
                })
                editor.fire("change")
                continueBatch = remaining > 0
                $("#trainingImageStatus").text(loaded + " of " + urls.length + " public raster images embedded." + (overBudget ? " Page size budget reached; some images were skipped." : "") + (remaining ? " Additional images remain after the automatic batch." : ""))
            }).fail(function () {
                if (current === generation) $("#trainingImageStatus").text("Images could not be verified. Check your session and try again.")
            }).always(function () {
                if (current === generation) {
                    finish()
                    if (continueBatch) load(true)
                }
            })
        }
        if (editor.mode !== "wysiwyg") editor.setMode("wysiwyg", request)
        else request()
        function finish() {
            pending = false
            $("#modalSubmit").prop("disabled", false)
        }
    }
    return {reset: reset, load: load, isPending: function () { return pending }}
})()
