import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const settings = readFileSync(new URL("../templates/settings.html", import.meta.url), "utf8")

test("Settings Account does not render the application version", () => {
  const account = settings.slice(settings.indexOf('id="mainSettings"'), settings.indexOf('id="uiSettings"'))
  assert.doesNotMatch(account, /DarkPhish version/)
  assert.doesNotMatch(account, /\{\{\.Version\}\}/)
  assert.match(account, /id="username"/)
})
