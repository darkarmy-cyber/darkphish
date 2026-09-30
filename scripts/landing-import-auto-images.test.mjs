import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const pageJS = readFileSync(new URL("../static/js/src/app/landing_pages.js", import.meta.url), "utf8")
const template = readFileSync(new URL("../templates/landing_pages.html", import.meta.url), "utf8")

test("landing-page import localizes safe public images automatically", () => {
  assert.match(pageJS, /CKEDITOR\.instances\["html_editor"\]\.setData\(data\.html,[\s\S]*trainingImagePreview\.load\(\)/)
  assert.doesNotMatch(template, /Load verified images/)
  assert.match(template, /Public HTTPS raster images are localized automatically after import/)
})

test("automatic import remains restricted to inert training content", () => {
  assert.match(pageJS, /setTrainingStatic\(data\.training_static\)/)
  assert.match(template, /Dynamic scripts, credential forms and private\/internal resources are not imported/)
})
