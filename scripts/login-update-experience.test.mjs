import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import test from 'node:test'
const read = path => readFileSync(new URL('../' + path, import.meta.url), 'utf8')

test('login uses the site identity without a footer and retains real authentication', () => {
  const html = read('templates/login.html')
  assert.match(html, /Phishing<br>simulations\./)
  assert.match(html, /Built for<br>defenders\./)
  assert.doesNotMatch(html, /<footer|auth-signature|auth-eyebrow/)
  assert.match(html, /method="POST"/)
  assert.match(html, /autocomplete="current-password"/)
  assert.match(html, /template "flashes"/)
  assert.doesNotMatch(html, /<script[^>]+src="https?:/)
})

test('login and admin CSS and update script are versioned across releases', () => {
  for (const name of ['login','base','reset_password']) assert.match(read(`templates/${name}.html`), /darkphish\.css\?v={{\.Version}}/)
  assert.match(read('templates/update.html'), /update\.min\.js\?v={{\.Version}}/)
  assert.equal((read('controllers/route.go').match(/Title: "Login", Version: config.Version/g) || []).length, 2)
})

test('credentials stay white with dark text including autofill and focus', () => {
  const css = read('static/css/auth-update.css')
  assert.match(css, /background: #fff !important; color: #111 !important; caret-color: #111/)
  assert.match(css, /input\.form-control:focus/)
  assert.match(css, /input\.form-control:autofill/)
  assert.match(css, /input\.form-control:-webkit-autofill/)
  assert.match(css, /-webkit-text-fill-color: #111 !important/)
  assert.match(read('static/css/dist/darkphish.css'), /body\.auth-login/)
})

test('sidebar credit and documentation open safely in a separate tab', () => {
  const html = read('templates/nav.html')
  for (const [,tag] of html.matchAll(/(<a href="https:[^>]+>)/g)) {
    assert.match(tag, /target="_blank"/)
    assert.match(tag, /rel="noopener noreferrer"/)
  }
  assert.match(html, /Developed by <a href="https:\/\/www.darkarmy.sk\/"[^>]*>Dark Army<\/a>/)
  assert.match(read('static/css/auth-update.css'), /margin: auto 0 0/)
})
