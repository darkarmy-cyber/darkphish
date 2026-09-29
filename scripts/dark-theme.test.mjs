import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

function luminance(hex) {
  const rgb = hex.match(/\w\w/g).map(v => parseInt(v, 16) / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4)
  return .2126 * rgb[0] + .7152 * rgb[1] + .0722 * rgb[2]
}
function contrast(a, b) {
  const values = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (values[0] + .05) / (values[1] + .05)
}

test("dark theme is explicit, persistent and keyboard accessible", () => {
  const base = read("templates/base.html")
  const controller = read("static/js/src/app/theme.js")
  assert.match(base, /id="themeToggle"/)
  assert.match(base, /aria-pressed="false"/)
  assert.match(base, /dark-theme\.css\?v=\{\{\.Version\}\}/)
  assert.match(base, /theme\.js\?v=\{\{\.Version\}\}/)
  assert.match(controller, /darkphish-theme/)
  assert.match(controller, /setAttribute\("data-theme", "dark"\)/)
  assert.match(controller, /addEventListener\("click"/)
})

test("dark theme keeps required contrast for core text and actions", () => {
  assert.ok(contrast("e8efe9", "111715") >= 4.5)
  assert.ok(contrast("aebbb2", "111715") >= 4.5)
  assert.ok(contrast("ffffff", "2d8067") >= 4.5)
  assert.ok(contrast("f1b5b5", "351f1f") >= 4.5)
})

test("dark theme preserves reduced motion and a readable CKEditor canvas", () => {
  const css = read("static/css/dark-theme.css")
  assert.match(css, /prefers-reduced-motion: reduce/)
  assert.match(css, /animation-duration: 1ms !important/)
  assert.match(css, /\.cke_contents\s*\{\s*background: #ffffff/)
  assert.match(css, /:focus-visible/)
})

test("dark theme assets are included in production Docker images", () => {
  const docker = read("Dockerfile")
  assert.match(docker, /theme\.js \.\/static\/js\/src\/app\/theme\.js/)
  assert.match(docker, /dark-theme\.css \.\/static\/css\/dark-theme\.css/)
})
