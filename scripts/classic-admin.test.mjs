import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import test from 'node:test'
const read = p => readFileSync(new URL('../' + p, import.meta.url), 'utf8')

test('classic shell is explicit and shipped after the optional enterprise theme', () => {
  assert.match(read('templates/base.html'), /<body class="classic-ui">/)
  assert.doesNotMatch(read('templates/base.html'), /class="[^"]*enterprise-ui/)
  const gulp = read('gulpfile.js')
  assert.ok(gulp.indexOf("css_directory + 'classic-admin.css'") > gulp.indexOf("css_directory + 'auth-update.css'"))
  assert.match(read('static/css/dist/darkphish.css'), /body\.classic-ui/)
  assert.match(read('templates/dashboard.html'), /col-sm-9 col-sm-offset-3 col-md-10 col-md-offset-2 main/)
  assert.match(read('templates/nav.html'), /col-sm-3 col-md-2 sidebar/)
})

test('classic styling retains accessibility and responsive navigation without touching authentication', () => {
  const css = read('static/css/classic-admin.css')
  assert.match(css, /background: #fff; color: #34495e/)
  assert.match(css, /:focus-visible/)
  assert.match(css, /max-width: 767px/)
  assert.match(css, /\.classic-ui \.sidebar-credit \{ margin: auto 0 0/)
  assert.doesNotMatch(css, /auth-page|auth-login|@import|https?:|url\(/)
  assert.match(read('templates/login.html'), /class="auth-page auth-login"/)
})

test('classic presentation keeps licensing warnings, current update feedback and external links', () => {
  assert.match(read('templates/dashboard.html'), /template "license_warning"/)
  assert.match(read('templates/license_warning.html'), /Campaigns and test emails cannot be sent/)
  assert.match(read('templates/update.html'), /update\.min\.js\?v={{\.Version}}/)
  assert.match(read('static/css/auth-update.css'), /\.update-progress-track/)
  assert.match(read('templates/nav.html'), /Developed by/)
  assert.match(read('templates/base.html'), /id="adminNotifications"/)
  assert.doesNotMatch(read('templates/base.html'), /navbar-toggle|navbar-collapse/)
})
