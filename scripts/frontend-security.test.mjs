import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import vm from "node:vm"

const source = (path) => readFileSync(new URL(`../static/js/src/${path}`, import.meta.url), "utf8")

test("date ordering parses only strict plaintext dates, without HTML rewriting", () => {
  const types = { detect: [], order: {} }
  const context = vm.createContext({ jQuery: { fn: { dataTable: { ext: { type: types } } } } })
  vm.runInContext(source("vendor/moment.min.js"), context)
  vm.runInContext(source("vendor/datetime-moment.js"), context)
  context.jQuery.fn.dataTable.moment("YYYY-MM-DD")
  const detect = types.detect[0], order = types.order["moment-YYYY-MM-DD-pre"]
  assert.equal(detect("2026-09-06"), "moment-YYYY-MM-DD")
  assert.ok(order("2026-09-05") < order("2026-09-06"))
  for (const value of ["", null]) {
    assert.equal(detect(value), "moment-YYYY-MM-DD")
    assert.equal(order(value), -Infinity)
  }
  for (const value of ["<b>2026-09-06</b>", "<script<>>2026-09-06", "2026-02-31", "2026-09-06<img src=x>"]) {
    assert.equal(detect(value), null)
    assert.ok(Number.isNaN(order(value)))
  }
})

test("spellcheck query names are literal and values retain the bridge's encoded protocol", () => {
  const context = vm.createContext({ window: { location: { search: "?cmd=done&data=a%26b%3Dc+raw==&a[[]]=literal&x.y=dot&xay=other", href: "https://admin.example.test/?cmd=done&data=a%26b%3Dc+raw==&a[[]]=literal&x.y=dot&xay=other#data=wrong" } } })
  const script = source("vendor/ckeditor/plugins/wsc/dialogs/ciframe.html").match(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/i)[1]
  vm.runInContext(script, context)
  assert.equal(context.gup("cmd"), "done")
  assert.equal(context.gup("data"), "a%26b%3Dc+raw==")
  assert.equal(context.gup("a[[]]"), "literal")
  assert.equal(context.gup("x.y"), "dot")
  assert.equal(context.gup("x.*"), "")
  assert.equal(context.gup("missing"), "")
  context.window.location.search = "?data=first&data=second"
  assert.equal(context.gup("data"), "first")
})

test("template autocomplete accepts all supported names but no ASCII range punctuation", () => {
  const context = vm.createContext({})
  vm.runInContext(source("app/autocomplete.js"), context)
  for (const tag of context.TEMPLATE_TAGS) {
    const text = `Hello {{.${tag.name}}}`
    const result = context.matchCallback(text, text.length)
    assert.equal(result.start, 6)
    assert.equal(result.end, text.length)
  }
  for (const punctuation of ["[", "\\", "]", "^", "_", "`", "<", ">"] ) {
    const text = `{{.First${punctuation}Name`
    assert.equal(context.matchCallback(text, text.length), null)
  }
})
