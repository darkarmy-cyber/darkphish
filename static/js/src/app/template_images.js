// Remote images stay blocked by the admin CSP. Safe public raster previews are
// localized automatically for the editor while the email's original URLs stay saved.
var templateImagePreview = (function () {
    var cache = Object.create(null)
    var attempted = Object.create(null)
    var generation = 0

    function images(editor) {
        return editor && editor.mode === 'wysiwyg' && editor.document ?
            Array.prototype.slice.call(editor.document.$.querySelectorAll('img')) : []
    }

    function source(img) {
        return img.getAttribute('data-cke-saved-src') || img.getAttribute('src') || ''
    }

    function eligible(src) {
        if (!/^https:\/\//i.test(src) || /[{}\\\s]/.test(src) || src.length > 4096) return false
        try {
            var url = new URL(src)
            return url.protocol === 'https:' && !url.username && !url.password && (!url.port || url.port === '443')
        } catch (e) { return false }
    }

    function apply(editor) {
        images(editor).forEach(function (img) {
            var original = source(img)
            if (!cache[original] || img.hasAttribute('srcset')) return
            // CKEditor's own output processor restores this attribute to src
            // when switching to Source, copying, or saving the template.
            img.setAttribute('data-cke-saved-src', original)
            img.setAttribute('src', cache[original])
        })
    }

    function reset(editor) {
        generation++
        cache = Object.create(null)
        attempted = Object.create(null)
        $('#templateImageStatus').text('Safe public external images are loaded automatically for preview.')
        if (editor && !editor.darkphishImagePreviewReady) {
            editor.darkphishImagePreviewReady = true
            editor.on('dataReady', function () { apply(editor) })
            editor.on('mode', function () { apply(editor) })
        }
    }

    function load() {
        var editor = CKEDITOR.instances.html_editor
        if (!editor) return
        var current = generation
        function request() {
            if (current !== generation) return
            var pending = [], unsupported = 0
            images(editor).forEach(function (img) {
                var src = source(img)
                if (eligible(src) && !img.hasAttribute('srcset')) {
                    if (pending.indexOf(src) === -1 && !cache[src]) pending.push(src)
                } else if (!/^data:/i.test(src) && !/^\//.test(src)) unsupported++
            })
            // Try every new URL before retrying failed previews. A failed first
            // batch must not prevent later images from ever being requested.
            var urls = pending.filter(function (src) { return !attempted[src] })
            if (!urls.length && pending.length) {
                attempted = Object.create(null)
                urls = pending
            }
            var omitted = Math.max(0, urls.length - 12)
            urls = urls.slice(0, 12)
            if (!urls.length) {
                apply(editor)
                $('#templateImageStatus').text(unsupported ?
                    'Some images cannot be previewed. Use a public HTTPS PNG, JPEG or GIF image URL without srcset; CID images need their original attachments.' :
                    'No additional external images to load.')
                return
            }
            $('#templateImageStatus').text('Loading image previews…')
            api.preview_email_images({urls: urls})
                .done(function (results) {
                    if (current !== generation) return
                    urls.forEach(function (src) { attempted[src] = true })
                    var loaded = 0
                    results.forEach(function (result) {
                        if (urls.indexOf(result.url) !== -1 && /^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(result.data || '')) {
                            cache[result.url] = result.data
                            loaded++
                        }
                    })
                    apply(editor)
                    var failed = pending.filter(function (src) { return attempted[src] && !cache[src] }).length
                    var message = loaded + ' of ' + urls.length + ' external image previews loaded automatically. Original email URLs are unchanged.'
                    if (failed || unsupported) message += ' Unavailable, private, unsupported or oversized images remain blocked.'
                    if (omitted) message += ' Additional images remain after the automatic batch.'
                    $('#templateImageStatus').text(message)
                })
                .fail(function () {
                    if (current === generation) $('#templateImageStatus').text('Image previews could not be loaded. Check your session or try again later.')
                })
                .always(function () {})
        }
        if (editor.mode !== 'wysiwyg') editor.setMode('wysiwyg', request)
        else request()
    }

    return {reset: reset, load: load}
})()
