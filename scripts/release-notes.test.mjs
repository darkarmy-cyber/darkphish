import assert from "node:assert/strict"
import test from "node:test"
import { canonicalReleaseNotes, releaseBody, releaseSection, requiredReleaseSections } from "./release-notes.mjs"

const requiredOrder = ["### Changed", "### Fixed", "### Security", "### Migration"]

function assertCanonical(notes) {
  for (const name of requiredReleaseSections) assert.match(notes, new RegExp(`^### ${name}$`, "m"))
  const offsets = requiredOrder.map((heading) => notes.indexOf(heading))
  assert.ok(offsets.every((offset) => offset >= 0))
  assert.deepEqual([...offsets].sort((a, b) => a - b), offsets)
  assert.equal((notes.match(/^### /gm) || []).length, 4)
}

test("canonical release notes always contain exactly the required 0.5-style sections", () => {
  const section = "## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- Fix one thing.\n\n### Security\n\n- Harden one thing."
  const notes = canonicalReleaseNotes(section)
  assertCanonical(notes)
  assert.match(notes, /### Changed\n\n- No notable changes in this release\./)
  assert.match(notes, /### Migration\n\n- No migration steps are required for this release\./)
})

test("legacy Added and Deprecated content is folded into Changed", () => {
  const section = "## 0.3.0 - 2026-09-05\n\n### Added\n\n- New feature.\n\n### Changed\n\n- Changed behavior.\n\n### Deprecated\n\n- Old path deprecated.\n\n### Security\n\n- Hardened."
  const notes = canonicalReleaseNotes(section)
  assertCanonical(notes)
  assert.match(notes, /### Changed\n\n- New feature\.\n\n- Changed behavior\.\n\n- Old path deprecated\./)
  assert.doesNotMatch(notes, /^### Added$/m)
  assert.doesNotMatch(notes, /^### Deprecated$/m)
})

test("legacy Breaking content is folded into Migration", () => {
  const section = "## 0.2.0 - 2026-09-04\n\n### Changed\n\n- Changed.\n\n### Breaking\n\n- Existing API keys stop authenticating."
  const notes = canonicalReleaseNotes(section)
  assertCanonical(notes)
  assert.match(notes, /### Migration\n\n- Existing API keys stop authenticating\./)
  assert.doesNotMatch(notes, /^### Breaking$/m)
})

test("release lookup trims the legacy changelog prose between historical releases", () => {
  const changelog = "# Changelog\n\n## 0.4.0 - 2026-09-06\n\n### Fixed\n\n- Fix.\n\nAll notable changes to Darkphish are documented here. The project follows\nSemantic Versioning while the public API and schema are still pre-1.0.\n\n## 0.3.0 - 2026-09-05\n\n### Changed\n\n- Old.\n"
  const section = releaseSection(changelog, "0.4.0")
  assert.doesNotMatch(section, /All notable changes/)
  assert.match(section, /^## 0\.4\.0 - 2026-09-06/)
})

test("release body is deterministically bound to source commit", () => {
  const changelog = "# Changelog\n\n## 0.7.1 - 2026-09-13\n\n### Fixed\n\n- Fix.\n\n## 0.7.0 - 2026-09-07\n\n### Changed\n\n- Old.\n"
  const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
  const body = releaseBody(changelog, "0.7.1", source)
  assertCanonical(body)
  assert.match(body, new RegExp(`Source commit: ${source}`))
  assert.match(body, new RegExp(`darkphish-release-source:${source}`))
})

test("unknown categories fail closed instead of silently changing release semantics", () => {
  assert.throws(() => canonicalReleaseNotes("## 0.8.0 - 2026-09-14\n\n### Experimental\n\n- Hidden behavior."), /unsupported release notes section Experimental/)
})

test("leading-zero SemVer is rejected consistently", () => {
  assert.throws(() => releaseSection("## 01.2.3 - 2026-09-14\n", "01.2.3"), /stable SemVer/)
  assert.throws(() => canonicalReleaseNotes("## 01.2.3 - 2026-09-14\n\n### Fixed\n\n- Fix."), /heading is malformed/)
})

test("section headings with leading or trailing whitespace fail closed", () => {
  assert.throws(() => canonicalReleaseNotes("## 0.8.0 - 2026-09-14\n\n### Security \n\n- Hidden disclosure."), /section heading is malformed/)
  assert.throws(() => canonicalReleaseNotes("## 0.8.0 - 2026-09-14\n\n###  Security\n\n- Hidden disclosure."), /section heading is malformed/)
})
