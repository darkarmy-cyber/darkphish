import assert from "node:assert/strict"
import test from "node:test"
import { comparableReleaseBody, matchesTrustedReleaseBody } from "./release-body-match.mjs"

const expected = "## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- trusted\n\nSource commit: 0123456789abcdef0123456789abcdef01234567"

test("trusted release body comparison accepts LF and CRLF encodings", () => {
  assert.equal(matchesTrustedReleaseBody(expected, expected), true)
  assert.equal(matchesTrustedReleaseBody(expected.replace(/\n/g, "\r\n"), expected), true)
})

test("trusted release body comparison does not normalize content changes", () => {
  assert.equal(matchesTrustedReleaseBody(expected.replace("trusted", "altered"), expected), false)
  assert.equal(matchesTrustedReleaseBody(`${expected}\nextra`, expected), false)
})

test("trusted release body comparison normalizes only CRLF", () => {
  const loneCR = expected.replace("### Fixed\n", "### Fixed\r")
  assert.equal(comparableReleaseBody(loneCR), loneCR)
  assert.equal(matchesTrustedReleaseBody(loneCR, expected), false)
})
