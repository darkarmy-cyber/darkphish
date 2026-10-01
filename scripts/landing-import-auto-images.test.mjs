import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const pageJS = readFileSync(new URL("../static/js/src/app/landing_pages.js", import.meta.url), "utf8")
const template = readFileSync(new URL("../templates/landing_pages.html", import.meta.url), "utf8")

test("landing-page import writes the response into CKEditor", () => {
  assert.match(pageJS, /CKEDITOR\.instances\["html_editor"\]\.setData\(data\.html\)/)
  assert.doesNotMatch(pageJS, /trainingStatic|trainingImportPending|trainingImagePreview|setTrainingStatic/)
})

test("landing-page form exposes capture and redirect controls", () => {
  assert.match(template, /Capture Submitted Data/)
  assert.match(template, /id="capture_passwords"/)
  assert.match(pageJS, /#capture_passwords, #redirect_url/)
  assert.doesNotMatch(template, /trainingStaticNotice|training_images\.min\.js/)
})
