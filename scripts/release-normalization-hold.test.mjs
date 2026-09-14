import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { normalizationHold } from "./release-normalization-hold.mjs"

const source = readFileSync(new URL("./release-normalization-hold.mjs", import.meta.url), "utf8")

test("normalization hold is active and bound to current VERSION", () => {
  const hold = normalizationHold()
  assert.equal(hold.version, "0.7.1")
  assert.equal(hold.tag, "v0.7.1")
  assert.equal(hold.source, "73bf5948ed19cd918638453d478ef3f1cbe83d89")
})

test("normalization hold fails closed on schema, fields and source identity", () => {
  assert.match(source, /unexpected fields/)
  assert.match(source, /schema is unsupported/)
  assert.match(source, /source SHA is invalid/)
  assert.match(source, /does not match current VERSION/)
  assert.match(source, /versionTag\(value\.version\)/)
})
