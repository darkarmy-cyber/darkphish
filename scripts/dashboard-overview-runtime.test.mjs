import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

test("dashboard KPI controller is included in the production runtime image", () => {
  const html = read("templates/dashboard.html")
  const docker = read("Dockerfile")
  assert.match(html, /\/js\/src\/app\/dashboard_overview\.js\?v=\{\{\.Version\}\}/)
  assert.match(docker, /dashboard_overview\.js \.\/static\/js\/src\/app\/dashboard_overview\.js/)
})
