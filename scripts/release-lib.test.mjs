import assert from "node:assert/strict"
import test from "node:test"
import { assetDisposition, assertGeneratedCommits, assertReleaseState, checksPassed, generatedPath, verifyChecksums, versionTag } from "./release-lib.mjs"

test("require every latest trusted check to succeed", () => {
  const check = { id: 1, name: "Go", app: { slug: "github-actions" }, status: "completed", conclusion: "success" }
  assert.equal(checksPassed([check], ["Go"]), true)
  assert.equal(checksPassed([check], ["Go", "Security"]), false)
  assert.equal(checksPassed([check, { ...check, id: 2, status: "in_progress" }], ["Go"]), false)
  assert.equal(checksPassed([{ ...check, app: { slug: "unknown" } }], ["Go"]), false)
  assert.equal(checksPassed([{ ...check, conclusion: "skipped" }], ["Go"]), false)
})
test("generated recovery refuses unknown commits and application files", () => {
  const commits = [{ author: "github-actions[bot]", subject: "release: Darkphish 0.3.0" }]
  assert.doesNotThrow(() => assertGeneratedCommits(commits, ["CHANGELOG.md", "changes/fix.md"], "0.3.0"))
  assert.throws(() => assertGeneratedCommits([{ ...commits[0], author: "developer" }], ["CHANGELOG.md"], "0.3.0"))
  assert.throws(() => assertGeneratedCommits(commits, ["models/user.go"], "0.3.0"))
  assert.equal(generatedPath("changes/../models/user.go"), false)
  assert.throws(() => versionTag("0.3.0;echo unsafe"))
})
test("release recovery never moves a tag or adopts unknown publication", () => {
  assert.equal(assertReleaseState(null, null, "abc"), "new")
  const release = { draft: true, body: "<!-- darkphish-release-source:abc -->" }
  assert.equal(assertReleaseState("abc", release, "abc"), "resume")
  assert.equal(assertReleaseState(null, { ...release, target_commitish: "abc" }, "abc"), "resume")
  assert.equal(assertReleaseState("abc", { ...release, draft: false }, "abc"), "published")
  assert.throws(() => assertReleaseState("old", release, "abc"))
  assert.throws(() => assertReleaseState("abc", { draft: true, body: "unknown" }, "abc"))
})
test("asset retries reuse only identical bytes and never replace", () => {
  assert.equal(assetDisposition(null, "abc", 123), "upload")
  assert.equal(assetDisposition({ digest: "sha256:abc", size: 123 }, "abc", 123), "reuse")
  assert.throws(() => assetDisposition({ digest: "sha256:old", size: 123 }, "abc", 123))
})
test("checksums cover each artifact exactly once", () => {
  const digest = "a".repeat(64)
  const hashes = new Map([["darkphish-one.zip", digest], ["darkphish-two.tar.gz", digest], ["SHA256SUMS", "unused"]])
  const first = `${digest}  darkphish-one.zip`
  const second = `${digest}  darkphish-two.tar.gz`
  assert.doesNotThrow(() => verifyChecksums(`${first}\n${second}\n`, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${first}\n`, hashes))
  assert.throws(() => verifyChecksums(first, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${"b".repeat(64)}  darkphish-two.tar.gz`, hashes))
})
