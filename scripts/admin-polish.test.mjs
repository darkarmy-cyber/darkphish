import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const read = path => readFileSync(new URL('../' + path, import.meta.url), 'utf8')

test('sidebar removes redundant visible heading but preserves accessible navigation and credit', () => {
  const nav = read('templates/nav.html')
  assert.doesNotMatch(nav, /class="sidebar-label"|>Workspace</)
  assert.match(nav, /aria-label="Workspace navigation"/)
  assert.match(nav, /Developed by/)
  for (const path of ['/campaigns', '/groups', '/templates', '/landing_pages', '/sending_profiles', '/settings']) {
    assert.ok(nav.includes('href="' + path + '"'))
  }
})

test('both import dialogs use explicit buttons and consistent scoped action styling', () => {
  for (const file of ['templates.html', 'landing_pages.html']) {
    const markup = read('templates/' + file)
    assert.equal((markup.match(/class="btn btn-primary btn-import"/g) || []).length, file === 'landing_pages.html' ? 3 : 2)
    assert.match(markup, /type="button" class="btn btn-primary btn-import" data-toggle="modal"/)
    assert.match(markup, /data-dismiss="modal" class="btn btn-default">Cancel/)
  }
  const css = read('static/css/classic-admin.css')
  assert.match(css, /\.classic-ui \.btn-import \{ background-color: #126e63/)
  assert.match(css, /\.classic-ui \.btn-import:focus/)
  assert.match(css, /\.classic-ui \.btn-import:disabled/)
  assert.match(read('static/css/dist/darkphish.css'), /\.classic-ui \.btn-import/)
})

for (const path of ['static/js/src/app/sending_profiles.js', 'static/js/dist/app/sending_profiles.min.js']) {
  function helper() {
    const context = vm.createContext({$: () => ({ready() {}}), window: {}, document: {}})
    vm.runInContext(read(path), context)
    return context.testEmailErrorMessage
  }
  test(path + ': SMTP 535 is explained without claiming success or changing authentication', () => {
    const message = 'Max connection attempts exceeded - 535 "5.7.8 Error: authentication failed: (reason unavailable)"'
    const result = helper()({responseJSON: {message}})
    assert.match(result, /SMTP server rejected authentication/)
    assert.match(result, /Do not disable TLS certificate validation/)
    assert.ok(result.endsWith(message))
  })
  test(path + ': other errors and missing JSON are handled without false success', () => {
    const format = helper()
    for (const response of [undefined, {}, {responseJSON: {}}, {responseJSON: {message: null}}, {responseJSON: {message: 535}}, {responseJSON: {message: ' '}}]) {
      assert.match(format(response), /could not be confirmed/)
    }
    for (const message of ['Connection timed out', 'A valid license is required', 'Unexpected code 1535', '<img src=x onerror=alert(1)>']) {
      assert.equal(format({responseJSON: {message}}), message)
    }
  })
}

test('server diagnostics are escaped at the rendering boundary', () => {
  assert.match(read('static/js/src/app/sending_profiles.js'), /escapeHtml\(testEmailErrorMessage\(data\)\)/)
  assert.match(read('static/js/dist/app/sending_profiles.min.js'), /escapeHtml\(testEmailErrorMessage\(/)
})
