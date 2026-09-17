import assert from 'node:assert/strict'
import {readFileSync, existsSync} from 'node:fs'
import {resolve} from 'node:path'
import {fileURLToPath} from 'node:url'
import test from 'node:test'

const root = fileURLToPath(new URL('../', import.meta.url))
const read = path => readFileSync(resolve(root, path), 'utf8')

test('all administrative shells use local DarkPhish brand assets', () => {
  for (const name of ['login', 'reset_password', 'base']) {
    const html = read(`templates/${name}.html`)
    assert.match(html, /rel="icon" type="image\/svg\+xml" href="\/images\/darkphish-mark.svg"/)
    assert.match(html, /class="navbar-brand darkphish-brand"/)
    assert.doesNotMatch(html, /logo_inv_small|darkphish_banner\.png|favicon\.ico|docs-assets/)
    for (const [, path] of html.matchAll(/(?:src|href)="(\/images\/[^"]+)"/g)) {
      assert.ok(existsSync(resolve(root, `static${path}`)), `${name}: ${path} must ship`)
    }
  }
})

test('both documentation menu destinations are the official site', () => {
  const nav = read('templates/nav.html')
  for (const label of ['User Guide', 'API Documentation']) {
    assert.ok(nav.includes(`<a href="https://docs.darkphish.sk/">${label}</a>`))
  }
  assert.doesNotMatch(nav, /getdarkphish\.com/)
})

test('brand derivatives preserve existing vector artwork without external resources', () => {
  const source = read('docs/assets/darkphish_banner.svg')
  const fish = source.match(/<g transform="translate\(585 165\)"[\s\S]*?<\/g>/)[0]
  for (const name of ['darkphish-mark.svg']) {
    const svg = read(`static/images/${name}`)
    assert.ok(svg.includes(fish), `${name}: preserve original fish artwork`)
    assert.doesNotMatch(svg, /<script|<foreignObject|(?:href|src)=|\son\w+=/i)
  }
})

test('the main brand image is the unchanged original DarkPhish artwork', () => {
  assert.deepEqual(readFileSync(resolve(root, 'static/images/darkphish-brand.png')),
    readFileSync(resolve(root, 'docs/assets/DarkPhish.png')))
})

test('authentication forms preserve POST and password fields', () => {
  for (const name of ['login', 'reset_password']) {
    const html = read(`templates/${name}.html`)
    assert.match(html, /<form class="form-signin" action="" method="POST">/)
    assert.match(html, /type="password"[^>]*name="password"/)
    assert.match(html, /template "flashes"/)
    assert.match(html, /body class="auth-page"/)
    assert.match(html, /id="logo"[^>]*width="1600" height="873"/)
  }
  assert.match(read('templates/reset_password.html'), /name="confirm_password"/)
  assert.match(read('templates/reset_password.html'), /minlength="12"/)
  const css = read('static/css/main.css')
  assert.match(css, /\.auth-page #logo\s*\{[^}]*max-width: 100%;[^}]*height: auto;/)
})
