import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync, existsSync} from 'node:fs'

const read = path => readFileSync(new URL('../' + path, import.meta.url), 'utf8')

test('manual external image preview workflow is removed from the template editor', () => {
  const html = read('templates/templates.html')
  const gulp = read('gulpfile.js')
  assert.doesNotMatch(html, /Load external images|loadTemplateImages|templateImagePreview|template_images\.min\.js/)
  assert.doesNotMatch(gulp, /template_images\.js/)
  assert.equal(existsSync(new URL('../static/js/src/app/template_images.js', import.meta.url)), false)
  assert.equal(existsSync(new URL('../static/js/dist/app/template_images.min.js', import.meta.url)), false)
})

test('email source import is an asynchronous enterprise workflow', () => {
  const html = read('templates/templates.html')
  const api = read('static/js/src/app/darkphish.js')
  const source = read('static/js/src/app/templates.js')
  const dist = read('static/js/dist/app/templates.min.js')
  const apiDist = read('static/js/dist/app/darkphish.min.js')

  assert.match(html, /id="importEmailSubmit"/)
  assert.match(html, /Import Email Source/)
  assert.match(html, /supported public HTTPS images are imported automatically/)
  assert.match(html, /id="emailImportStatus"[^>]*role="status"/)
  assert.doesNotMatch(html, /id="modalSubmit"[^>]*onclick="importEmail/)

  assert.match(api, /query\("\/import\/email", "POST", req, true\)/)
  assert.doesNotMatch(api, /preview_email_images|\/import\/email\/images/)
  assert.doesNotMatch(apiDist, /import\/email\/images/)

  for (const js of [source, dist]) {
    assert.match(js, /previewInlineAssetRefs/)
    assert.match(js, /restoreInlineAssetRefs/)
    assert.match(js, /data\.attachments/)
    assert.match(js, /Import completed with warnings/)
    assert.doesNotMatch(js, /templateImagePreview|loadTemplateImages/)
  }
})

test('imported CID previews are restored before template persistence', () => {
  const source = read('static/js/src/app/templates.js')
  assert.match(source, /template\.html = restoreInlineAssetRefs\(CKEDITOR\.instances\["html_editor"\]\.getData\(\)\)/)
  assert.match(source, /"cid:" \+ file\.name/)
  assert.match(source, /"data:" \+ type \+ ";base64," \+ file\.content/)
  assert.match(source, /attachmentsTable\.clear\(\)\.draw\(\)/)
})
