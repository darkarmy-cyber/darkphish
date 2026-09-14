import assert from "node:assert/strict"
import test from "node:test"
import { comparableReleaseBody, matchesTrustedReleaseBody } from "./release-body-match.mjs"

const expected = "## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- trusted\n\nSource commit: 0123456789abcdef0123456789abcdef01234567"
const historicalSource = "355d2881a4d5eea162d46a1c155998b1fb8c9f36"
const canonicalFooter = "Native binaries, SHA-256 checksums and SPDX SBOM are attached."
const historicalFooter = `${canonicalFooter} Artifact attestations require GitHub Enterprise Cloud for this private repository; see docs/REPOSITORY_ADMIN.md.`
const historicalExpected = `## 0.7.0 - 2026-09-07\n\n### Changed\n\n- trusted historical release\n\nSource commit: ${historicalSource}\n\n<!-- darkphish-release-source:${historicalSource} -->\n\n${canonicalFooter}`

test("trusted release body comparison accepts LF and CRLF encodings", () => {
  assert.equal(matchesTrustedReleaseBody(expected, expected), true)
  assert.equal(matchesTrustedReleaseBody(expected.replace(/\n/g, "\r\n"), expected), true)
})

test("trusted release body comparison does not normalize content changes", () => {
  assert.equal(matchesTrustedReleaseBody(expected.replace("trusted", "altered"), expected), false)
  assert.equal(matchesTrustedReleaseBody(`${expected}\nextra`, expected), false)
})

test("trusted release body comparison normalizes only CRLF", () => {
  const loneCR = "trusted\rbody"
  assert.equal(comparableReleaseBody(loneCR), loneCR)
  assert.equal(matchesTrustedReleaseBody(loneCR, "trusted\nbody"), false)
})

test("trusted release body comparison accepts the exact immutable v0.7.0 publisher footer", () => {
  const historicalActual = historicalExpected.slice(0, -canonicalFooter.length) + historicalFooter
  assert.equal(matchesTrustedReleaseBody(historicalActual, historicalExpected), true)
  assert.equal(matchesTrustedReleaseBody(historicalActual.replace(/\n/g, "\r\n"), historicalExpected), true)
})

test("historical publisher footer remains bound to the exact immutable source and text", () => {
  const historicalActual = historicalExpected.slice(0, -canonicalFooter.length) + historicalFooter
  const otherSource = "0123456789abcdef0123456789abcdef01234567"
  const otherExpected = historicalExpected.replaceAll(historicalSource, otherSource)
  const otherActual = historicalActual.replaceAll(historicalSource, otherSource)
  assert.equal(matchesTrustedReleaseBody(otherActual, otherExpected), false)
  assert.equal(matchesTrustedReleaseBody(`${historicalActual} extra`, historicalExpected), false)
  assert.equal(matchesTrustedReleaseBody(historicalActual.replace("GitHub Enterprise Cloud", "GitHub Enterprise"), historicalExpected), false)
})
