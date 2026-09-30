import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

for (const path of [
  ".github/scripts/release-recover.mjs",
  ".github/scripts/release-postpublish-verify.mjs",
  ".github/scripts/release-publication-guard.mjs",
]) {
  test(path + " normalizes trusted GitHub release body line endings", () => {
    const source = read(path)
    assert.match(source, /matchesTrustedReleaseBody/)
    assert.doesNotMatch(source, /release\.body !== expected\.body/)
  })
}

test("shared release-body matcher is limited to CRLF normalization for ordinary releases", () => {
  const source = read("scripts/release-body-match.mjs")
  assert.match(source, /value\.replace\(\/\\r\\n\/g, "\\n"\)/)
  assert.match(source, /comparableReleaseBody\(actual\)/)
})
