import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import test from 'node:test'
const read = p => readFileSync(new URL('../' + p, import.meta.url), 'utf8')

test('enterprise stylesheet is shipped last, scoped to admin and uses no external assets', () => {
  const css = read('static/css/enterprise.css')
  const gulp = read('gulpfile.js')
  assert.ok(gulp.indexOf("css_directory + 'enterprise.css'") > gulp.indexOf("css_directory + 'select2-bootstrap.min.css'"))
  assert.match(read('templates/base.html'), /body class="enterprise-ui"/)
  assert.match(css, /--dp-canvas: #f5f3ee/)
  assert.match(css, /prefers-reduced-motion/)
  assert.match(css, /:focus-visible/)
  assert.doesNotMatch(css, /@import|https?:|url\(/)
  assert.match(read('static/css/dist/darkphish.css'), /--dp-canvas:#f5f3ee/)
})

test('real authentication is retained, without the redundant login link or black artwork panel', () => {
  for (const name of ['login','reset_password']) {
    const html = read(`templates/${name}.html`)
    assert.match(html, /method="POST"/)
    assert.match(html, /template "flashes"/)
    assert.doesNotMatch(html, /navbar-toggle|navbar-collapse|darkphish-brand\.png/)
    assert.match(html, /label for="password"/)
    assert.match(html, /autocomplete="(?:current|new)-password"/)
    assert.doesNotMatch(html, /<script[^>]+src="https?:/)
  }
  assert.doesNotMatch(read('templates/login.html'), /href="\/login"/)
  assert.match(read('templates/login.html'), /label for="username"/)
  assert.match(read('templates/reset_password.html'), /id="password-strength-bar"/)
  assert.match(read('templates/reset_password.html'), /minlength="12"/)
})

test('reduced-motion durations survive the shipped CSS minifier', () => {
  const bundle = read('static/css/dist/darkphish.css')
  const rule = bundle.match(/@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{([^}]+)\}/)?.[1]
  assert.ok(rule, 'shipped bundle contains the reduced-motion rule')
  assert.match(rule, /\.enterprise-ui\s*\*/)
  assert.match(rule, /\.auth-page\s*\*/)
  for (const property of ['animation-duration', 'transition-duration']) {
    const value = rule.match(new RegExp(`(?:[;{])\\s*${property}:\\s*([\\d.]+)(ms|s)\\s*!important(?:;|$)`))
    assert.ok(value, `${property} must be a valid important CSS time`)
    const milliseconds = Number(value[1]) * (value[2] === 's' ? 1000 : 1)
    assert.ok(milliseconds > 0 && milliseconds <= 1, `${property} is bounded to 1ms`)
  }
  assert.match(rule, /animation-iteration-count:1!important/)
  assert.match(rule, /scroll-behavior:auto!important/)
})

test('wide editors retain modal integration, import IDs and external image consent', () => {
  for (const name of ['templates','landing_pages']) {
    const html = read(`templates/${name}.html`)
    assert.match(html, /class="modal fade editor-workspace" id="modal"/)
    assert.match(html, /id="html_editor"/)
    assert.match(html, /onclick="dismiss\(\)"/)
    assert.match(html, /data-backdrop="static"/)
  }
  const email = read('templates/templates.html')
  assert.match(email, /aria-labelledby="templateModalLabel"/)
  assert.match(email, /id="loadTemplateImages"/)
  assert.match(email, /data-target="#importEmailModal"/)
})

test('default palette has readable normal text and primary action contrast', () => {
  const linear = n => (n /= 255) <= .04045 ? n / 12.92 : ((n + .055) / 1.055) ** 2.4
  const luminance = hex => hex.match(/\w\w/g).map(v => linear(parseInt(v,16))).reduce((n,v,i) => n + v * [.2126,.7152,.0722][i],0)
  for (const [fg,bg] of [['26352f','f5f3ee'],['606a63','fffefa'],['fffefa','245d4e'],['89571c','faf0dc'],['a13737','f9eaea']]) {
    const values = [luminance(fg),luminance(bg)].sort((a,b)=>b-a)
    assert.ok((values[0]+.05)/(values[1]+.05) >= 4.5, `${fg} on ${bg}`)
  }
})
