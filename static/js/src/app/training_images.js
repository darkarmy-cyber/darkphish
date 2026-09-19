// Only inert training imports use this helper. No image fetch until consent.
var trainingImagePreview = (function () {
    var generation = 0, pending = false, attempted = Object.create(null)
    function reset() {
        generation++
        pending = false
        attempted = Object.create(null)
        $("#trainingImageStatus").text("Images have not been requested.")
    }
    function load() {
        if (!trainingStatic || pending) return
        var editor = CKEDITOR.instances.html_editor, current = generation
        if (!editor) return
        function request() {
            if (!trainingStatic || current !== generation || !editor.document) return
            var images = Array.prototype.slice.call(editor.document.$.querySelectorAll("img[data-training-image-url]"))
            var urls = []
            images.forEach(function (img) {
                var url = img.getAttribute("data-training-image-url")
                if (!/^https:\/\/[^\s{}\\]+$/i.test(url || "") || /^data:image\/png;base64,/.test(img.getAttribute("src") || "")) return
                if (urls.indexOf(url) < 0) urls.push(url)
            })
            var all = urls
            urls = all.filter(function (url) { return !attempted[url] })
            if (!urls.length && all.length) { attempted = Object.create(null); urls = all }
            var remaining = Math.max(0, urls.length - 12)
            urls = urls.slice(0, 12)
            if (!urls.length) { $("#trainingImageStatus").text("No additional supported images to load. Inline SVG and dynamic images are not supported."); return }
            pending = true
            $("#loadTrainingImages").prop("disabled", true)
            $("#trainingImageStatus").text("Verifying images…")
            api.preview_email_images({urls: urls}).done(function (results) {
                if (current !== generation || !trainingStatic) return
                var loaded = 0
                if (!Array.isArray(results)) { $("#trainingImageStatus").text("Unexpected image response. No images were changed."); return }
                urls.forEach(function (url) { attempted[url] = true })
                results.forEach(function (result) {
                    if (!result || urls.indexOf(result.url) < 0 || !/^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(result.data || "")) return
                    loaded++
                    images.forEach(function (img) {
                        if (img.getAttribute("data-training-image-url") !== result.url) return
                        img.setAttribute("src", result.data)
                        img.setAttribute("data-cke-saved-src", result.data)
                    })
                })
                editor.fire("change")
                $("#trainingImageStatus").text(loaded + " of " + urls.length + " images embedded. Unavailable, private, oversized or unsupported images remain blocked." + (remaining ? " Click again for remaining images." : ""))
            }).fail(function () {
                if (current === generation) $("#trainingImageStatus").text("Images could not be verified. Check your session and try again.")
            }).always(function () {
                if (current === generation) { pending = false; $("#loadTrainingImages").prop("disabled", !trainingStatic) }
            })
        }
        if (editor.mode !== "wysiwyg") editor.setMode("wysiwyg", request)
        else request()
    }
    return {reset: reset, load: load}
})()
