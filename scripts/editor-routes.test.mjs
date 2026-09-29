import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

test("v0.22 editors expose stable route-addressable entry points", () => {
  const templates = read("templates/templates.html")
  const pages = read("templates/landing_pages.html")
  assert.match(templates, /href="\/templates\/new"/)
  assert.match(pages, /href="\/landing_pages\/new"/)
  assert.match(templates, /editor_routes\.js\?v=\{\{\.Version\}\}/)
  assert.match(pages, /editor_routes\.js\?v=\{\{\.Version\}\}/)
})

test("editor route controller validates owned records before rendering", () => {
  const routes = read("controllers/route.go")
  assert.match(routes, /\/templates\/\{id:\[0-9\]\+\}\/\{mode:edit\|copy\}/)
  assert.match(routes, /models\.GetTemplate\(id, params\.User\.Id\)/)
  assert.match(routes, /\/landing_pages\/\{id:\[0-9\]\+\}\/\{mode:edit\|copy\}/)
  assert.match(routes, /models\.GetPage\(id, params\.User\.Id\)/)
})

test("route editor guards unsaved changes and converts list actions to routes", () => {
  const source = read("static/js/src/app/editor_routes.js")
  assert.match(source, /beforeunload/)
  assert.match(source, /You have unsaved changes/)
  assert.match(source, /button\[onclick\]/)
  assert.match(source, /window\.location\.assign\(path \+ "\/" \+ items\[idx\]\.id \+ "\/" \+ action\[1\]\)/)
  assert.match(source, /\/api\/import\//)
})
