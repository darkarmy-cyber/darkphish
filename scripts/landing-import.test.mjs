import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const pageJS = readFileSync(new URL("../static/js/src/app/landing_pages.js", import.meta.url), "utf8")
const template = readFileSync(new URL("../templates/landing_pages.html", import.meta.url), "utf8")

test("landing-page import waits for CKEditor and uses sanitized API HTML", () => {
  assert.match(pageJS, /var importPending = false/)
  assert.match(pageJS, /if \(importPending\) return/)
  assert.match(pageJS, /setData\(data\.html, function \(\)/)
  assert.match(pageJS, /#modalSubmit.*disabled/s)
  assert.doesNotMatch(pageJS, /training|data-darkphish-training|static-v1/i)
})

test("landing-page form exposes capture and redirect controls", () => {
  assert.match(template, /Capture Submitted Data/)
  assert.match(template, /id="capture_passwords"/)
  assert.match(pageJS, /#capture_passwords, #redirect_url/)
  assert.doesNotMatch(template, /training/i)
})
