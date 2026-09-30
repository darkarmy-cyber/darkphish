import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const files = [
  "scripts/release-publish.mjs",
  ".github/scripts/release-recover.mjs",
  ".github/scripts/release-publication-guard.mjs",
  ".github/scripts/release-postpublish-verify.mjs",
]

test("all publication paths share the canonical release display name helper", () => {
  for (const path of files) {
    const source = readFileSync(new URL("../" + path, import.meta.url), "utf8")
    assert.match(source, /releaseDisplayName/)
    assert.doesNotMatch(source, /version\.split\("\."\)\.slice\(0, 2\)\.join\("\."\)/)
  }
})
