import assert from "node:assert/strict"
import test from "node:test"
import { canonicalReleaseNotes, releaseBody, releaseSection, requiredReleaseSections } from "./release-notes.mjs"

test("canonical release notes always contain the required 0.5-style sections", () => {
  const section = "## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- Fix one thing.\n\n### Security\n\n- Harden one thing."
  const notes = canonicalReleaseNotes(section)
  for (const name of requiredReleaseSections) assert.match(notes, new RegExp(`^### ${name}$`, "m"))
  assert.match(notes, /### Changed\n\n- No notable changes in this release\./)
  assert.match(notes, /### Migration\n\n- No migration steps are required for this release\./)
  assert.match(notes, /### Fixed\n\n- Fix one thing\./)
  assert.match(notes, /### Security\n\n- Harden one thing\./)
})

test("additional changelog categories are preserved after the required release sections", () => {
  const section = "## 0.8.0 - 2026-09-14\n\n### Added\n\n- New feature.\n\n### Fixed\n\n- Fix."
  const notes = canonicalReleaseNotes(section)
  assert.ok(notes.indexOf("### Migration") < notes.indexOf("### Added"))
  assert.match(notes, /### Added\n\n- New feature\./)
})

test("release body is deterministically bound to source commit", () => {
  const changelog = "# Changelog\n\n## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- Fix.\n\n## 0.7.0 - 2026-09-07\n\n### Changed\n\n- Old.\n"
  const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
  assert.equal(releaseSection(changelog, "0.7.1").includes("## 0.7.0"), false)
  const body = releaseBody(changelog, "0.7.1", source)
  assert.match(body, /### Changed/)
  assert.match(body, /### Fixed/)
  assert.match(body, /### Security/)
  assert.match(body, /### Migration/)
  assert.match(body, new RegExp(`Source commit: ${source}`))
  assert.match(body, new RegExp(`darkphish-release-source:${source}`))
})

test("malformed headings and source SHAs fail closed", () => {
  assert.throws(() => canonicalReleaseNotes("## release\n\n### Fixed\n\n- Fix."), /heading is malformed/)
  assert.throws(() => releaseBody("# Changelog\n\n## 0.8.0 - 2026-09-14\n\n### Fixed\n\n- Fix.", "0.8.0", "main"), /source SHA is malformed/)
})
