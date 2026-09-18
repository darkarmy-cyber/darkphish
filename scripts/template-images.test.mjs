import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'

const png = 'data:image/png;base64,iVBORw0KGgo='
function page(path, sources) {
  const elements = new Map(), requests = []
  const $ = selector => {
    if (!elements.has(selector)) elements.set(selector, {
      text(value) { this.value = value; return this },
      prop(key, value) { this[key] = value; return this }
    })
    return elements.get(selector)
  }
  const nodes = sources.map(src => {
    const attrs = typeof src === 'string' ? {src} : {...src}
    return {attrs, getAttribute: key => attrs[key], setAttribute: (key, value) => { attrs[key] = value }, hasAttribute: key => key in attrs}
  })
  const events = {}
  const editor = {mode: 'wysiwyg', document: {$: {querySelectorAll: () => nodes}}, on: (name, fn) => { events[name] = fn }, setMode(mode, fn) { this.mode = mode; fn() }}
  const context = vm.createContext({$, URL, CKEDITOR: {instances: {html_editor: editor}}, api: {preview_email_images(body) {
    const callbacks = {}
    const request = {body, done(fn) { callbacks.done = fn; return this }, fail(fn) { callbacks.fail = fn; return this }, always(fn) { callbacks.always = fn; return this },
      resolve(results) { callbacks.done(results); callbacks.always() }, reject() { callbacks.fail(); callbacks.always() }}
    requests.push(request)
    return request
  }}})
  vm.runInContext(readFileSync(new URL(path, import.meta.url), 'utf8'), context)
  const helper = context.templateImagePreview
  helper.reset(editor)
  return {helper, editor, nodes, events, requests, $}
}

for (const path of ['../static/js/src/app/template_images.js', '../static/js/dist/app/template_images.min.js']) {
  test(`${path}: no fetch before consent, preview preserves original URL for CKEditor output`, () => {
    const url = 'https://images.example.test/logo.png?x=1&y=2'
    const p = page(path, [url, url])
    p.events.dataReady()
    assert.equal(p.requests.length, 0)
    p.helper.load()
    assert.deepEqual([...p.requests[0].body.urls], [url])
    assert.equal(p.$('#loadTemplateImages').disabled, true)
    p.requests[0].resolve([{url, data: png}])
    for (const {attrs} of p.nodes) {
      assert.equal(attrs.src, png)
      assert.equal(attrs['data-cke-saved-src'], url)
    }
    assert.equal(p.$('#loadTemplateImages').disabled, false)
    p.helper.load()
    assert.equal(p.requests.length, 1)
    // Source/wysiwyg round trip resets DOM src; cached preview needs no network.
    p.nodes[0].attrs.src = url
    p.events.dataReady()
    assert.equal(p.nodes[0].attrs.src, png)
    p.nodes[0].attrs.src = url
    p.events.mode()
    assert.equal(p.nodes[0].attrs.src, png)
  })

  test(`${path}: unsupported inputs are not requested; batches are bounded`, () => {
    const sources = ['http://example.test/a', 'cid:part1', '//example.test/a', 'https://u:p@example.test/a', 'https://example.test:8443/a', 'https://{{.URL}}', png,
      {src: 'https://example.test/a', srcset: 'https://example.test/b 2x'}]
    const p = page(path, sources)
    p.helper.load()
    assert.equal(p.requests.length, 0)
    assert.match(p.$('#templateImageStatus').value, /cannot be previewed/)
    const many = page(path, Array.from({length: 15}, (_, i) => `https://example.test/${i}.png`))
    many.editor.mode = 'source'
    many.helper.load()
    assert.equal(many.requests[0].body.urls.length, 12)
    many.requests[0].resolve(many.requests[0].body.urls.map(url => ({url, data: png})))
    many.helper.load()
    assert.equal(many.requests[1].body.urls.length, 3)
  })

  test(`${path}: late replies cannot alter another template; errors restore the button`, () => {
    const url = 'https://example.test/a.png'
    const p = page(path, [url])
    p.helper.load()
    p.helper.reset(p.editor)
    p.requests[0].resolve([{url, data: png}])
    assert.equal(p.nodes[0].attrs.src, url)
    p.helper.load()
    p.requests[1].reject()
    assert.equal(p.$('#loadTemplateImages').disabled, false)
    assert.match(p.$('#templateImageStatus').value, /could not/)
    p.helper.load()
    p.requests[2].resolve([{url, data: 'data:image/svg+xml;base64,AAAA'}, {url: 'https://other.test/a', data: png}])
    assert.equal(p.nodes[0].attrs.src, url)
    assert.match(p.$('#templateImageStatus').value, /0 of 1/)
  })

  test(`${path}: failed batches cannot starve later images and can be retried afterwards`, () => {
    const urls = Array.from({length: 27}, (_, i) => `https://example.test/${i}.png`)
    const p = page(path, urls)
    p.helper.load()
    assert.deepEqual([...p.requests[0].body.urls], urls.slice(0, 12))
    p.requests[0].resolve(urls.slice(0, 12).map(url => ({url, error: 'unavailable'})))
    assert.match(p.$('#templateImageStatus').value, /remaining images/)
    p.helper.load()
    assert.deepEqual([...p.requests[1].body.urls], urls.slice(12, 24))
    p.requests[1].resolve([{url: urls[12], data: png}])
    p.helper.load()
    assert.deepEqual([...p.requests[2].body.urls], urls.slice(24))
    p.requests[2].resolve(urls.slice(24).map(url => ({url, data: png})))
    assert.match(p.$('#templateImageStatus').value, /retry unavailable images/)
    p.helper.load()
    assert.deepEqual([...p.requests[3].body.urls], urls.slice(0, 12))
    p.requests[3].resolve([])
    p.helper.load()
    assert.deepEqual([...p.requests[4].body.urls], urls.slice(13, 24))
    p.requests[4].resolve([])
    p.helper.reset(p.editor)
    p.helper.load()
    assert.deepEqual([...p.requests[5].body.urls], urls.slice(0, 12))
  })

  test(`${path}: HTTP failures remain retryable and numeric HTTPS ports keep original URLs`, () => {
    const urls = ['https://example.test:0443/logo.png', 'https://example.test/other.png']
    const p = page(path, urls)
    p.helper.load()
    assert.deepEqual([...p.requests[0].body.urls], urls)
    p.requests[0].reject()
    p.helper.load()
    assert.deepEqual([...p.requests[1].body.urls], urls)
    p.requests[1].resolve(urls.map(url => ({url, data: png})))
    assert.equal(p.nodes[0].attrs['data-cke-saved-src'], urls[0])
    assert.equal(p.nodes[0].attrs.src, png)
  })
}

test('image preview is wired to the template editor without relaxing admin CSP', () => {
  const read = path => readFileSync(new URL('../' + path, import.meta.url), 'utf8')
  assert.match(read('templates/templates.html'), /template_images.min.js/)
  assert.match(read('templates/templates.html'), /may register an email open/)
  assert.match(read('static/js/src/app/templates.js'), /templateImagePreview.reset\(\)/)
  assert.match(read('static/js/src/app/darkphish.js'), /query\("\/import\/email\/images", "POST", req, true\)/)
  assert.match(read('middleware/middleware.go'), /img-src 'self' data:;/)
  assert.doesNotMatch(read('middleware/middleware.go'), /img-src[^;]*https:/)
})
