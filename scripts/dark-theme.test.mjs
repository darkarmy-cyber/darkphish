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
  assert.match(base, /theme-toggle-track/)
  assert.match(base, /theme-toggle-orb/)
  assert.match(base, /theme-toggle-cloud/)
  assert.match(base, /theme-toggle-stars/)
  assert.match(base, /css\/dist\/darkphish\.css\?v=\{\{\.Version\}\}/)
  assert.doesNotMatch(base, /href="\/css\/dark-theme\.css/)
  assert.match(base, /theme\.js\?v=\{\{\.Version\}\}/)
  assert.match(controller, /darkphish-theme/)
  assert.match(controller, /setAttribute\("data-theme", "dark"\)/)
  assert.match(controller, /addEventListener\("click"/)
  assert.match(controller, /setAttribute\("aria-label"/)
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
  assert.match(css, /darkphish-theme-toggle\[aria-pressed="true"\]/)
  assert.match(css, /theme-toggle-orb/)
  assert.match(css, /height: 44px/)
  assert.match(css, /forced-colors: active/)
})

test("dark theme assets are included in Docker and native release packages", () => {
  const docker = read("Dockerfile")
  assert.match(docker, /theme\.js \.\/static\/js\/src\/app\/theme\.js/)
  assert.match(docker, /static\/css\/dist \.\/static\/css\/dist/)
  assert.match(read("gulpfile.js"), /css_directory \+ 'dark-theme\.css'/)
  assert.match(read("static/css/dist/darkphish.css"), /data-theme=dark/)
  const release = read(".github/workflows/release.yml")
  assert.match(release, /Copy-Item static\/css\/dist -Recurse/)
  assert.match(release, /Copy-Item static\/js\/dist,static\/js\/src -Recurse/)
})
