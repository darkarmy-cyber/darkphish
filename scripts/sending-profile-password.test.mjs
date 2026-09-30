import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const js = readFileSync(new URL("../static/js/src/app/sending_profiles.js", import.meta.url), "utf8")
const api = readFileSync(new URL("../controllers/api/util.go", import.meta.url), "utf8")
const model = readFileSync(new URL("../models/smtp.go", import.meta.url), "utf8")

test("saved SMTP passwords remain write-only in the browser", () => {
  assert.match(model, /Password\\s+string\\s+`json:"-"/)
  assert.match(model, /PasswordSet\\s+bool\\s+`json:"password_set"/)
  assert.doesNotMatch(js, /\\.val\\(profile\\.password\\)/)
  assert.match(js, /Stored password - leave blank to keep/)
})

test("test email from an edited profile identifies the saved profile without exposing its password", () => {
  assert.match(js, /var activeProfileId = 0/)
  assert.match(js, /activeProfileId = profile\\.id/)
  assert.match(js, /smtp:\\s*\\{[\\s\\S]*id: activeProfileId/)
  assert.match(api, /s\\.SMTP\\.Id != 0 && s\\.SMTP\\.Password == ""/)
  assert.match(api, /models\\.GetSMTP\\(s\\.SMTP\\.Id, s\\.UserId\\)/)
  assert.match(api, /s\\.SMTP\\.Password = stored\\.Password/)
})

test("new and copied profiles do not silently reuse another stored password", () => {
  assert.match(js, /function copy\\(idx\\) \\{\\s*activeProfileId = 0/)
  assert.match(js, /Enter password for copied profile/)
  assert.match(js, /function dismiss\\(\\)[\\s\\S]*activeProfileId = 0/)
})
